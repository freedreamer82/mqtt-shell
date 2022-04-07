package mqtt

import (
	"fmt"
	"log"
	shell "mqtt-shell/internal/pkg/shellcmd"

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

func WithOptionBeaconTopic(topic string) MqttChatOption {
	return func(h *MqttChat) {
		h.beaconTopic = topic
	}
}

func NewServerChat(mqttOpts *MQTT.ClientOptions, rxTopic string, txtopic string, opts ...MqttChatOption) *MqttServerChat {

	sc := MqttServerChat{}
	chat := NewChat(mqttOpts, rxTopic, txtopic, opts...)
	chat.SetDataCallback(sc.OnDataRx)
	sc.MqttChat = chat
	return &sc
}
