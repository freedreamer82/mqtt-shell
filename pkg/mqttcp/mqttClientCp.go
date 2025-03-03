package mqttcp

import (
	"errors"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/lithammer/shortuuid/v3"
	"io"
	"os"
	"path"
	"time"
)

type MqttClientCp struct {
	*MqttCp
	waitServerChan chan bool
	bufferInbound  chan MqttJsonCp
	uuid           string
	writer         io.Writer
}

func NewMqttClientCp(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string) *MqttClientCp {
	mqttOpts.SetOrderMatters(true)
	clientCp := MqttClientCp{uuid: shortuuid.New(), writer: os.Stdout, bufferInbound: make(chan MqttJsonCp, 5)}
	cp := NewCp(mqttOpts, rxTopic, txTopic)
	cp.SetDataCallback(clientCp.onDataRx)
	clientCp.MqttCp = cp
	return &clientCp
}

func (c *MqttClientCp) onDataRx(data MqttJsonCp) {
	if data.ClientUUID == c.uuid {
		c.bufferInbound <- data
	}
}

func (c *MqttClientCp) startUpClient() bool {
	if !c.IsRunning() {
		c.Start()
		time.Sleep(time.Second)
	}

	if !c.worker.GetMqttClient().IsConnected() {
		c.print("mqtt connection fail")
		return false
	}
	return true
}

func (c *MqttClientCp) CopyRemoteToLocal(remoteFile string, localPath string) {
	connection := c.startUpClient()
	if !connection {
		return
	}

	if !path.IsAbs(remoteFile) {
		c.print("remote path must be absolute")
		return
	}

	newLocalPath, errCheck := fileDestinationPathCheck(localPath, remoteFile)
	if errCheck != nil {
		c.print(errCheck.Error())
		return
	}

	startMsg, errHandShake := c.remote2LocalHandshakeProcedure(newLocalPath, remoteFile)
	if errHandShake != nil {
		c.printf("error in handshake: %s", errHandShake.Error())
		return
	} else {
		c.print("handshake success, start transmission")
		c.println()
	}

	inChan := make(chan []byte, 10000)

	onMftFrame := func(client MQTT.Client, msg MQTT.Message) {
		mqttPayload := msg.Payload()
		inChan <- mqttPayload
	}

	errSub := c.worker.Subscribe(startMsg.Topic, onMftFrame)
	if errSub != nil {
		c.printf("error in subscribe %s", errSub.Error())
		return
	}
	defer c.worker.Unsubscribe(startMsg.Topic)

	tmpName := fmt.Sprintf("%s.tmp", newLocalPath)
	f, errCreation := os.Create(tmpName)
	if errCreation != nil {
		c.printf("error in subscribe %s", errCreation.Error())
		os.Remove(tmpName)
		return
	}

	errTrans := c.Transmit(*startMsg)
	if errTrans != nil {
		c.printf("error in start msg %s", errTrans.Error())
		os.Remove(tmpName)
		return
	}

	errReceive := c.receiveFileAndCheck(f, inChan, startMsg.Request.MD5, startMsg.Request.Size)
	if errReceive != nil {
		c.print(errReceive.Error())
		os.Remove(tmpName)
		return
	}
	c.printf("file received with sucess: %s", newLocalPath)
	c.println()

}

func (c *MqttClientCp) CopyLocalToRemote(localFile string, remotePath string) {
	connection := c.startUpClient()
	if !connection {
		return
	}

	if !path.IsAbs(remotePath) {
		c.print("remote path must be absolute")
		return
	}

	size, md5Value, err := takeFileInfo(localFile)
	if err != nil {
		c.printf(err.Error())
		return
	}

	uuid, transmissionTopic, errHandShake := c.local2RemoteHandshakeProcedure(localFile, remotePath, size, md5Value)
	if errHandShake != nil {
		c.printf("error in handshake: %s", errHandShake.Error())
		return
	} else {
		c.print("handshake success, start transmission")
		c.println()
	}

	errTrans := c.mftTransmitFile(localFile, transmissionTopic)
	if errTrans != nil {
		c.printf("error in data transfer: %s", errTrans.Error())
		return
	} else {
		c.printf("%d bytes sent", size)
		c.println()
	}

	str, errV := c.verifyTransmission(uuid)
	if errV != nil {
		c.printf("error in data receiving: %s", errV.Error())
		return
	} else {
		c.printf("success: %s", str)
		c.println()
	}

}

func (c *MqttClientCp) verifyTransmission(uuid string) (string, error) {
	res, errEnd := c.awaitResponse(uuid, MqttCpStep_End, defaultTransmissionTimeout)
	if errEnd != nil {
		return "", errEnd
	} else {
		if res.Error == "" {
			return res.EndStr, nil
		} else {
			return "", errors.New(res.Error)
		}
	}
}

func (c *MqttClientCp) remote2LocalHandshakeProcedure(localFile, remoteFile string) (*MqttJsonCp, error) {

	msg := MqttJsonCp{}
	msg.ClientUUID = c.uuid
	msg.UUID = shortuuid.New()
	msg.Step = MqttCpStep_Handshake1
	msg.Request.Cmd = MqttCpCommand_CopyRemoteToLocal
	msg.Request.ClientPath = localFile
	msg.Request.ServerPath = remoteFile

	errTrans := c.Transmit(msg)
	if errTrans != nil {
		return nil, errTrans
	}

	res, errRes := c.awaitResponse(msg.UUID, MqttCpStep_Handshake2, c.handshakeTimeout)
	if errRes != nil {
		return nil, errRes
	}

	errHandshake := c.validateHandshake(res)
	if errHandshake != nil {
		return nil, errHandshake
	}

	startMsg := res
	startMsg.Step = MqttCpStep_Start

	return &startMsg, nil

}

func (c *MqttClientCp) local2RemoteHandshakeProcedure(localFile string, remotePath string, localFileSize int64, localFileMd5 string) (string, string, error) {

	msg := MqttJsonCp{}
	msg.ClientUUID = c.uuid
	msg.UUID = shortuuid.New()
	msg.Step = MqttCpStep_Handshake1
	msg.Request.Cmd = MqttCpCommand_CopyLocalToRemote
	msg.Request.Size = localFileSize
	msg.Request.MD5 = localFileMd5
	msg.Request.ClientPath = localFile
	msg.Request.ServerPath = remotePath

	errTrans := c.Transmit(msg)
	if errTrans != nil {
		return "", "", errTrans
	}

	res, errRes := c.awaitResponse(msg.UUID, MqttCpStep_Handshake2, c.handshakeTimeout)
	if errRes != nil {
		return "", "", errRes
	}

	errHandshake := c.validateHandshake(res)
	if errHandshake != nil {
		return "", "", errHandshake
	}

	transmissionTopic := res.Topic

	_, errStart := c.awaitResponse(msg.UUID, MqttCpStep_Start, c.handshakeTimeout)
	if errRes != nil {
		return "", "", errStart
	}

	return msg.UUID, transmissionTopic, nil

}

func (c *MqttClientCp) validateHandshake(response MqttJsonCp) error {
	if response.Error != "" {
		return errors.New(response.Error)
	} else if response.Topic == "" {
		return errors.New("topic missing")
	} else if response.Request.MD5 == "" {
		return errors.New("md5 missing")
	} else if response.Request.Size == 0 {
		return errors.New("size missing")
	}
	return nil
}

func (c *MqttClientCp) awaitResponse(msgUUID string, step MqttCpStep, timeout time.Duration) (MqttJsonCp, error) {
	ticker := time.NewTicker(timeout)
	for {
		select {
		case msg := <-c.bufferInbound:
			if msg.UUID == msgUUID && msg.Step == string(step) {
				return msg, nil
			}
		case <-ticker.C:
			return MqttJsonCp{}, errors.New("timeout")
		}
	}
}

func (c *MqttClientCp) print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(c.writer, a...)
}

func (c *MqttClientCp) println() (n int, err error) {
	return fmt.Fprintln(c.writer)
}

func (c *MqttClientCp) printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(c.writer, format, a...)
}
