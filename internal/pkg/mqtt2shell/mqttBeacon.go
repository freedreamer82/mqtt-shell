package mqtt

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

type BeaconDiscovery struct {
	mqttClient          MQTT.Client
	mqttOpts            *MQTT.ClientOptions
	beaconRequestTopic  string
	beaconResponseTopic string
	timeout             time.Duration
	closeChan           chan bool
	converter           NodeIdFromTopic
}

type NodeIdFromTopic func(string) string

func NewBeaconDiscovery(mqttOpts *MQTT.ClientOptions, beaconRequestTopic string, beaconResponseTopic string, timeoutDiscoverySec uint64, converter NodeIdFromTopic) *BeaconDiscovery {
	rand.Seed(time.Now().UnixNano())

	timeout := time.Duration(timeoutDiscoverySec * uint64(time.Second))

	b := BeaconDiscovery{mqttOpts: mqttOpts, beaconRequestTopic: beaconRequestTopic, beaconResponseTopic: beaconResponseTopic, timeout: timeout, converter: converter}

	if b.mqttOpts.ClientID == "" {
		b.mqttOpts.SetClientID(getRandomClientId())
	}

	b.closeChan = make(chan bool)

	b.mqttOpts.SetConnectionLostHandler(b.onBrokerDisconnect)
	b.mqttOpts.SetOnConnectHandler(b.onBrokerConnect)

	b.mqttClient = MQTT.NewClient(b.mqttOpts)

	return &b
}

func (b *BeaconDiscovery) Run() {

	b.brokerStartConnect()

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
			log.Infoln("Stop beacon discovery")
			b.mqttClient.Disconnect(100)
		}
	}
}

func (b *BeaconDiscovery) onBrokerConnect(client MQTT.Client) {
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
		fmt.Printf("Ip: %15s - Id: %20s - Version: %10s - Time: %s\r\n", jData.Ip, nodeId, jData.Version, jData.Datetime)
	}

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
				if !b.mqttClient.IsConnected() {
					if token := b.mqttClient.Connect(); token.Wait() && token.Error() != nil {
						log.Info("failed connection to broker retrying...")
					} else {
						return
					}
				} else {
					return
				}
			}
		}
	}(b)
}
