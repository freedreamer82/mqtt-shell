package sshbridge

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
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

type params struct {
	command string
	rawmode bool
	user    string
	host    string
	port    int
}

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
	session         *ssh.Session
	stdinPipe       io.WriteCloser
	stdoutPipe      io.Reader
	stderrPipe      io.Reader
	outputChan      chan string
	mqttClientID    string
	sshHost         string
	lastCommandId   string
	lastCommandTime time.Time
	mutex           sync.Mutex
	stdoutCancel    context.CancelFunc
	removeEcho      bool
	params          params
	rawmode         bool
	echorawmode     bool
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
			str = ""
			return
		}
		value, ok := s.sshConnections.Load(data.ClientUUID)
		if !ok {
			s.post(fmt.Sprintf("SSH plugin connection not established - try: %s help", s.pluginName), data.ClientUUID, data.CmdUUID)
			return
		}
		mqtt2ssh, ok := value.(*mqtt2sshConnection)
		if !ok {
			s.post("invalid connection type", data.ClientUUID, data.CmdUUID)
			return
		}
		s.updateTimeOnCommand(mqtt2ssh, data.CmdUUID)
		err := s.sendCommand(mqtt2ssh, str)
		if err != nil {
			if strings.Contains(err.Error(), "connection lost") || strings.Contains(err.Error(), "EOF") {
				s.stopSSHConnection(data.ClientUUID, true)
			}
			s.post(err.Error(), data.ClientUUID, data.CmdUUID)
		}
	}()
}

func (s *SSHBridge) updateTimeOnCommand(mqtt2ssh *mqtt2sshConnection, lastCommandId string) {
	mqtt2ssh.mutex.Lock()
	mqtt2ssh.lastCommandId = lastCommandId
	mqtt2ssh.lastCommandTime = time.Now()
	mqtt2ssh.mutex.Unlock()
	s.sshConnections.Store(mqtt2ssh.mqttClientID, mqtt2ssh)
}

func (s *SSHBridge) isClientConnected(mqttClientId string) (bool, string) {
	conn, ok := s.sshConnections.Load(mqttClientId)
	if ok {
		connection := conn.(*mqtt2sshConnection)
		return ok, connection.sshHost
	}
	return ok, ""
}

func (s *SSHBridge) isHostAlreadyConnected(host string) (bool, string) {
	isConnected := false
	mqttClientId := ""
	s.sshConnections.Range(func(k, v interface{}) bool {
		connection := v.(*mqtt2sshConnection)
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
	var keyPath, password string

	if argsLen == 1 && args[0] == "help" {
		return getSSHHelpText(s.pluginName)
	} else if argsLen >= 2 && isUserAtHost(args[0]) {
		userHost := args[0]
		port := "22"
		rawMode := false
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
			case "--raw":
				rawMode = true
				i++
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
			return s.startSSHConnectionWithKey(mqttClientId, host, port, user, keyPath, rawMode)
		}
		return s.startSSHConnection(mqttClientId, host, port, user, password, rawMode)
	} else if argsLen == 1 && args[0] == "disconnect" {
		go s.stopSSHConnection(mqttClientId, false)
		return "disconnected"
	}
	return getSSHErrorText(s.pluginName)
}

func isUserAtHost(s string) bool {
	return strings.Count(s, "@") == 1 && !strings.Contains(s, " ")
}

func (s *SSHBridge) startSSHConnection(mqttClientId, host, port, user, password string, rawmode bool) string {
	addr := fmt.Sprintf("%s:%s", host, port)
	isClientConnected, connectedHost := s.isClientConnected(mqttClientId)
	if isClientConnected {
		return fmt.Sprintf("this client is already connected to %s, disconnect before creating a new connection", connectedHost)
	}
	//	isHostConnected, connectedMqttClient := s.isHostAlreadyConnected(addr)
	//if isHostConnected {
	//	return fmt.Sprintf("this host is already connected to another mqtt client: %s", connectedMqttClient)
	//}
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

	mqtt2ssh := &mqtt2sshConnection{
		mqttClientID:    mqttClientId,
		client:          client,
		sshHost:         addr,
		lastCommandTime: time.Now(),
		rawmode:         rawmode,
		echorawmode:     false,
	}
	err = s.startInteractiveSession(mqtt2ssh)
	if err != nil {
		return fmt.Sprintf("failed to start interactive session: %v", err)
	}
	s.sshConnections.Store(mqttClientId, mqtt2ssh)
	return fmt.Sprintf("connection established with %s", addr)
}

func (s *SSHBridge) startSSHConnectionWithKey(mqttClientId, host, port, user, keyPath string, rawmode bool) string {
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
	mqtt2ssh := &mqtt2sshConnection{
		mqttClientID:    mqttClientId,
		client:          client,
		sshHost:         addr,
		lastCommandTime: time.Now(),
		rawmode:         rawmode,
		echorawmode:     false,
	}
	err = s.startInteractiveSession(mqtt2ssh)
	if err != nil {
		return fmt.Sprintf("failed to start interactive session: %v", err)
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
	mqtt2ssh, ok := value.(*mqtt2sshConnection)
	if !ok {
		res := "invalid connection type"
		log.Error(res)
		s.post(res, mqttClientId, "")
		return
	}
	s.StopStdoutReader(mqtt2ssh)
	if isForTimeout {
		mqtt2ssh.mutex.Lock()
		lastCommandId := mqtt2ssh.lastCommandId
		mqtt2ssh.mutex.Unlock()
		s.post("connection closed due to inactivity", mqttClientId, lastCommandId)
	}
	if mqtt2ssh.session != nil {
		_ = mqtt2ssh.session.Close()
	}
	_ = mqtt2ssh.client.Close()
	s.sshConnections.Delete(mqttClientId)
	mqtt2ssh.mutex.Lock()
	lastCommandId := mqtt2ssh.lastCommandId
	sshHost := mqtt2ssh.sshHost
	mqtt2ssh.mutex.Unlock()
	s.post(fmt.Sprintf("connection closed with %s", sshHost), mqttClientId, lastCommandId)
}

func (s *SSHBridge) startInteractiveSession(conn *mqtt2sshConnection) error {
	session, err := conn.client.NewSession()
	if err != nil {
		return err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	conn.session = session
	conn.stdinPipe = stdin
	conn.stdoutPipe = stdout

	s.StartStdoutReader(conn)
	if !conn.rawmode {
		err = conn.session.Shell()
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SSHBridge) StartStdoutReader(conn *mqtt2sshConnection) {
	ctx, cancel := context.WithCancel(context.Background())
	conn.stdoutCancel = cancel

	// Lettura stdout
	go func() {
		scanner := bufio.NewScanner(conn.stdoutPipe)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if !scanner.Scan() {
					break
				}
				line := scanner.Text()
				if conn.rawmode && !conn.echorawmode {
					conn.mutex.Lock()
					lastCmd := conn.lastCommandId
					conn.mutex.Unlock()
					if conn.removeEcho && strings.Contains(line, lastCmd) {
						conn.removeEcho = false
						continue
					}
				}
				s.post(line+"\r\n", conn.mqttClientID, "")
			}
		}
		if err := scanner.Err(); err != nil {
			s.post(fmt.Sprintf("stdout error: %v", err), conn.mqttClientID, "")
		}
	}()

	// Lettura stderr
	go func() {
		stderr, err := conn.session.StderrPipe()
		if err != nil {
			s.post(fmt.Sprintf("stderr pipe error: %v", err), conn.mqttClientID, "")
			return
		}
		scanner := bufio.NewScanner(stderr)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if !scanner.Scan() {
					break
				}
				line := scanner.Text()
				s.post(line+"\r\n", conn.mqttClientID, "")
			}
		}
		if err := scanner.Err(); err != nil {
			s.post(fmt.Sprintf("stderr error: %v", err), conn.mqttClientID, "")
		}
	}()
}

func (s *SSHBridge) StopStdoutReader(conn *mqtt2sshConnection) {
	if conn.stdoutCancel != nil {
		conn.stdoutCancel()
		conn.stdoutCancel = nil
	}
}

func (s *SSHBridge) sendCommand(conn *mqtt2sshConnection, cmd string) error {
	var err error

	if !conn.rawmode {
		var outBuf, errBuf bytes.Buffer

		conn.mutex.Lock()
		session, err := conn.client.NewSession()
		conn.mutex.Unlock()

		if err != nil {
			log.Errorf("Errore creazione sessione SSH: %v", err)
			return err
		}
		defer session.Close()

		session.Stdout = &outBuf
		session.Stderr = &errBuf

		err = session.Run(cmd)
		output := outBuf.String()
		if err != nil {
			output += "\n" + errBuf.String()
			output += fmt.Sprintf("\nErrore esecuzione comando: %v", err)
		}
		s.post(output, conn.mqttClientID, "")
	} else {
		conn.mutex.Lock()
		conn.lastCommandId = cmd

		// Default: modalità interattiva
		if !strings.HasSuffix(cmd, "\r") && !strings.HasSuffix(cmd, "\n") {
			cmd += "\r"
		}
		conn.removeEcho = true
		_, err = fmt.Fprintln(conn.stdinPipe, cmd)

		conn.mutex.Unlock()
	}

	return err
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
				connection := v.(*mqtt2sshConnection)
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
