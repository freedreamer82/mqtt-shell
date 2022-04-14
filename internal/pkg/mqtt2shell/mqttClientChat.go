package mqtt

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

const prompt = ">"
const login = "-------------------------------------------------\r\n|  Mqtt-shell client \r\n|\r\n|  IP: %s \r\n|  SERVER VER: %s - CLIENT VER: %s\r\n|  TX: %s\r\n|  RX: %s\r\n|\r\n-------------------------------------------------\r\n"

type MqttClientChat struct {
	*MqttChat
	waitServerChan chan bool
}

func (m *MqttClientChat) OnDataRx(data MqttJsonData) {

	if data.Uuid == "" || data.Cmd == "" || data.Data == "" {
		return
	}
	out := strings.TrimSuffix(data.Data, "\n") // remove newline
	fmt.Print(out)
	fmt.Println()
	m.printPrompt()
}

func (m *MqttClientChat) waitServerCb(data MqttJsonData) {

	if data.Uuid == "" || data.Cmd != "shell" || data.Data == "" {
		return
	}
	m.waitServerChan <- true
	ip := data.Ip
	serverVersion := data.Version
	m.printLogin(ip, serverVersion)
}

func (m *MqttClientChat) printPrompt() {
	fmt.Print(prompt)
}

func (m *MqttClientChat) printLogin(ip string, serverVersion string) {
	log.Info("Connected")
	fmt.Printf(login, ip, serverVersion, m.version, m.txTopic, m.rxTopic)
	m.printPrompt()
}

func (m *MqttClientChat) waitServer() {
	m.SetDataCallback(m.waitServerCb)
	for {
		log.Info("Connecting to server...")
		m.Transmit("whoami", "")
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
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				m.printPrompt()
			} else {
				m.Transmit(line, "")
			}
		}
	}
}

func NewClientChat(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string, version string, opts ...MqttChatOption) *MqttClientChat {

	cc := MqttClientChat{}
	chat := NewChat(mqttOpts, rxTopic, txTopic, version, opts...)
	chat.SetDataCallback(cc.OnDataRx)
	cc.MqttChat = chat
	cc.waitServerChan = make(chan bool)
	go cc.clientTask()

	return &cc
}
