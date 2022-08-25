package mqtt2telnet

import (
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	shell "github.com/freedreamer82/mqtt-shell/internal/pkg/shellcmd"
	"github.com/freedreamer82/mqtt-shell/pkg/mqttchat"
	"github.com/reiver/go-telnet"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"path"
	"strings"
	"sync"
	"time"
)

const bridgeCmdPrefix = "bridge "
const escapeChar = "^]"
const quitString = "quit"
const maxConnections = 5

const timeoutCheckConnection = 10 * time.Second
const timeoutConnection = 1 * time.Minute
const timeoutBridgeScriptCmd = 0 * time.Minute

type MqttBridgeChat struct {
	*mqttchat.MqttChat
	telnetConnections sync.Map
	chOut             chan outMessage
	scripts           []string
	scriptsPath       string
}

type mqtt2telnetConnection struct {
	connection      *telnet.Conn
	mqttClientID    string
	lastCommandId   string
	telnetHost      string
	lastCommandTime time.Time
	chClose         chan bool
}

type outMessage struct {
	msg        string
	clientUUID string
	cmdUUID    string
}

func NewBridgeChat(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string, version string, scriptsPath string, opts ...mqttchat.MqttChatOption) *MqttBridgeChat {

	bc := MqttBridgeChat{scriptsPath: scriptsPath}
	chat := mqttchat.NewChat(mqttOpts, rxTopic, txTopic, version, opts...)
	chat.SetDataCallback(bc.OnDataRx)

	out := make(chan outMessage, 100)
	bc.chOut = out

	bc.MqttChat = chat

	if scriptsPath != "" {
		n, err := bc.loadScripts()
		if err != nil {
			log.Error(err.Error())
		} else {
			log.Infof("Loaded %d scripts", n)
		}
	}

	go bc.mqttTransmit()
	go bc.checkTimeout()

	return &bc
}

func (m *MqttBridgeChat) loadScripts() (int, error) {
	res := 0
	files, err := ioutil.ReadDir(m.scriptsPath)
	if err != nil {
		return 0, err
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".sh") {
			fileName := strings.Replace(file.Name(), ".sh", "", 1)
			m.scripts = append(m.scripts, fileName)
			res++
		}
	}
	return res, nil
}

func (m *MqttBridgeChat) OnDataRx(data mqttchat.MqttJsonData) {

	if data.CmdUUID == "" || data.Cmd == "" || data.Data == "" || data.ClientUUID == "" {
		return
	}

	str := strings.TrimSpace(fmt.Sprintf("%s", data.Data))

	go func() {

		isBridgeCmd, args, argsLen := isBridgeCommand(str)
		if isBridgeCmd {
			m.post(m.bridgeExec(data.ClientUUID, args, argsLen), data.ClientUUID, data.CmdUUID)
			return
		}

		value, ok := m.telnetConnections.Load(data.ClientUUID)
		if !ok {
			m.post("connection not established - try: bridge help", data.ClientUUID, data.CmdUUID)
			return
		}

		mqtt2telnet := value.(mqtt2telnetConnection)
		m.updateOnCommand(mqtt2telnet, data.CmdUUID)
		log.Debug(str)

		_, err := mqtt2telnet.connection.Write([]byte(str + "\r\n"))
		if err != nil {
			m.post(err.Error(), data.ClientUUID, data.CmdUUID)
			return
		}

	}()

}

func (m *MqttBridgeChat) updateOnCommand(mqtt2telnet mqtt2telnetConnection, lastCommandId string) {
	mqtt2telnet.lastCommandId = lastCommandId
	mqtt2telnet.lastCommandTime = time.Now()
	m.telnetConnections.Store(mqtt2telnet.mqttClientID, mqtt2telnet)
}

func (m *MqttBridgeChat) isScript(str string) bool {
	for _, s := range m.scripts {
		if s == str {
			return true
		}
	}
	return false
}

func (m *MqttBridgeChat) runShellScript(scriptName string, args []string) string {
	script := fmt.Sprintf("%s.sh", scriptName)
	scriptAbsolute := path.Join(m.scriptsPath, script)
	cmd := scriptAbsolute
	if args != nil && len(args) > 0 {
		for _, arg := range args {
			cmd = fmt.Sprintf("%s %s", cmd, arg)
		}
	}
	err, out := shell.Shellout(cmd, timeoutBridgeScriptCmd)
	if err != nil {
		log.Error(err.Error())
		return err.Error()
	}
	return out
}

func (m *MqttBridgeChat) bridgeExec(mqttClientId string, args []string, argsLen int) string {
	if argsLen == 1 && args[0] == "list" {
		res := "Active telnet connections: ... "
		m.telnetConnections.Range(func(k, v interface{}) bool {
			connection := v.(mqtt2telnetConnection)
			res = fmt.Sprintf("%s\r\n%s - %s", res, connection.mqttClientID, connection.telnetHost)
			return true
		})
		return res
	} else if argsLen == 1 && args[0] == "help" {
		return bridgeHelp
	} else if argsLen == 3 && args[0] == "connect" {
		return m.startTelnetConnection(mqttClientId, args[1], args[2])
	} else if argsLen == 1 && args[0] == "disconnect" {
		go m.stopTelnetConnection(mqttClientId, false)
		return ""
	} else if argsLen == 1 && args[0] == "scripts" {
		res := fmt.Sprintf("Bridge scripts, [ %s ] ...", m.scriptsPath)
		for _, s := range m.scripts {
			res = fmt.Sprintf("%s\r\n- %s", res, s)
		}
		return res
	} else if argsLen >= 1 && m.isScript(args[0]) {
		scriptName := args[0]
		var scriptArg []string
		if len(args) > 0 {
			scriptArg = args[1:]
		}
		if isConnected, _ := m.isClientAlreadyConnected(mqttClientId); isConnected {
			return "bridge scripts are enabled only in disconnected status, disconnect first"
		}
		return m.runShellScript(scriptName, scriptArg)
	}
	return "bridge: command not valid, try > bridge help"
}

func isBridgeCommand(str string) (bool, []string, int) {
	isBridge := strings.HasPrefix(str, bridgeCmdPrefix)
	if isBridge {
		cmd := strings.Replace(str, bridgeCmdPrefix, "", -1)
		if len(cmd) > 0 {
			split := strings.Split(cmd, " ")
			if len(split) > 0 {
				return true, split, len(split)
			}
		}
	}
	return false, nil, 0
}

func (m *MqttBridgeChat) isClientAlreadyConnected(mqttClientId string) (bool, string) {
	conn, ok := m.telnetConnections.Load(mqttClientId)
	if ok {
		return ok, conn.(mqtt2telnetConnection).telnetHost
	}
	return ok, ""
}

func (m *MqttBridgeChat) isHostAlreadyConnected(host string) (bool, string) {
	isConnected := false
	mqttClientId := ""
	m.telnetConnections.Range(func(k, v interface{}) bool {
		connection := v.(mqtt2telnetConnection)
		if connection.telnetHost == host {
			isConnected = true
			mqttClientId = connection.mqttClientID
		}
		return true
	})
	return isConnected, mqttClientId
}

func (m *MqttBridgeChat) countConnections() int {
	size := 0
	m.telnetConnections.Range(func(k, v interface{}) bool {
		size++
		return true
	})
	return size
}

func (m *MqttBridgeChat) startTelnetConnection(mqttClientId, host, port string) string {

	addr := fmt.Sprintf("%s:%s", host, port)

	isClientConnected, connectedHost := m.isClientAlreadyConnected(mqttClientId)
	if isClientConnected {
		return fmt.Sprintf("this client is already connected to %s, disconnect before creating a new connection", connectedHost)
	}

	isHostConnected, connectedMqttClient := m.isHostAlreadyConnected(addr)
	if isHostConnected {
		return fmt.Sprintf("this host is already connected to another mqtt client: %s", connectedMqttClient)
	}

	if m.countConnections() >= maxConnections {
		return "max number of connection reached"
	}

	log.Infof("start creating connection - host: %s, clientId: %s", addr, mqttClientId)
	conn, err := telnet.DialTo(addr)
	if err != nil {
		log.Error(err.Error())
		return err.Error()
	}

	mqtt2telnet := mqtt2telnetConnection{mqttClientID: mqttClientId, connection: conn, telnetHost: addr, lastCommandTime: time.Now(), chClose: make(chan bool, 1)}
	m.telnetConnections.Store(mqttClientId, mqtt2telnet)

	go m.listen(mqtt2telnet)

	return fmt.Sprintf("connection established with %s", addr)

}

func (m *MqttBridgeChat) stopTelnetConnection(mqttClientId string, isForTimeout bool) {

	value, ok := m.telnetConnections.Load(mqttClientId)
	if !ok {
		res := "connection not found - cant close it"
		log.Error(res)
		m.post(res, mqttClientId, "")
	}

	mqtt2telnet := value.(mqtt2telnetConnection)

	if isForTimeout {
		m.post("connection close due to inactivity", mqttClientId, mqtt2telnet.lastCommandId)
	}

	mqtt2telnet.chClose <- true

	_, _ = mqtt2telnet.connection.Write([]byte(escapeChar))
	time.Sleep(300 * time.Millisecond)
	_, _ = mqtt2telnet.connection.Write([]byte(quitString))

	_ = mqtt2telnet.connection.Close()

	m.telnetConnections.Delete(mqttClientId)

	m.post(fmt.Sprintf("connection closed with %s", mqtt2telnet.telnetHost), mqttClientId, mqtt2telnet.lastCommandId)
}

func (m *MqttBridgeChat) listen(mqtt2telnet mqtt2telnetConnection) {
	if mqtt2telnet.connection != nil {

		log.Info("start lister")

		buffChan := make(chan byte, 100)

		go func(buffChan chan byte) {
			for {
				buff := make([]byte, 1)
				_, err := mqtt2telnet.connection.Read(buff)

				if err != nil {
					log.Debug("closing2...")
					break
				}

				if buff != nil {
					b := buff[0]
					buffChan <- b
				}
			}
		}(buffChan)

		go func(buffChan chan byte) {
			ticker := time.NewTicker(250 * time.Millisecond)
			bigBuff := make([]byte, 256)
			flush := func(buff []byte) {
				cp := make([]byte, len(buff))
				copy(cp, buff)
				m.post(string(cp), mqtt2telnet.mqttClientID, mqtt2telnet.lastCommandId)
			}
			for {
				select {
				case b := <-buffChan:
					bigBuff = append(bigBuff, b)
					ticker.Reset(250 * time.Millisecond)
					if len(bigBuff) == 256 {
						flush(bigBuff)
						bigBuff = bigBuff[:0]
					}
				case <-ticker.C:
					if len(bigBuff) > 0 {
						flush(bigBuff)
						bigBuff = bigBuff[:0]
					}
				case <-mqtt2telnet.chClose:
					log.Debug("closing...")
					return
				}
			}
		}(buffChan)

	}
}

func (m *MqttBridgeChat) mqttTransmit() {
	for {
		select {
		case out := <-m.chOut:
			outMsg := out.msg
			if outMsg != "" {
				//fmt.Print(outMsg)
				m.Transmit(outMsg, out.cmdUUID, out.clientUUID)
			}
		}
	}
}

func (m *MqttBridgeChat) post(msg, mqttClientId, mqttCmdId string) {
	out := outMessage{msg, mqttClientId, mqttCmdId}
	m.chOut <- out
}

func (m *MqttBridgeChat) checkTimeout() {

	ticker := time.NewTicker(timeoutCheckConnection)

	for {
		select {
		case <-ticker.C:
			log.Debug("start check timeout on connection")
			now := time.Now()
			m.telnetConnections.Range(func(k, v interface{}) bool {
				connection := v.(mqtt2telnetConnection)
				if now.Sub(connection.lastCommandTime) > (timeoutConnection) {
					m.stopTelnetConnection(connection.mqttClientID, true)
				}
				return true
			})
		}
	}
}
