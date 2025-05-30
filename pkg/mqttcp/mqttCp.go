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

func (m *MqttCp) mftReceiveFile(f *os.File, inboundChan chan []byte, progress *chan mft.MftProgress) error {
	writer := bufio.NewWriter(f)
	ready := false
	lastFrameTs := time.Now()
	var frameRef uint16 = 0
	var framesLeft uint16 = 0
	var frameTotal uint32 = 0
	var frameReceived uint32 = 0
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
				framesLeft = frameRef
				frameTotal = uint32(frameRef)
				frameReceived = 0
				if progress != nil {
					*progress <- mft.MftProgress{
						FrameTotal:    frameTotal,
						FrameReceived: frameReceived,
						Percent:       0,
					}
				}
			case mft.MftFrameType_TRANSMISSION:
				if ready {
					frameNo := frame.GetFrameNo()
					if frameNo != framesLeft {
						return errors.New("wrong frame order")
					}
					framesLeft--
					frameReceived++
					_, errW := writer.Write(frame.GetPayload())
					if errW != nil {
						return errW
					}
					if progress != nil && frameTotal > 0 {
						*progress <- mft.MftProgress{
							FrameTotal:    frameTotal,
							FrameReceived: frameReceived,
							Percent:       float32(frameReceived) / float32(frameTotal) * 100,
						}
					}
				} else {
					return errors.New("missing start frame")
				}
			case mft.MftFrameType_END:
				if ready {
					if frame.GetFrameNo() != 0 {
						return errors.New("frame END con numero diverso da 0")
					}
					writer.Flush()
					if progress != nil && frameTotal > 0 {
						*progress <- mft.MftProgress{
							FrameTotal:    frameTotal,
							FrameReceived: frameTotal,
							Percent:       100,
						}
					}
					return nil
				} else {
					return errors.New("missing start frame")
				}
			}
		case <-ticker.C:
			if time.Since(lastFrameTs) > timeoutMftTransfer {
				return errors.New("timeout on reception")
			}
		}
	}
}

func (m *MqttCp) receiveFileAndCheck(f *os.File, inChan chan []byte, md5Expected string, sizeExpected int64, progress *chan mft.MftProgress) error {
	fName := f.Name()
	errReception := m.mftReceiveFile(f, inChan, progress)
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

func (m *MqttCp) mftTransmitFile(fileName, transmissionTopic string, progress *chan mft.MftProgress) error {
	f, errOpen := os.Open(fileName)
	if errOpen != nil {
		return errOpen
	}
	defer f.Close()

	fileInfo, errStat := f.Stat()
	if errStat != nil {
		return errStat
	}
	fileSize := fileInfo.Size()
	mftSize := mft.MFT_PAYLOAD_SIZE()
	numFrames := int((fileSize + int64(mftSize) - 1) / int64(mftSize)) // round up

	buf := make([]byte, mftSize)

	// Invia il frame di start con il numero totale di frame
	m.mftTransmitStart(uint16(numFrames), transmissionTopic)

	for frameIdx := 0; frameIdx < numFrames; frameIdx++ {
		offset := int64(frameIdx) * int64(mftSize)
		_, errSeek := f.Seek(offset, io.SeekStart)
		if errSeek != nil {
			return errSeek
		}
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}

		frameNo := uint16(numFrames - frameIdx)
		errT := m.mftTransmit(buf[:n], frameNo, transmissionTopic)
		if errT != nil {
			return errT
		}

		if progress != nil {
			*progress <- mft.MftProgress{
				FrameTotal:    uint32(numFrames),
				FrameReceived: uint32(frameIdx + 1),
				Percent:       float32(frameIdx+1) / float32(numFrames) * 100,
			}
		}

		time.Sleep(mft.MFT_FRAME_DELAY())
	}

	// Invia il frame di end
	m.mftTransmitEnd(0, transmissionTopic)
	return nil
}
