package telnetbridge

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/freedreamer82/mqtt-shell/pkg/mqttchat"
	"github.com/reiver/go-telnet"
	log "github.com/sirupsen/logrus"
)

const defaultTelnetBridgePluginName = "telnet"

const flushTimeout = 250 * time.Millisecond
const bufferOutputSize = 512

const timeoutCheckConnection = 10 * time.Second
const timeoutConnection = 1 * time.Minute

func WithTelnetBridge(maxConnections int, keyword string) mqttchat.MqttServerChatOption {
	return func(m *mqttchat.MqttServerChat) {
		telnetBridge := NewTelnetBridgePlugin(maxConnections, keyword, m.GetOutputChan())
		m.AddPlugin(telnetBridge)
	}
}

type TelnetBridge struct {
	pluginName        string
	telnetConnections sync.Map
	maxConnections    int
	chOut             chan mqttchat.OutMessage
	prompt            string
}

func (t *TelnetBridge) PluginId() string {
	return t.pluginName
}

func (t *TelnetBridge) GetPrompt() string {
	if t.prompt == "" {
		t.prompt = t.pluginName
	}
	return t.prompt
}

type mqtt2telnetConnection struct {
	connection      *telnet.Conn
	mqttClientID    string
	lastCommandId   string
	telnetHost      string
	lastCommandTime time.Time
	chClose         chan bool
}

func (t *TelnetBridge) isTelnetBridgeCommand(str string) (bool, []string, int) {
	isBridge := strings.HasPrefix(str, t.pluginName)
	if isBridge {
		cmd := strings.TrimSpace(strings.Replace(str, t.pluginName, "", -1))
		if len(cmd) > 0 {
			split := strings.Split(cmd, " ")
			if len(split) > 0 {
				return true, split, len(split)
			}
		}
	}
	return false, nil, 0
}
func (t *TelnetBridge) GetName() string {
	return "telnet"
}

func (t *TelnetBridge) OnDataRx(data mqttchat.MqttJsonData) {

	if data.CmdUUID == "" || data.Cmd == "" || data.Data == "" || data.ClientUUID == "" {
		return
	}

	str := strings.TrimSpace(fmt.Sprintf("%s", data.Data))

	go func() {

		// telnet bridge conf command
		isBridgeCmd, args, argsLen := t.isTelnetBridgeCommand(str)
		if isBridgeCmd {
			t.post(t.exec(data.ClientUUID, args, argsLen), data.ClientUUID, data.CmdUUID)
			return
		}

		// telnet direct command but not connected
		value, ok := t.telnetConnections.Load(data.ClientUUID)
		if !ok {
			t.post(fmt.Sprintf("SSH plugin connection not established - try: %s help", t.pluginName), data.ClientUUID, data.CmdUUID)
			return
		}

		// telnet direct command
		mqtt2telnet := value.(mqtt2telnetConnection)
		t.updateTimeOnCommand(mqtt2telnet, data.CmdUUID)
		log.Debug(str)
		_, err := mqtt2telnet.connection.Write([]byte(str + "\r\n"))
		if err != nil {
			t.post(err.Error(), data.ClientUUID, data.CmdUUID)
		}

	}()

}

func (t *TelnetBridge) updateTimeOnCommand(mqtt2telnet mqtt2telnetConnection, lastCommandId string) {
	mqtt2telnet.lastCommandId = lastCommandId
	mqtt2telnet.lastCommandTime = time.Now()
	t.telnetConnections.Store(mqtt2telnet.mqttClientID, mqtt2telnet)
}

func (t *TelnetBridge) isClientConnected(mqttClientId string) (bool, string) {
	conn, ok := t.telnetConnections.Load(mqttClientId)
	if ok {
		return ok, conn.(mqtt2telnetConnection).telnetHost
	}
	return ok, ""
}

func (t *TelnetBridge) isHostAlreadyConnected(host string) (bool, string) {
	isConnected := false
	mqttClientId := ""
	t.telnetConnections.Range(func(k, v interface{}) bool {
		connection := v.(mqtt2telnetConnection)
		if connection.telnetHost == host {
			isConnected = true
			mqttClientId = connection.mqttClientID
		}
		return true
	})
	return isConnected, mqttClientId
}

func (t *TelnetBridge) countConnections() int {
	size := 0
	t.telnetConnections.Range(func(k, v interface{}) bool {
		size++
		return true
	})
	return size
}

func (t *TelnetBridge) exec(mqttClientId string, args []string, argsLen int) string {
	if argsLen == 1 && args[0] == "list" {
		res := "Active telnet connections: ... "
		t.telnetConnections.Range(func(k, v interface{}) bool {
			connection := v.(mqtt2telnetConnection)
			res = fmt.Sprintf("%s\r\n%s - %s", res, connection.mqttClientID, connection.telnetHost)
			return true
		})
		return res
	} else if argsLen == 1 && args[0] == "help" {
		return getTelnetHelpText(t.pluginName)
	} else if argsLen == 3 && args[0] == "connect" {
		return t.startTelnetConnection(mqttClientId, args[1], args[2])
	} else if argsLen == 1 && args[0] == "disconnect" {
		go t.stopTelnetConnection(mqttClientId, false)
		return ""
	}
	return getTelnetErrorText(t.pluginName)
}

func (t *TelnetBridge) startTelnetConnection(mqttClientId, host, port string) string {

	addr := fmt.Sprintf("%s:%s", host, port)

	isClientConnected, connectedHost := t.isClientConnected(mqttClientId)
	if isClientConnected {
		return fmt.Sprintf("this client is already connected to %s, disconnect before creating a new connection", connectedHost)
	}

	isHostConnected, connectedMqttClient := t.isHostAlreadyConnected(addr)
	if isHostConnected {
		return fmt.Sprintf("this host is already connected to another mqtt client: %s", connectedMqttClient)
	}

	if t.countConnections() >= t.maxConnections {
		return "max number of connection reached"
	}

	log.Infof("start creating connection - host: %s, clientId: %s", addr, mqttClientId)
	conn, err := telnet.DialTo(addr)
	if err != nil {
		log.Error(err.Error())
		return err.Error()
	}

	mqtt2telnet := mqtt2telnetConnection{
		mqttClientID:    mqttClientId,
		connection:      conn,
		telnetHost:      addr,
		lastCommandTime: time.Now(),
		chClose:         make(chan bool, 1),
	}
	t.telnetConnections.Store(mqttClientId, mqtt2telnet)

	go t.listen(mqtt2telnet)

	return fmt.Sprintf("connection established with %s", addr)

}

func (t *TelnetBridge) stopTelnetConnection(mqttClientId string, isForTimeout bool) {

	value, ok := t.telnetConnections.Load(mqttClientId)
	if !ok {
		res := "connection not found - cant close it"
		log.Error(res)
		t.post(res, mqttClientId, "")
	}

	mqtt2telnet := value.(mqtt2telnetConnection)

	if isForTimeout {
		t.post("connection close due to inactivity", mqttClientId, mqtt2telnet.lastCommandId)
	}

	mqtt2telnet.chClose <- true

	_ = mqtt2telnet.connection.Close()

	t.telnetConnections.Delete(mqttClientId)

	t.post(fmt.Sprintf("connection closed with %s", mqtt2telnet.telnetHost), mqttClientId, mqtt2telnet.lastCommandId)
}

func (t *TelnetBridge) listen(mqtt2telnet mqtt2telnetConnection) {
	if mqtt2telnet.connection != nil {

		log.Info("start lister")

		telnetListenerChan := make(chan byte, 100)

		go func(buffChan chan byte) {
			for {
				buff := make([]byte, 1)
				_, err := mqtt2telnet.connection.Read(buff)

				if err != nil {
					log.Debug("closing routine telnet listener...")
					break
				}

				if buff != nil {
					b := buff[0]
					buffChan <- b
				}
			}
		}(telnetListenerChan)

		go func(telnetListenerChan chan byte) {
			ticker := time.NewTicker(flushTimeout)
			bigBuff := make([]byte, bufferOutputSize)
			flush := func(buff []byte) {
				cp := make([]byte, len(buff))
				copy(cp, buff)
				t.post(string(cp), mqtt2telnet.mqttClientID, mqtt2telnet.lastCommandId)
			}
			for {
				select {
				case b := <-telnetListenerChan:
					bigBuff = append(bigBuff, b)
					ticker.Reset(flushTimeout)
					if len(bigBuff) == bufferOutputSize {
						flush(bigBuff)
						bigBuff = bigBuff[:0]
					}
				case <-ticker.C:
					if len(bigBuff) > 0 {
						flush(bigBuff)
						bigBuff = bigBuff[:0]
					}
				case <-mqtt2telnet.chClose:
					log.Debug("closing routine mqtt writer...")
					return
				}
			}
		}(telnetListenerChan)

	}
}

func (t *TelnetBridge) post(msg, mqttClientId, mqttCmdId string) {
	prompt := t.pluginName
	isConnected, host := t.isClientConnected(mqttClientId)
	if isConnected {
		prompt = fmt.Sprintf("%s - %s", t.pluginName, host)
	}
	t.prompt = prompt
	out := mqttchat.NewOutMessageWithPrompt(msg, mqttClientId, mqttCmdId, prompt)
	t.chOut <- out
}

func (t *TelnetBridge) timeout() {

	ticker := time.NewTicker(timeoutCheckConnection)

	for {
		select {
		case <-ticker.C:
			log.Debug("start check timeout on connection")
			now := time.Now()
			t.telnetConnections.Range(func(k, v interface{}) bool {
				connection := v.(mqtt2telnetConnection)
				if now.Sub(connection.lastCommandTime) > (timeoutConnection) {
					t.stopTelnetConnection(connection.mqttClientID, true)
				}
				return true
			})
		}
	}
}

func NewTelnetBridgePlugin(maxConnection int, keyword string, outputChan chan mqttchat.OutMessage) *TelnetBridge {
	tb := TelnetBridge{chOut: outputChan, pluginName: keyword, maxConnections: maxConnection}
	if tb.pluginName == "" {
		tb.pluginName = defaultTelnetBridgePluginName
	}
	go tb.timeout()
	return &tb
}
