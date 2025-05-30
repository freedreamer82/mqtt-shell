package mqttchat

import (
	"encoding/json"
	"fmt"
	"github.com/freedreamer82/mqtt-shell/pkg/mqtt"
	"math/rand"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

type Client struct {
	Id      string
	Ip      string
	Version string
	Time    string
	Uptime  string
}

type BeaconDiscovery struct {
	mqttClient          MQTT.Client
	mqttOpts            *MQTT.ClientOptions
	beaconRequestTopic  string
	beaconResponseTopic string
	timeout             time.Duration
	closeChan           chan bool
	converter           NodeIdFromTopic
	clients             chan Client
	cb                  mqtt.ConnectionCallback
	timerCheckEnabled   bool
}

type BeaconDiscoveryOption func(*BeaconDiscovery)
type NodeIdFromTopic func(string) string

func WithDiscoveryConnectionCallback(cb mqtt.ConnectionCallback) BeaconDiscoveryOption {
	return func(h *BeaconDiscovery) {
		h.cb = cb
	}
}

func WithDiscoveryMqttClient(client MQTT.Client) BeaconDiscoveryOption {
	return func(h *BeaconDiscovery) {
		h.mqttClient = client
	}
}

func NewBeaconDiscovery(mqttOpts *MQTT.ClientOptions,
	beaconRequestTopic string, beaconResponseTopic string, timeoutDiscoverySec uint64,
	converter NodeIdFromTopic, opts ...BeaconDiscoveryOption) *BeaconDiscovery {
	rand.Seed(time.Now().UnixNano())

	timeout := time.Duration(timeoutDiscoverySec * uint64(time.Second))

	b := BeaconDiscovery{mqttOpts: mqttOpts, cb: nil,
		beaconRequestTopic: beaconRequestTopic, beaconResponseTopic: beaconResponseTopic, timeout: timeout, converter: converter, timerCheckEnabled: true}

	b.closeChan = make(chan bool)

	b.mqttOpts.SetConnectionLostHandler(b.onBrokerDisconnect)
	b.mqttOpts.SetOnConnectHandler(b.onBrokerConnect)

	for _, opt := range opts {
		// Call the option giving the instantiated
		opt(&b)
	}

	if b.mqttClient == nil {
		b.mqttClient = MQTT.NewClient(b.mqttOpts)

	}

	return &b
}

func (b *BeaconDiscovery) Run(ch chan Client) {

	b.brokerStartConnect()

	b.clients = ch

	start := time.Now()

	go func() {
		for {
			time.Sleep(time.Second)
			now := time.Now()
			if now.Sub(start) > b.timeout {
				b.closeChan <- true
			}
		}
	}()

	select {
	case <-b.closeChan:
		{
			b.timerCheckEnabled = false
			log.Infoln("Stop beacon discovery")
			b.mqttClient.Disconnect(100)
		}
	}
}

func (b *BeaconDiscovery) onBrokerConnect(client MQTT.Client) {
	if b.cb != nil {
		b.cb(mqtt.ConnectionStatus_Connected)
	}
	log.Debugln("Connect to broker")
	err := b.subscribeMessagesToBroker()
	if err != nil {
		log.Error("error in subscription")
	}
	b.sendBeaconRequest()
}

func (b *BeaconDiscovery) onBrokerDisconnect(client MQTT.Client, err error) {
	log.Debug("BROKER disconnected !", err)
	b.closeChan <- true
	if b.cb != nil {
		b.cb(mqtt.ConnectionStatus_Disconnected)
	}
}

func (b *BeaconDiscovery) subscribeMessagesToBroker() error {
	client := b.mqttClient
	if b.beaconResponseTopic != "" {
		// Go For MQTT Publish
		log.Printf("Sub topic %s, Qos: %d", b.beaconResponseTopic, 0)
		if token := client.Subscribe(b.beaconResponseTopic, 0, b.onBeaconDiscovery); token.Error() != nil {
			// Return Error
			return token.Error()
		}
	}
	return nil
}

func (b *BeaconDiscovery) onBeaconDiscovery(client MQTT.Client, msg MQTT.Message) {
	if b.converter == nil {
		log.Errorln("Node Id converter nil ?")
	} else {
		nodeId := b.converter(msg.Topic())
		jData := MqttJsonData{}
		err := json.Unmarshal(msg.Payload(), &jData)
		if err != nil {
			log.Errorln("error deserializing message")
		}
		c := Client{Id: nodeId, Ip: jData.Ip, Version: jData.Version,
			Time: jData.Datetime, Uptime: jData.Data}

		if b.clients != nil {
			b.clients <- c
		}

		uptimeDuration, err := time.ParseDuration(jData.Data)
		if err != nil {
			log.Errorln("error parsing uptime duration:", err)
			return
		}
		formattedUptime := fmt.Sprintf("%d days %02d:%02d", int(uptimeDuration.Hours())/24, int(uptimeDuration.Hours())%24, int(uptimeDuration.Minutes())%60)
		fmt.Printf("Ip: %15s - Id: %20s - Version: %10s - Time: %s - Uptime: %s \r\n", jData.Ip, nodeId, jData.Version, jData.Datetime, formattedUptime)	}

}

func (b *BeaconDiscovery) sendBeaconRequest() {
	if b.beaconRequestTopic != "" {
		b.mqttClient.Publish(b.beaconRequestTopic, 0, false, []byte{})
	}
}

func (b *BeaconDiscovery) brokerStartConnect() {

	//on first connection library doesn't retry...do it manually
	go func(b *BeaconDiscovery) {
		b.mqttClient.Connect()
		retry := time.NewTicker(30 * time.Second)
		defer retry.Stop()

		for {
			select {
			case <-retry.C:
				if b.timerCheckEnabled {
					if !b.mqttClient.IsConnected() {
						if token := b.mqttClient.Connect(); token.Wait() && token.Error() != nil {
							log.Info("failed connection to broker retrying...")
						}
					} else {
						return
					}
				} else {
					return
				}
			}
		}
		fmt.Println("exit from check timer mqtt..")
	}(b)

}
