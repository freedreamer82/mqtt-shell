package mqttchat

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/freedreamer82/mqtt-shell/pkg/mqtt"

	log "github.com/sirupsen/logrus"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/lithammer/shortuuid/v3"
)

const defaultTimeoutCmd = 5 * time.Second

type SubScribeMessage struct {
	Topic string
	Qos   byte
}

type MqttJsonData struct {
	Ip           string `json:"ip"`
	Version      string `json:"version"`
	Cmd          string `json:"cmd"`
	Data         string `json:"data"`
	CmdUUID      string `json:"cmduuid"`
	ClientUUID   string `json:"clientuuid"`
	Datetime     string `json:"datetime"`
	CustomPrompt string `json:"customprompt"`
	Flags        uint32 `json:"flags"`
}

type OnDataCallback func(data MqttJsonData)

type MqttChat struct {
	worker             *mqtt.Worker
	timeoutCmdShell    time.Duration
	Cb                 OnDataCallback
	txTopic            string
	rxTopic            string
	beaconTopic        string
	beaconRequestTopic string
	version            string
	startTime          time.Time
	isRunning          bool
	netInterface       string
}

func (m *MqttChat) SetDataCallback(cb OnDataCallback) {
	m.Cb = cb
}

func getInterfaceIpv4Addr(interfaceName string) (addr string, err error) {
	var (
		ief      *net.Interface
		addrs    []net.Addr
		ipv4Addr net.IP
	)
	if ief, err = net.InterfaceByName(interfaceName); err != nil { // get interface
		return
	}
	if addrs, err = ief.Addrs(); err != nil { // get addresses
		return
	}
	for _, addr := range addrs { // get ipv4 address
		if ipv4Addr = addr.(*net.IPNet).IP.To4(); ipv4Addr != nil {
			break
		}
	}
	if ipv4Addr == nil {
		return "", errors.New(fmt.Sprintf("interface %s don't have an ipv4 address\n", interfaceName))
	}
	return ipv4Addr.String(), nil
}

func (m *MqttChat) getIpAddress() string {

	if m.netInterface != "" {
		addr, _ := getInterfaceIpv4Addr(m.netInterface)
		return addr
	}

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

//func (m *MqttChat) subscribeMessagesToBroker() error {
//	client := m.worker.GetMqttClient()
//	if m.rxTopic != "" {
//		// Go For MQTT Publish
//		log.Infof("Sub topic %s, Qos: %d", m.rxTopic, mqttQOS)
//		if token := client.Subscribe(m.rxTopic, mqttQOS, m.onBrokerData); token.Error() != nil {
//			// Return Error
//			return token.Error()
//		}
//	}
//	if m.beaconRequestTopic != "" {
//		// Go For MQTT Publish
//		log.Infof("Sub topic %s, Qos: %d", m.beaconRequestTopic, mqttQOS)
//		if token := client.Subscribe(m.beaconRequestTopic, mqttQOS, m.onBeaconRequest); token.Error() != nil {
//			// Return Error
//			return token.Error()
//		}
//	}
//	return nil
//}
//
//func (m *MqttChat) unsubscribeMessagesToBroker() error {
//	client := m.worker.GetMqttClient()
//	if m.rxTopic != "" {
//		// Go For MQTT Publish
//		log.Infof("UnSub topic %s, Qos: %d", m.rxTopic, mqttQOS)
//		if token := client.Unsubscribe(m.rxTopic); token.Error() != nil {
//			// Return Error
//			return token.Error()
//		}
//	}
//	if m.beaconRequestTopic != "" {
//		// Go For MQTT Publish
//		log.Infof("UnSub topic %s, Qos: %d", m.beaconRequestTopic, mqttQOS)
//		if token := client.Unsubscribe(m.beaconRequestTopic); token.Error() != nil {
//			// Return Error
//			return token.Error()
//		}
//	}
//	return nil
//}

func (m *MqttChat) transmit(out string, cmdUuid string, clientUuid string, customPrompt string, flags uint32) {

	if cmdUuid == "" {
		//generate one random..
		cmdUuid = shortuuid.New()
	}

	now := time.Now().Format(time.DateTime)
	reply := MqttJsonData{Ip: m.getIpAddress(), Version: m.version, Data: out, Cmd: "shell", Datetime: now, CmdUUID: cmdUuid,
		ClientUUID: clientUuid, CustomPrompt: customPrompt, Flags: flags}

	b, err := json.Marshal(reply)
	if err != nil {
		fmt.Println(err)
		return
	}

	encodedString := base64.StdEncoding.EncodeToString(b)
	m.worker.Publish(m.txTopic, encodedString)

}

func (m *MqttChat) TransmitWithFlags(out string, cmdUuid string, clientUuid string, flags uint32) {
	m.transmit(out, cmdUuid, clientUuid, "", 0)
}

func (m *MqttChat) TransmitWithPrompt(out string, cmdUuid string, clientUuid string, customPrompt string) {
	m.transmit(out, cmdUuid, clientUuid, customPrompt, 0)
}

func (m *MqttChat) Transmit(out string, cmdUuid string, clientUuid string) {
	m.transmit(out, cmdUuid, clientUuid, "", 0)
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

		now := time.Now().Format(time.DateTime)
		fromNow := fmtDuration(m.uptime())
		reply := MqttJsonData{Ip: m.getIpAddress(), Version: m.version, Cmd: "beacon", Datetime: now, Data: fromNow}

		b, err := json.Marshal(reply)
		if err != nil {
			fmt.Println(err)
			return
		}
		m.worker.Publish(m.beaconTopic, b)
	}
}

type MqttChatOption func(*MqttChat)

func WithOptionTimeoutCmd(timeout time.Duration) MqttChatOption {
	return func(h *MqttChat) {
		h.timeoutCmdShell = timeout
	}
}

func (m *MqttChat) uptime() time.Duration {
	return time.Since(m.startTime)
}

func fmtDuration(d time.Duration) string {
	//d = d.Round(time.Minute)
	//h := d / time.Hour
	//d -= h * time.Hour
	//m := d / time.Minute
	//return fmt.Sprintf("%02d:%02d", h, m)
	return fmt.Sprintf("%s", d)
}

func NewChat(mqttOpts *MQTT.ClientOptions, rxTopic string, txtopic string, version string, opts ...MqttChatOption) *MqttChat {
	rand.Seed(time.Now().UnixNano())

	w := mqtt.NewWorker(mqttOpts, true, nil)
	m := MqttChat{worker: w, rxTopic: rxTopic, txTopic: txtopic, version: version,
		beaconTopic: "", Cb: nil, isRunning: false, netInterface: ""}

	m.startTime = time.Now()
	m.timeoutCmdShell = defaultTimeoutCmd
	for _, opt := range opts {
		// Call the option giving the instantiated
		// *House as the argument
		opt(&m)
	}

	m.worker.SetConnectionCB(
		func(status mqtt.ConnectionStatus) {
			switch status {
			case mqtt.ConnectionStatus_Connected:
				{
					m.worker.Subscribe(m.rxTopic, m.onBrokerData)
					m.worker.Subscribe(m.beaconRequestTopic, m.onBeaconRequest)
					m.sendBeacon()
				}
			case mqtt.ConnectionStatus_Disconnected:
				{
					//do stuff
				}
			}
		},
	)

	return &m
}

func (m *MqttChat) Start() {
	m.isRunning = true
	m.worker.StartMQTT()
}

func (m *MqttChat) IsRunning() bool {
	return m.isRunning
}

func (m *MqttChat) Stop() {
	m.worker.Unsubscribe(m.rxTopic)
	m.worker.Unsubscribe(m.beaconRequestTopic)
	m.isRunning = false
	m.worker.StopMQTT()
}
