package mqtt

import (
	MQTT "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"time"
)

const MaxClientIdLen = 14
const mqttQOS = 1
const quiescenceMs = 100

type ConnectionStatus string

const ConnectionStatus_Connected = "connected"
const ConnectionStatus_Disconnected = "disconnected"

type ConnectionCallback func(status ConnectionStatus)

type Worker struct {
	client                        MQTT.Client
	mqttOpts                      *MQTT.ClientOptions
	brokerStartConnectTimerEnable bool
	connCb                        []ConnectionCallback
	started                       bool
}

func NewWorker(mqttOpts *MQTT.ClientOptions, timerEnable bool, connectionCb []ConnectionCallback) *Worker {
	return &Worker{mqttOpts: mqttOpts, brokerStartConnectTimerEnable: timerEnable, connCb: connectionCb}
}

func (m *Worker) IsConnected() bool {
	if m.client != nil {
		return m.client.IsConnected()
	}
	return false
}

func (m *Worker) AddConnectionCB(cb ConnectionCallback) {
	m.connCb = append(m.connCb, cb)
}

func (m *Worker) GetOpts() *MQTT.ClientOptions {
	return m.mqttOpts
}

func (m *Worker) Subscribe(topic string, onMessageCb MQTT.MessageHandler) error {
	if topic != "" && onMessageCb != nil {
		log.Infof("Sub topic %s, Qos: %d", topic, mqttQOS)
		if token := m.client.Subscribe(topic, mqttQOS, onMessageCb); token.Error() != nil {
			// Return Error
			return token.Error()
		}
	}
	return nil
}

func (m *Worker) Unsubscribe(topic string) error {
	if topic != "" {
		log.Infof("UnSub topic %s", topic)
		if token := m.client.Unsubscribe(topic); token.Error() != nil {
			// Return Error
			return token.Error()
		}
	}
	return nil
}

func (m *Worker) GetMqttClient() MQTT.Client {
	return m.client
}

func (m *Worker) StartMQTT() {
	if m.started {
		return
	}
	m.started = true
	if m.mqttOpts.ClientID == "" {
		m.mqttOpts.SetClientID(getRandomClientId())
	}

	m.mqttOpts.SetAutoReconnect(true)

	//m.mqttOpts.SetConnectRetry(true)
	m.mqttOpts.SetMaxReconnectInterval(45 * time.Second)
	m.mqttOpts.SetConnectionLostHandler(m.onBrokerDisconnect)
	m.mqttOpts.SetOnConnectHandler(m.onBrokerConnect)

	if m.client == nil {
		client := MQTT.NewClient(m.mqttOpts)
		m.client = client
	}
	m.brokerStartConnect()
}

func (m *Worker) StopMQTT() {

	if m.client != nil {
		m.client.Disconnect(quiescenceMs)
	}
	m.client = nil
}

func (m *Worker) brokerStartConnect() {

	//on first connection library doesn't retry...do it manually
	go func(m *Worker) {
		m.client.Connect()
		retry := time.NewTicker(30 * time.Second)
		defer retry.Stop()

		for {
			select {
			case <-retry.C:

				if m.brokerStartConnectTimerEnable {
					if !m.client.IsConnected() {
						if token := m.client.Connect(); token.Wait() && token.Error() != nil {
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
	}(m)
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

func (m *Worker) onBrokerConnect(client MQTT.Client) {

	for _, cb := range m.connCb {
		if cb != nil {
			cb(ConnectionStatus_Connected)
		}
	}

	log.Debug("BROKER connected!")
}

func (m *Worker) onBrokerDisconnect(client MQTT.Client, err error) {

	for _, cb := range m.connCb {
		if cb != nil {
			cb(ConnectionStatus_Disconnected)
		}
	}

	log.Debug("BROKER disconnected !", err)
}

func (m *Worker) Publish(topic string, payload interface{}) {
	if m.client != nil && payload != nil {
		m.client.Publish(topic, mqttQOS, false, payload)
	} else {
		log.Warnf("Publish fallita: client MQTT nullo o payload nullo (topic: %s)", topic)
	}
}
