package mqttchat

import (
	"bufio"
	"fmt"
	"github.com/lithammer/shortuuid/v3"
	"io"
	"os"
	"strings"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

const prompt = ">"
const login = "-------------------------------------------------\r\n|  Mqtt-shell client \r\n|\r\n|  IP: %s \r\n|  SERVER VER: %s - CLIENT VER: %s\r\n|  CLIENT UUID: %s\r\n|  TX: %s\r\n|  RX: %s\r\n|\r\n-------------------------------------------------\r\n"

type MqttClientChat struct {
	*MqttChat
	waitServerChan chan bool
	ch             chan []byte
	io             ClientChatIO
	uuid           string
}

func (m *MqttClientChat) print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(m.io.Writer, a...)
}

func (m *MqttClientChat) println() (n int, err error) {
	return fmt.Fprintln(m.io.Writer)
}

func (m *MqttClientChat) printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(m.io.Writer, format, a...)
}

func (m *MqttClientChat) printWithoutLn(a ...interface{}) (n int, err error) {
	return fmt.Fprint(m.io.Writer, a...)
}

func (m *MqttClientChat) IsDataInvalid(data MqttJsonData) bool {
	return data.CmdUUID == "" || data.Cmd == "" || data.Data == "" || data.ClientUUID != m.uuid
}

func (m *MqttClientChat) OnDataRx(data MqttJsonData) {

	if m.IsDataInvalid(data) {
		return
	}
	out := strings.TrimSuffix(data.Data, "\n") // remove newline
	m.print(out)
	m.println()
	m.printPrompt()
}

func (m *MqttClientChat) waitServerCb(data MqttJsonData) {

	if m.IsDataInvalid(data) {
		log.Debug()
		return
	}
	m.waitServerChan <- true
	ip := data.Ip
	serverVersion := data.Version
	m.printLogin(ip, serverVersion)
}

func (m *MqttClientChat) printPrompt() {
	m.printWithoutLn(prompt)
}

func (m *MqttClientChat) printLogin(ip string, serverVersion string) {
	log.Info("Connected")
	m.printf(login, ip, serverVersion, m.version, m.uuid, m.txTopic, m.rxTopic)
	m.printPrompt()
}

func (m *MqttClientChat) waitServer() {
	m.SetDataCallback(m.waitServerCb)
	for {
		log.Info("Connecting to server...")
		m.Transmit("whoami", "", m.uuid)
		select {
		case ok := <-m.waitServerChan:
			if ok {
				m.SetDataCallback(m.OnDataRx)
				return
			}
		case <-time.After(5 * time.Second):
			log.Info("TIMEOUT , retry...")
		}
	}

}

func (m *MqttClientChat) clientTask() {
	m.waitServer()
	for {
		scanner := bufio.NewScanner(m.io.Reader)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				m.printPrompt()
			} else {
				m.Transmit(line, "", m.uuid)
			}
		}
	}
}

type ClientChatIO struct {
	io.Reader
	io.Writer
}

func defaultIO() ClientChatIO {
	return struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stdout}
}

func NewClientChat(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string,
	version string, opts ...MqttChatOption) *MqttClientChat {

	cc := MqttClientChat{io: defaultIO(), uuid: shortuuid.New()}
	chat := NewChat(mqttOpts, rxTopic, txTopic, version, opts...)
	chat.SetDataCallback(cc.OnDataRx)
	cc.MqttChat = chat
	cc.waitServerChan = make(chan bool)
	go cc.clientTask()

	return &cc
}

func NewClientChatWithCustomIO(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string, version string,
	customIO ClientChatIO, opts ...MqttChatOption) *MqttClientChat {

	cc := MqttClientChat{io: customIO, uuid: shortuuid.New()}
	chat := NewChat(mqttOpts, rxTopic, txTopic, version, opts...)
	chat.SetDataCallback(cc.OnDataRx)
	cc.MqttChat = chat
	cc.waitServerChan = make(chan bool)
	go cc.clientTask()

	return &cc
}
