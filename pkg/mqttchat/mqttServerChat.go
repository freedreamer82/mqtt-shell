package mqttchat

import (
	"fmt"
	"log"
	"sync"

	shell "github.com/freedreamer82/mqtt-shell/internal/pkg/shellcmd"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const pluginCmdPrefix = "plugin"
const outputMsgSize = 10000

type MqttServerChat struct {
	*MqttChat
	plugins    []MqttSeverChatPlugin
	pluginMap  sync.Map
	outputChan chan OutMessage
}

func (m *MqttServerChat) OnDataRx(data MqttJsonData) {

	if data.CmdUUID == "" || data.Cmd == "" || data.Data == "" {
		return
	}
	str := fmt.Sprintf("%s", data.Data)
	if str != "" {

		isPlugin, args, argsNo := m.isPluginConfigCmd(str)
		if isPlugin && data.ClientUUID != "" {
			res, p := m.handlePluginConfigCmd(data.ClientUUID, args, argsNo)
			m.outputChan <- NewOutMessageWithPrompt(res, data.ClientUUID, data.CmdUUID, p)
			return
		}

		pluginId, hasPluginActive := m.hasActivePlugin(data.ClientUUID)
		if hasPluginActive {
			m.execPluginCommand(pluginId, data)
			return
		}

		out := m.execShellCommand(str)
		m.outputChan <- NewOutMessage(out, data.ClientUUID, data.CmdUUID)
	}

}

func (m *MqttServerChat) execShellCommand(cmd string) string {
	err, out := shell.Shellout(cmd, m.timeoutCmdShell)
	if err != nil {
		log.Printf("error: %v\n", err)
	}
	return out
}

func (m *MqttServerChat) GetOutputChan() chan OutMessage {
	return m.outputChan
}

func WithOptionBeaconTopic(topic string, topicRequest string) MqttChatOption {
	return func(h *MqttChat) {
		h.beaconTopic = topic
		h.beaconRequestTopic = topicRequest
	}
}

type ServerTopic struct {
	RxTopic       string
	TxTopic       string
	BeaconRxTopic string
	BeaconTxTopic string
}

type MqttServerChatOption func(*MqttServerChat)

func NewServerChat(mqttOpts *MQTT.ClientOptions, topics ServerTopic, version string, opts ...MqttServerChatOption) *MqttServerChat {
	sc := MqttServerChat{}
	chat := NewChat(mqttOpts, topics.RxTopic, topics.TxTopic, version, WithOptionBeaconTopic(topics.BeaconRxTopic, topics.BeaconTxTopic))
	chat.SetDataCallback(sc.OnDataRx)
	sc.MqttChat = chat
	sc.outputChan = make(chan OutMessage, outputMsgSize)
	for _, opt := range opts {
		opt(&sc)
	}
	go sc.mqttTransmit()
	return &sc
}

type OutMessage struct {
	msg        string
	clientUUID string
	cmdUUID    string
	prompt     string
}

func NewOutMessage(msg, clientUUID, cmdUUID string) OutMessage {
	return OutMessage{msg: msg, clientUUID: clientUUID, cmdUUID: cmdUUID}
}

func NewOutMessageWithPrompt(msg, clientUUID, cmdUUID, prompt string) OutMessage {
	return OutMessage{msg: msg, clientUUID: clientUUID, cmdUUID: cmdUUID, prompt: prompt}
}

func (m *MqttServerChat) mqttTransmit() {
	for {
		select {
		case out := <-m.outputChan:
			outMsg := out.msg
			if outMsg != "" {
				m.TransmitWithPrompt(outMsg, out.cmdUUID, out.clientUUID, out.prompt)
			}
		}
	}
}
