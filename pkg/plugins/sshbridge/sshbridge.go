package sshbridge

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/freedreamer82/mqtt-shell/pkg/mqttchat"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

const defaultSSHBridgePluginName = "ssh"
const timeoutCheckConnection = 10 * time.Second
const timeoutConnection = 1 * time.Minute

type SSHBridge struct {
	pluginName     string
	sshConnections sync.Map
	maxConnections int
	chOut          chan mqttchat.OutMessage
	prompt         string
}

func (s *SSHBridge) GetPrompt() string {
	if s.prompt == "" {
		s.prompt = s.pluginName
	}
	return s.prompt
}

type mqtt2sshConnection struct {
	client          *ssh.Client
	mqttClientID    string
	sshHost         string
	lastCommandId   string
	lastCommandTime time.Time
	mutex           sync.Mutex
}

func WithSSHBridge(maxConnections int, keyword string) mqttchat.MqttServerChatOption {
	return func(m *mqttchat.MqttServerChat) {
		sshBridge := NewSSHBridgePlugin(maxConnections, keyword, m.GetOutputChan())
		m.AddPlugin(sshBridge)
	}
}

func (s *SSHBridge) PluginId() string { return s.pluginName }
func (s *SSHBridge) GetName() string  { return "ssh" }

func (s *SSHBridge) isSSHBridgeCommand(str string) (bool, []string, int) {
	isBridge := strings.HasPrefix(str, s.pluginName)
	if isBridge {
		cmd := strings.TrimSpace(strings.Replace(str, s.pluginName, "", 1))
		if len(cmd) > 0 {
			split := strings.Fields(cmd)
			if len(split) > 0 {
				return true, split, len(split)
			}
		}
	}
	return false, nil, 0
}

func (s *SSHBridge) OnDataRx(data mqttchat.MqttJsonData) {
	if data.CmdUUID == "" || data.Cmd == "" || data.Data == "" || data.ClientUUID == "" {
		return
	}
	str := strings.TrimSpace(fmt.Sprintf("%s", data.Data))
	go func() {
		isBridgeCmd, args, argsLen := s.isSSHBridgeCommand(str)
		if isBridgeCmd {
			s.post(s.exec(data.ClientUUID, args, argsLen), data.ClientUUID, data.CmdUUID)
			return
		}
		value, ok := s.sshConnections.Load(data.ClientUUID)
		if !ok {
			s.post(fmt.Sprintf("SSH plugin connection not established - try: %s help", s.pluginName), data.ClientUUID, data.CmdUUID)
			return
		}
		mqtt2ssh, ok := value.(mqtt2sshConnection)
		if !ok {
			s.post("invalid connection type", data.ClientUUID, data.CmdUUID)
			return
		}
		s.updateTimeOnCommand(&mqtt2ssh, data.CmdUUID)
		output, err := s.runSSHCommand(&mqtt2ssh, str)
		if err != nil {
			if strings.Contains(err.Error(), "connection lost") || strings.Contains(err.Error(), "EOF") {
				s.stopSSHConnection(data.ClientUUID, true)
			}
			s.post(err.Error(), data.ClientUUID, data.CmdUUID)
		} else {
			s.post(output, data.ClientUUID, data.CmdUUID)
		}
	}()
}

func (s *SSHBridge) updateTimeOnCommand(mqtt2ssh *mqtt2sshConnection, lastCommandId string) {
	mqtt2ssh.mutex.Lock()
	mqtt2ssh.lastCommandId = lastCommandId
	mqtt2ssh.lastCommandTime = time.Now()
	mqtt2ssh.mutex.Unlock()
	s.sshConnections.Store(mqtt2ssh.mqttClientID, *mqtt2ssh)
}

func (s *SSHBridge) isClientConnected(mqttClientId string) (bool, string) {
	conn, ok := s.sshConnections.Load(mqttClientId)
	if ok {
		connection := conn.(mqtt2sshConnection)
		return ok, connection.sshHost
	}
	return ok, ""
}

func (s *SSHBridge) isHostAlreadyConnected(host string) (bool, string) {
	isConnected := false
	mqttClientId := ""
	s.sshConnections.Range(func(k, v interface{}) bool {
		connection := v.(mqtt2sshConnection)
		if connection.sshHost == host {
			isConnected = true
			mqttClientId = connection.mqttClientID
		}
		return true
	})
	return isConnected, mqttClientId
}

func (s *SSHBridge) countConnections() int {
	size := 0
	s.sshConnections.Range(func(k, v interface{}) bool {
		size++
		return true
	})
	return size
}

func (s *SSHBridge) exec(mqttClientId string, args []string, argsLen int) string {
	if argsLen == 1 && args[0] == "list" {
		res := "Active ssh connections: ... "
		s.sshConnections.Range(func(k, v interface{}) bool {
			connection := v.(mqtt2sshConnection)
			res = fmt.Sprintf("%s\r\n%s - %s", res, connection.mqttClientID, connection.sshHost)
			return true
		})
		return res
	} else if argsLen == 1 && args[0] == "help" {
		return getSSHHelpText(s.pluginName)
	} else if argsLen >= 2 && isUserAtHost(args[0]) {
		userHost := args[0]
		port := "22"
		password := ""
		keyPath := ""
		i := 1
		for i < argsLen {
			switch args[i] {
			case "-p":
				if i+1 < argsLen {
					port = args[i+1]
					i += 2
				} else {
					return getSSHErrorText(s.pluginName)
				}
			case "-i":
				if i+1 < argsLen {
					keyPath = args[i+1]
					i += 2
				} else {
					return getSSHErrorText(s.pluginName)
				}
			default:
				password = args[i]
				i++
			}
		}
		userHostSplit := strings.SplitN(userHost, "@", 2)
		if len(userHostSplit) != 2 {
			return "invalid user@host format"
		}
		user := userHostSplit[0]
		host := userHostSplit[1]
		if keyPath != "" {
			return s.startSSHConnectionWithKey(mqttClientId, host, port, user, keyPath)
		}
		if password == "" {
			s.post("Inserisci la password (non verrà mostrata):", mqttClientId, "")
			return ""
		}
		return s.startSSHConnection(mqttClientId, host, port, user, password)
	} else if argsLen == 1 && args[0] == "disconnect" {
		go s.stopSSHConnection(mqttClientId, false)
		return ""
	}
	return getSSHErrorText(s.pluginName)
}

func isUserAtHost(s string) bool {
	return strings.Count(s, "@") == 1 && !strings.Contains(s, " ")
}

func (s *SSHBridge) startSSHConnection(mqttClientId, host, port, user, password string) string {
	addr := fmt.Sprintf("%s:%s", host, port)
	isClientConnected, connectedHost := s.isClientConnected(mqttClientId)
	if isClientConnected {
		return fmt.Sprintf("this client is already connected to %s, disconnect before creating a new connection", connectedHost)
	}
	isHostConnected, connectedMqttClient := s.isHostAlreadyConnected(addr)
	if isHostConnected {
		return fmt.Sprintf("this host is already connected to another mqtt client: %s", connectedMqttClient)
	}
	if s.countConnections() >= s.maxConnections {
		return "max number of connection reached"
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		log.Error(err.Error())
		return err.Error()
	}

	mqtt2ssh := mqtt2sshConnection{
		mqttClientID:    mqttClientId,
		client:          client,
		sshHost:         addr,
		lastCommandTime: time.Now(),
	}
	s.sshConnections.Store(mqttClientId, mqtt2ssh)
	return fmt.Sprintf("connection established with %s", addr)
}

func (s *SSHBridge) startSSHConnectionWithKey(mqttClientId, host, port, user, keyPath string) string {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Sprintf("errore lettura chiave privata: %v", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return fmt.Sprintf("errore parsing chiave privata: %v", err)
	}
	addr := fmt.Sprintf("%s:%s", host, port)
	isClientConnected, connectedHost := s.isClientConnected(mqttClientId)
	if isClientConnected {
		return fmt.Sprintf("this client is already connected to %s, disconnect before creating a new connection", connectedHost)
	}
	isHostConnected, connectedMqttClient := s.isHostAlreadyConnected(addr)
	if isHostConnected {
		return fmt.Sprintf("this host is already connected to another mqtt client: %s", connectedMqttClient)
	}
	if s.countConnections() >= s.maxConnections {
		return "max number of connection reached"
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		log.Error(err.Error())
		return err.Error()
	}
	mqtt2ssh := mqtt2sshConnection{
		mqttClientID:    mqttClientId,
		client:          client,
		sshHost:         addr,
		lastCommandTime: time.Now(),
	}
	s.sshConnections.Store(mqttClientId, mqtt2ssh)
	return fmt.Sprintf("connection established with %s", addr)
}

func (s *SSHBridge) stopSSHConnection(mqttClientId string, isForTimeout bool) {
	value, ok := s.sshConnections.Load(mqttClientId)
	if !ok {
		res := "connection not found - cant close it"
		log.Error(res)
		s.post(res, mqttClientId, "")
		return
	}
	mqtt2ssh, ok := value.(mqtt2sshConnection)
	if !ok {
		res := "invalid connection type"
		log.Error(res)
		s.post(res, mqttClientId, "")
		return
	}
	if isForTimeout {
		mqtt2ssh.mutex.Lock()
		lastCommandId := mqtt2ssh.lastCommandId
		mqtt2ssh.mutex.Unlock()
		s.post("connection close due to inactivity", mqttClientId, lastCommandId)
	}
	_ = mqtt2ssh.client.Close()
	_ = mqtt2ssh.client.Close()
	s.sshConnections.Delete(mqttClientId)
	mqtt2ssh.mutex.Lock()
	lastCommandId := mqtt2ssh.lastCommandId
	sshHost := mqtt2ssh.sshHost
	mqtt2ssh.mutex.Unlock()
	s.post(fmt.Sprintf("connection closed with %s", sshHost), mqttClientId, lastCommandId)
}

func (s *SSHBridge) runSSHCommand(conn *mqtt2sshConnection, cmd string) (string, error) {
	var outBuf, errBuf bytes.Buffer

	conn.mutex.Lock()
	session, err := conn.client.NewSession()
	conn.mutex.Unlock()

	if err != nil {
		log.Errorf("Errore creazione sessione SSH: %v", err)
		return "", err
	}
	defer session.Close()

	// Configura i buffer per stdout e stderr separatamente
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	err = session.Run(cmd)

	time.Sleep(100 * time.Millisecond)

	output := outBuf.String()
	errOutput := errBuf.String()

	if errOutput != "" {
		if output != "" {
			output = output + "\n--- STDERR ---\n" + errOutput
		} else {
			output = errOutput
		}
	}

	log.Infof("Output comando: '%s', errore: %v", output, err)

	if err != nil {
		if strings.Contains(err.Error(), "connection lost") || strings.Contains(err.Error(), "EOF") {
			s.stopSSHConnection(conn.mqttClientID, true)
			return "", fmt.Errorf("SSH connection lost, please reconnect")
		}
		// Se abbiamo output nonostante l'errore, restituiamolo insieme all'errore
		if output != "" {
			return output, nil
		}
		return "", err
	}

	return output, nil
}

func (s *SSHBridge) runSSHCommandWithTimeout(conn *mqtt2sshConnection, cmd string) (string, error) {
	var outBuf, errBuf bytes.Buffer

	conn.mutex.Lock()
	session, err := conn.client.NewSession()
	conn.mutex.Unlock()

	if err != nil {
		log.Errorf("Error while creating SSH session: %v", err)
		return "", err
	}
	defer session.Close()

	// Configura i buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	// Esegui il comando con timeout
	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case err = <-done:
		// Comando completato normalmente
	case <-time.After(30 * time.Second):
		// Timeout del comando
		session.Signal(ssh.SIGTERM)
		return "", fmt.Errorf("command timeout after 30 seconds")
	}

	// Attendi un momento per l'output
	time.Sleep(100 * time.Millisecond)

	// Combina stdout e stderr
	output := outBuf.String()
	errOutput := errBuf.String()

	if errOutput != "" {
		if output != "" {
			output = output + "\n--- STDERR ---\n" + errOutput
		} else {
			output = errOutput
		}
	}

	if err != nil {
		if strings.Contains(err.Error(), "connection lost") || strings.Contains(err.Error(), "EOF") {
			s.stopSSHConnection(conn.mqttClientID, true)
			return "", fmt.Errorf("SSH connection lost, please reconnect")
		}
		// Restituisci output + errore se disponibile
		if output != "" {
			return fmt.Sprintf("%s\n--- ERROR ---\n%s", output, err.Error()), nil
		}
		return "", err
	}

	return output, nil
}

func (s *SSHBridge) post(msg, mqttClientId, mqttCmdId string) {
	prompt := s.pluginName
	isConnected, host := s.isClientConnected(mqttClientId)
	if isConnected {
		prompt = fmt.Sprintf("%s - %s", s.pluginName, host)
	}
	s.prompt = prompt
	out := mqttchat.NewOutMessageWithPrompt(msg, mqttClientId, mqttCmdId, prompt)
	s.chOut <- out
}

func (s *SSHBridge) timeout() {
	ticker := time.NewTicker(timeoutCheckConnection)
	for {
		select {
		case <-ticker.C:
			log.Debug("start check timeout on connection")
			now := time.Now()
			s.sshConnections.Range(func(k, v interface{}) bool {
				connection := v.(mqtt2sshConnection)
				connection.mutex.Lock()
				lastCommandTime := connection.lastCommandTime
				connection.mutex.Unlock()
				if now.Sub(lastCommandTime) > timeoutConnection {
					s.stopSSHConnection(connection.mqttClientID, true)
				}
				return true
			})
		}
	}
}

func NewSSHBridgePlugin(maxConnection int, keyword string, outputChan chan mqttchat.OutMessage) *SSHBridge {
	sb := SSHBridge{chOut: outputChan, pluginName: keyword, maxConnections: maxConnection}
	if sb.pluginName == "" {
		sb.pluginName = defaultSSHBridgePluginName
	}
	go sb.timeout()
	return &sb
}
