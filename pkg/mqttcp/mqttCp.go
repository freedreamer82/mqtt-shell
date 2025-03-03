package mqttcp

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/freedreamer82/mqtt-shell/pkg/mqtt"
	"github.com/freedreamer82/mqtt-shell/pkg/mqttcp/mft"
	"io"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

type OnDataCallback func(data MqttJsonCp)

type MqttCp struct {
	worker           *mqtt.Worker
	Cb               OnDataCallback
	txTopic          string
	rxTopic          string
	version          string
	startTime        time.Time
	isRunning        bool
	handshakeTimeout time.Duration
}

func (m *MqttCp) SetDataCallback(cb OnDataCallback) {
	m.Cb = cb
}

func (m *MqttCp) mftTransmitStart(no uint16, topic string) {
	mftFrame := mft.BuildMftStartFrame(no).Encode()
	log.Tracef("send mft start: %d bytes", len(mftFrame))
	m.worker.Publish(topic, mftFrame)
}

func (m *MqttCp) mftTransmitEnd(no uint16, topic string) {
	mftFrame := mft.BuildMftEndFrame(no).Encode()
	log.Tracef("send mft end: %d bytes", len(mftFrame))
	m.worker.Publish(topic, mftFrame)
}

func (m *MqttCp) mftTransmit(payload []byte, no uint16, topic string) error {
	mftF, err := mft.BuildMftFrame(no, payload)
	if err != nil {
		return err
	}
	mftFrame := mftF.Encode()
	log.Tracef("send mft transmission: %d) %d bytes", no, len(mftFrame))
	m.worker.Publish(topic, mftFrame)
	return nil
}

func (m *MqttCp) Transmit(msg MqttJsonCp) error {
	msg.Ts = time.Now().UnixMilli()
	return m.transmit(msg)
}

func (m *MqttCp) transmit(msg MqttJsonCp) error {

	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	encodedString := base64.StdEncoding.EncodeToString(b)
	m.worker.Publish(m.txTopic, encodedString)
	return nil
}

func (m *MqttCp) onBrokerData(client MQTT.Client, msg MQTT.Message) {

	rawDecoded := decodeData(msg.Payload())
	jData := MqttJsonCp{}
	err := json.Unmarshal(rawDecoded, &jData)
	if err != nil {
		log.Errorf("unmarshall error: %s", err.Error())
	} else {
		if m.Cb != nil {
			m.Cb(jData)
		}
	}
}

type MqttCpOption func(*MqttCp)

func WithOptionMqttWorker(worker *mqtt.Worker) MqttCpOption {
	return func(h *MqttCp) {
		h.worker = worker
	}
}

func NewCp(mqttOpts *MQTT.ClientOptions, rxTopic string, txtopic string, opts ...MqttCpOption) *MqttCp {

	w := mqtt.NewWorker(mqttOpts, true, nil)
	m := MqttCp{worker: w, rxTopic: rxTopic, txTopic: txtopic, isRunning: false, handshakeTimeout: defaultHandshakeTimeout}

	for _, opt := range opts {
		// Call the option giving the instantiated
		// *House as the argument
		opt(&m)
	}

	m.worker.AddConnectionCB(
		func(status mqtt.ConnectionStatus) {
			switch status {
			case mqtt.ConnectionStatus_Connected:
				{
					m.worker.Subscribe(m.rxTopic, m.onBrokerData)
				}
			case mqtt.ConnectionStatus_Disconnected:
				{
					//do stuff
				}
			}
		},
	)

	if m.worker.IsConnected() {
		m.worker.Subscribe(m.rxTopic, m.onBrokerData)
	}

	return &m
}

func (m *MqttCp) Start() {
	m.isRunning = true
	m.worker.StartMQTT()
}

func (m *MqttCp) IsRunning() bool {
	return m.isRunning
}

func (m *MqttCp) Stop() {
	m.worker.Unsubscribe(m.rxTopic)
	m.isRunning = false
	m.worker.StopMQTT()
}

func (m *MqttCp) mftReceiveFile(f *os.File, inboundChan chan []byte) error {
	writer := bufio.NewWriter(f)
	ready := false
	lastFrameTs := time.Now()
	var frameRef uint16 = 0
	timeoutMftTransfer := mft.MFT_FRAME_TIMEOUT()
	ticker := time.NewTicker(timeoutMftTransfer)
	for {
		select {
		case b := <-inboundChan:
			lastFrameTs = time.Now()
			frame, errM := mft.DecodeMftFrame(b)
			if errM != nil {
				return errM
			}
			fType := frame.GetFrameType()
			switch fType {
			case mft.MftFrameType_START:
				ready = true
				frameRef = frame.GetFrameNo()
			case mft.MftFrameType_TRANSMISSION, mft.MftFrameType_END:
				if ready {
					frameRef++
					if frameRef == frame.GetFrameNo() {
						if fType == mft.MftFrameType_TRANSMISSION {
							_, errW := writer.Write(frame.GetPayload())
							if errW != nil {
								return errW
							}
						} else {
							writer.Flush()
							return nil
						}
					} else {
						return errors.New("wrong frame order")
					}
				} else {
					return errors.New("missing start frame")
				}
			}
		case <-ticker.C:
			if time.Now().Sub(lastFrameTs) > timeoutMftTransfer {
				return errors.New("timeout on reception")
			}
		}
	}
}

func (m *MqttCp) receiveFileAndCheck(f *os.File, inChan chan []byte, md5Expected string, sizeExpected int64) error {
	fName := f.Name()
	errReception := m.mftReceiveFile(f, inChan)
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
		return errors.New(fmt.Sprintf("fail check actual md5 %s, expected: %s", md5, md5Expected))
	}
	realName := strings.TrimSuffix(f.Name(), ".tmp")
	return os.Rename(fName, realName)
}

func (m *MqttCp) mftTransmitFile(fileName, transmissionTopic string) error {
	f, errOpen := os.Open(fileName)
	if errOpen != nil {
		return errOpen
	}
	defer f.Close()
	var counter uint16 = 0
	reader := bufio.NewReader(f)
	mftSize := mft.MFT_PAYLOAD_SIZE()
	buf := make([]byte, mftSize)
	m.mftTransmitStart(counter, transmissionTopic)
	for {
		counter++
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				return err
			}
			m.mftTransmitEnd(counter, transmissionTopic)
			break
		}
		errT := m.mftTransmit(buf[:n], counter, transmissionTopic)
		if errT != nil {
			return errT
		}
		time.Sleep(mft.MFT_FRAME_DELAY())
	}
	return nil
}
