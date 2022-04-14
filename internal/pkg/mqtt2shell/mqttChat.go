package mqtt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"time"

	log "github.com/sirupsen/logrus"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/lithammer/shortuuid/v3"
)

const MaxClientIdLen = 14
const defaultTimeoutCmd = 5 * time.Second

type SubScribeMessage struct {
	Topic string
	Qos   byte
}

type MqttJsonData struct {
	Ip       string `json:"ip"`
	Version  string `json:"version"`
	Cmd      string `json:"cmd"`
	Data     string `json:"data"`
	Uuid     string `json:"uuid"`
	Datetime string `json:"datetime"`
}

type OnDataCallack func(data MqttJsonData)

type MqttChat struct {
	mqttClient         MQTT.Client
	mqttOpts           *MQTT.ClientOptions
	timeoutCmdShell    time.Duration
	Cb                 OnDataCallack
	txTopic            string
	rxTopic            string
	beaconTopic        string
	beaconRequestTopic string
	version            string
}

func (m *MqttChat) SetDataCallback(cb OnDataCallack) {
	m.Cb = cb
}

func (m *MqttChat) getIpAddress() string {
	// conn, err := net.Dial("udp", "8.8.8.8:80")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer conn.Close()

	// localAddr := conn.LocalAddr().(*net.UDPAddr)

	// return localAddr.IP.String()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""

}

func getRandomClientId() string {
	var characterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, MaxClientIdLen)
	for i := range b {
		b[i] = characterRunes[rand.Intn(len(characterRunes))]
	}
	id := "mqtt-shell-" + string(b)
	log.Debug("ID: ", id)
	return id
}

func (m *MqttChat) subscribeMessagesToBroker() error {
	client := m.mqttClient
	if m.rxTopic != "" {
		// Go For MQTT Publish
		log.Printf("Sub topic %s, Qos: %d\r\n", m.rxTopic, 0)
		if token := client.Subscribe(m.rxTopic, 0, m.onBrokerData); token.Error() != nil {
			// Return Error
			return token.Error()
		}
	}
	if m.beaconRequestTopic != "" {
		// Go For MQTT Publish
		log.Printf("Sub topic %s, Qos: %d\r\n", m.beaconRequestTopic, 0)
		if token := client.Subscribe(m.beaconRequestTopic, 0, m.onBeaconRequest); token.Error() != nil {
			// Return Error
			return token.Error()
		}
	}
	return nil
}

func (m *MqttChat) Transmit(out string, uuid string) {

	if uuid == "" {
		//generate one random..
		uuid = shortuuid.New()
	}

	go func() {
		now := time.Now().String()
		reply := MqttJsonData{Ip: m.getIpAddress(), Version: m.version, Data: out, Cmd: "shell", Datetime: now, Uuid: uuid}

		b, err := json.Marshal(reply)
		if err != nil {
			fmt.Println(err)
			return
		}
		// fmt.Print(string(b))

		encodedString := base64.StdEncoding.EncodeToString(b)

		m.mqttClient.Publish(m.txTopic, 0, false, encodedString)
	}()
}

func decodeData(dataraw []byte) []byte {

	var data string = string(dataraw)
	rawdecoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		log.Error("decode error:", err)
		return nil
	}
	return rawdecoded
}

func (m *MqttChat) onBrokerData(client MQTT.Client, msg MQTT.Message) {

	rawdecoded := decodeData(msg.Payload())
	// var jsondata map[string]interface{}
	// if err := json.Unmarshal(rawdecoded, &jsondata); err != nil {
	// 	panic(err)
	// }
	jdata := MqttJsonData{}
	err := json.Unmarshal(rawdecoded, &jdata)
	if err != nil {

	}
	//fmt.Println(jsondata)

	if m.Cb != nil {
		m.Cb(jdata)
	}
}

func (m *MqttChat) onBeaconRequest(client MQTT.Client, msg MQTT.Message) {

	m.sendBeacon()
}

func (m *MqttChat) sendBeacon() {
	if m.beaconTopic != "" {
		now := time.Now().String()
		reply := MqttJsonData{Ip: m.getIpAddress(), Version: m.version, Cmd: "beacon", Datetime: now, Uuid: ""}

		b, err := json.Marshal(reply)
		if err != nil {
			fmt.Println(err)
			return
		}
		m.mqttClient.Publish(m.beaconTopic, 0, false, b)
	}
}

func (m *MqttChat) onBrokerConnect(client MQTT.Client) {
	log.Debug("BROKER connected!")
	m.subscribeMessagesToBroker()
	m.sendBeacon()
}

func (m *MqttChat) onBrokerDisconnect(client MQTT.Client, err error) {
	log.Debug("BROKER disconnecred !", err)
}

func (m *MqttChat) setupMQTT() {
	if m.mqttOpts.ClientID == "" {
		m.mqttOpts.SetClientID(getRandomClientId())
	}

	m.mqttOpts.SetAutoReconnect(true)

	//m.mqttOpts.SetConnectRetry(true)
	m.mqttOpts.SetMaxReconnectInterval(45 * time.Second)
	m.mqttOpts.SetConnectionLostHandler(m.onBrokerDisconnect)
	m.mqttOpts.SetOnConnectHandler(m.onBrokerConnect)

	client := MQTT.NewClient(m.mqttOpts)
	m.mqttClient = client
	m.brokerStartConnect()
}

func (m *MqttChat) brokerStartConnect() {

	//on first connection library doesn't retry...do it manually
	go func(m *MqttChat) {
		m.mqttClient.Connect()
		retry := time.NewTicker(30 * time.Second)
		defer retry.Stop()

		for {
			select {
			case <-retry.C:
				if !m.mqttClient.IsConnected() {
					if token := m.mqttClient.Connect(); token.Wait() && token.Error() != nil {
						log.Info("failed connection to broker retrying...")
					} else {
						return
					}
				} else {
					return
				}
			}
		}
	}(m)
}

type MqttChatOption func(*MqttChat)

func WithOptionTimeoutCmd(timeout time.Duration) MqttChatOption {
	return func(h *MqttChat) {
		h.timeoutCmdShell = timeout
	}
}

func NewChat(mqttOpts *MQTT.ClientOptions, rxTopic string, txtopic string, version string, opts ...MqttChatOption) *MqttChat {
	rand.Seed(time.Now().UnixNano())

	m := MqttChat{mqttOpts: mqttOpts, rxTopic: rxTopic, txTopic: txtopic, version: version, beaconTopic: "", Cb: nil}

	m.timeoutCmdShell = defaultTimeoutCmd
	for _, opt := range opts {
		// Call the option giving the instantiated
		// *House as the argument
		opt(&m)
	}

	m.setupMQTT()

	return &m
}
