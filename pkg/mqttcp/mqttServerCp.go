package mqttcp

import (
	"errors"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

type MqttServerCp struct {
	*MqttCp
	mutex             sync.Mutex
	connections       map[string]ClientCpConnection
	maxConnections    int
	timeoutConnection time.Duration
}

type ClientCpConnection struct {
	transferUUID   string
	connectionType MqttCpCommand
	msgChan        chan MqttJsonCp
	start          time.Time
}

func (c *ClientCpConnection) awaitResponse(step MqttCpStep, timeout time.Duration) (MqttJsonCp, error) {
	ticker := time.NewTicker(timeout)
	for {
		select {
		case msg := <-c.msgChan:
			if msg.UUID == c.transferUUID && msg.Step == string(step) {
				return msg, nil
			}
		case <-ticker.C:
			return MqttJsonCp{}, errors.New("timeout")
		}
	}
}

func NewMqttServerCp(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string, opts ...MqttCpOption) *MqttServerCp {
	mqttOpts.SetOrderMatters(true)
	serverCp := MqttServerCp{
		maxConnections:    defaultServerMaxConnections,
		timeoutConnection: defaultServerTimeoutConnection,
		connections:       make(map[string]ClientCpConnection),
	}
	cp := NewCp(mqttOpts, rxTopic, txTopic, opts...)
	cp.SetDataCallback(serverCp.OnDataRx)
	serverCp.MqttCp = cp

	go serverCp.closeOldConnections()
	return &serverCp
}

func (s *MqttServerCp) closeOldConnections() {
	ticker := time.NewTicker(defaultServerCheckConnectionInterval)
	for {
		select {
		case <-ticker.C:
			s.mutex.Lock()
			for k, e := range s.connections {
				if time.Now().Sub(e.start) >= s.timeoutConnection {
					delete(s.connections, k)
				}
			}
			s.mutex.Unlock()
		}
	}
}

func (s *MqttServerCp) OnDataRx(data MqttJsonCp) {
	if data.ClientUUID != "" {
		if data.Step == MqttCpStep_Handshake1 {
			s.handleNewHandshake(data)
		} else if data.Step == MqttCpStep_Start {
			s.mutex.Lock()
			connection, exist := s.connections[data.ClientUUID]
			if exist {
				connection.msgChan <- data
			} else {
				log.Error("client uuid not recognized")
			}
			s.mutex.Unlock()
		} else {
			log.Error("message with unhandled step")
		}
	} else {
		log.Error("client uuid missing")
	}
}

func (s *MqttServerCp) IsBusy() bool {
	return len(s.connections) >= s.maxConnections
}

func (s *MqttServerCp) failHandshake(msg MqttJsonCp, fail string) {
	msg.Step = MqttCpStep_Handshake2
	msg.Error = fail
	log.Error(fail)
	err := s.Transmit(msg)
	if err != nil {
		log.Error(err.Error())
	}

}

func (s *MqttServerCp) failStart(msg MqttJsonCp, fail string) {
	msg.Step = MqttCpStep_Start
	msg.Error = fail
	log.Error(fail)
	err := s.Transmit(msg)
	if err != nil {
		log.Error(err.Error())
	}
}

func (s *MqttServerCp) failEnd(msg MqttJsonCp, fail string) {
	msg.Step = MqttCpStep_End
	msg.Error = fail
	log.Error(fail)
	err := s.Transmit(msg)
	if err != nil {
		log.Error(err.Error())
	}
}

func (s *MqttServerCp) handleNewHandshake(data MqttJsonCp) {
	log.Info("new handshake request")
	if s.IsBusy() {
		s.failHandshake(data, "server busy, try again")
	} else {
		err := s.validateHandshakeMsg(&data)
		if err != nil {
			s.failHandshake(data, err.Error())
		} else {
			c := s.registerTransfer(data)
			switch data.Request.Cmd {
			case MqttCpCommand_CopyLocalToRemote:
				go s.runClientToServerTransfer(&data, c)
			case MqttCpCommand_CopyRemoteToLocal:
				go s.runServerToClientTransfer(&data, c)
			default:
				log.Error("unhandled mqtt cp command")
			}
		}
	}
}

func (s *MqttServerCp) runServerToClientTransfer(msg *MqttJsonCp, conn ClientCpConnection) {
	defer s.unregisterTransfer(msg.ClientUUID)

	msg.Step = MqttCpStep_Handshake2
	msg.Topic = mftTopic(msg.ClientUUID, msg.UUID)
	err := s.Transmit(*msg)
	if err != nil {
		log.Error(err.Error())
		return
	}

	startMsg, errStart := conn.awaitResponse(MqttCpStep_Start, s.handshakeTimeout)
	if errStart != nil {
		log.Error(errStart.Error())
		return
	} else if startMsg.Error != "" {
		log.Error(startMsg.Error)
		return
	}

	errTrans := s.mftTransmitFile(msg.Request.ServerPath, msg.Topic, nil)
	if errTrans != nil {
		log.Errorf("error in data transfer: %s", errTrans.Error())
	} else {
		log.Errorf("%d bytes sent", msg.Request.Size)
	}

}

func (s *MqttServerCp) runClientToServerTransfer(msg *MqttJsonCp, conn ClientCpConnection) {
	defer s.unregisterTransfer(msg.ClientUUID)

	msg.Step = MqttCpStep_Handshake2
	msg.Topic = mftTopic(msg.ClientUUID, msg.UUID)
	err := s.Transmit(*msg)
	if err != nil {
		log.Error(err.Error())
		return
	}

	inChan := make(chan []byte, 10000)

	onMftFrame := func(client MQTT.Client, msg MQTT.Message) {
		mqttPayload := msg.Payload()
		inChan <- mqttPayload
	}

	errSub := s.worker.Subscribe(msg.Topic, onMftFrame)
	if errSub != nil {
		log.Error(errSub.Error())
		s.failStart(*msg, errSub.Error())
		return
	}
	defer s.worker.Unsubscribe(msg.Topic)

	tmpName := fmt.Sprintf("%s.tmp", msg.Request.ServerPath)
	f, errCreation := os.Create(tmpName)
	if errCreation != nil {
		log.Error(errCreation.Error())
		s.failStart(*msg, errCreation.Error())
		return
	}

	msg.Step = MqttCpStep_Start
	errT := s.Transmit(*msg)
	if errT != nil {
		log.Error(errT.Error())
		return
	}

	errTrans := s.handleFileTransferClient2Server(f, inChan, msg.Request.MD5, msg.Request.Size)
	if errTrans != nil {
		log.Error(errTrans.Error())
		os.Remove(tmpName)
		s.failEnd(*msg, errTrans.Error())
		return
	}

	msg.Step = MqttCpStep_End
	finalMsg := fmt.Sprintf("file received with sucess: %s", msg.Request.ServerPath)
	msg.EndStr = finalMsg
	log.Info(finalMsg)
	errTx := s.Transmit(*msg)
	if errTx != nil {
		log.Error(errT.Error())
	}

}

func (s *MqttServerCp) handleFileTransferClient2Server(f *os.File, inChan chan []byte, md5Expected string, sizeExpected int64) error {
	fName := f.Name()
	errReception := s.mftReceiveFile(f, inChan, nil)
	f.Close()
	if errReception != nil {
		return errReception
	}
	size, md5, errInfo := takeFileInfo(fName)
	if errInfo != nil {
		return errInfo
	} else if size != sizeExpected {
		return errors.New(fmt.Sprintf("fail check actual size %d, expected: %d", size, sizeExpected))
	} else if md5 != md5Expected {
		return errors.New(fmt.Sprintf("fail check actual md5 %d, expected: %d", md5, md5Expected))
	}
	realName := strings.TrimSuffix(f.Name(), ".tmp")
	return os.Rename(fName, realName)
}

func (s *MqttServerCp) registerTransfer(data MqttJsonCp) ClientCpConnection {
	newConnection := ClientCpConnection{
		transferUUID:   data.UUID,
		start:          time.Now(),
		connectionType: MqttCpCommand(data.Request.Cmd),
		msgChan:        make(chan MqttJsonCp, 5),
	}
	s.mutex.Lock()
	s.connections[data.ClientUUID] = newConnection
	s.mutex.Unlock()
	return newConnection
}

func (s *MqttServerCp) unregisterTransfer(clientUuid string) {
	s.mutex.Lock()
	delete(s.connections, clientUuid)
	s.mutex.Unlock()
}

func (s *MqttServerCp) validateHandshakeMsg(data *MqttJsonCp) error {

	if data.UUID == "" {
		return errors.New("missing transfer uuid")
	} else if data.Request.ClientPath == "" {
		return errors.New("missing local path")
	} else if data.Request.ServerPath == "" {
		return errors.New("missing remote path")
	} else if !path.IsAbs(data.Request.ServerPath) {
		return errors.New("path must be absolute")
	}

	if data.Request.Cmd == MqttCpCommand_CopyLocalToRemote {
		if data.Request.MD5 == "" {
			return errors.New("missing transfer uuid")
		} else if data.Request.Size == 0 {
			return errors.New("missing size")
		}

		newServerPath, errCheck := fileDestinationPathCheck(data.Request.ServerPath, data.Request.ClientPath)
		if errCheck != nil {
			return errCheck
		}

		data.Request.ServerPath = newServerPath

	} else if data.Request.Cmd == MqttCpCommand_CopyRemoteToLocal {
		info, err := os.Stat(data.Request.ServerPath)
		if os.IsNotExist(err) {
			return errors.New(fmt.Sprintf("%s : not found", data.Request.ServerPath))
		} else if err != nil {
			return err
		} else if info.IsDir() {
			return errors.New(fmt.Sprintf("%s is a dir", data.Request.ServerPath))
		}

		size, md5Value, errInfo := takeFileInfo(data.Request.ServerPath)
		if errInfo != nil {
			return errInfo
		}

		data.Request.MD5 = md5Value
		data.Request.Size = size

	} else {
		return errors.New("command unrecognized")
	}
	return nil
}
