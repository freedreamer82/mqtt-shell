package mqttchat

import (
	"fmt"
	"log"

	shell "github.com/freedreamer82/mqtt-shell/internal/pkg/shellcmd"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

type MqttServerChat struct {
	*MqttChat
}

func (m *MqttServerChat) OnDataRx(data MqttJsonData) {

	if data.Uuid == "" || data.Cmd == "" || data.Data == "" {
		return
	}

	str := fmt.Sprintf("%s", data.Data)
	if str != "" {
		err, out := shell.Shellout(str, m.timeoutCmdShell)
		if err != nil {
			log.Printf("error: %v\n", err)

		} else {
			fmt.Println(out)
		}
		m.Transmit(out, data.Uuid)
	}

}

func WithOptionBeaconTopic(topic string, topicRequest string) MqttChatOption {
	return func(h *MqttChat) {
		h.beaconTopic = topic
		h.beaconRequestTopic = topicRequest
	}
}

func NewServerChat(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string, version string, opts ...MqttChatOption) *MqttServerChat {

	sc := MqttServerChat{}
	chat := NewChat(mqttOpts, rxTopic, txTopic, version, opts...)
	chat.SetDataCallback(sc.OnDataRx)
	sc.MqttChat = chat
	return &sc
}
