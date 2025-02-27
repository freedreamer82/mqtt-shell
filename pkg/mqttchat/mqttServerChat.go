package mqttchat

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	shell "github.com/freedreamer82/mqtt-shell/internal/pkg/shellcmd"
)

const (
	pluginCmdPrefix   = "plugin"        // Prefix for plugin commands
	outputMsgSize     = 10000           // Size of the output message channel
	inactivityTimeout = 3 * time.Minute // Timeout for client inactivity
)

const MaxOptionsSize = 90

// ClientState represents the state of a connected client.
type ClientState struct {
	ClientUUID string    // UUID of the client
	CurrentDir string    // Current directory of the client
	PluginId   string    // Active plugin ID (if any)
	LastActive time.Time // Last activity time of the client
}

// MqttServerChat represents the MQTT server chat with plugin support and directory context.
type MqttServerChat struct {
	*MqttChat
	plugins           []MqttSeverChatPlugin // List of plugins
	pluginMap         sync.Map              // Map to store plugin states
	outputChan        chan OutMessage       // Channel for outgoing messages
	clientStates      sync.Map              // Map to store client states
	currentDir        string                // Default directory for the server
	inactivityTimeout time.Duration         // Timeout for client inactivity
}

func (m *MqttServerChat) GetInactivityTimeout() time.Duration {
	return m.inactivityTimeout
}

func (m *MqttServerChat) SetInactivityTimeout(timeout time.Duration) {
	m.inactivityTimeout = timeout
}

func (m *MqttServerChat) sendPong(cmduuid string, clientuuid string) {
	pingData := NewMqttJsonDataEmpty()

	pingData.Cmd = MSG_DATA_TYPE_CMD_PONG
	pingData.CmdUUID = cmduuid
	pingData.ClientUUID = clientuuid

	m.Transmit(pingData)
}

type MqttServerChatOption func(*MqttServerChat)

// OnDataRx handles incoming data from clients.
func (m *MqttServerChat) OnDataRx(data MqttJsonData) {
	if data.CmdUUID == "" || data.Cmd == "" {
		return
	}

	// Check if the client UUID is already being managed
	clientState, exists := m.clientStates.Load(data.ClientUUID)
	if !exists {
		// If the UUID is not managed, log it and optionally handle it
		log.Printf("UUID %s is not managed. Creating new client state.", data.ClientUUID)
		clientState = m.createClientState(data.ClientUUID)
		m.clientStates.Store(data.ClientUUID, clientState)
	}

	state := clientState.(*ClientState)

	// Update the last activity time for the client
	state.LastActive = time.Now()

	// Handle the incoming message based on its type
	switch data.Cmd {
	case MSG_DATA_TYPE_CMD_PING:
		m.handlePing(data, state)
	case MSG_DATA_TYPE_CMD_AUTOCOMPLETE:
		m.handleAutocomplete(data, state)
	default:
		m.handleCommand(data, state)
	}
}

// isUUIDManaged checks if the UUID is already managed by the server
func (m *MqttServerChat) isUUIDManaged(uuid string) bool {
	_, exists := m.clientStates.Load(uuid)
	return exists
}

// createClientState creates a new client state with default values
func (m *MqttServerChat) createClientState(clientUUID string) *ClientState {
	return &ClientState{
		ClientUUID: clientUUID,
		CurrentDir: m.currentDir,
		LastActive: time.Now(),
	}
}

func (m *MqttServerChat) handlePing(data MqttJsonData, state *ClientState) {
	// Send a PONG response with the CmdUUID and ClientUUID
	m.sendPong(data.CmdUUID, data.ClientUUID)

}

func (m *MqttServerChat) handleAutocomplete(data MqttJsonData, state *ClientState) {
	partialInput := fmt.Sprintf("%s", data.Data)
	options := m.generateAutocompleteOptions(partialInput, state.CurrentDir)
	responseData := NewMqttJsonDataEmpty()
	responseData.Data = options
	responseData.CmdUUID = data.CmdUUID
	responseData.ClientUUID = state.ClientUUID
	responseData.CurrentPath = state.CurrentDir
	responseData.Cmd = MSG_DATA_TYPE_CMD_AUTOCOMPLETE
	m.Transmit(responseData)
}

// handleCommand handles generic commands
func (m *MqttServerChat) handleCommand(data MqttJsonData, state *ClientState) {
	str := fmt.Sprintf("%s", data.Data)

	// Check if the command is a plugin configuration command
	isPlugin, args, argsNo := m.isPluginConfigCmd(str)
	if isPlugin && data.ClientUUID != "" {
		res, p := m.handlePluginConfigCmd(data.ClientUUID, args, argsNo)
		m.outputChan <- NewOutMessageWithPrompt(res, data.ClientUUID, data.CmdUUID, p)
		return
	}

	// Check if the client has an active plugin
	if state.PluginId != "" {
		m.execPluginCommand(state.PluginId, data)
		return
	}

	// Execute the command in the client's current directory context
	out := m.execShellCommand(str, state)
	responseData := NewMqttJsonDataEmpty()
	responseData.Data = out
	responseData.CmdUUID = data.CmdUUID
	responseData.ClientUUID = state.ClientUUID
	responseData.CurrentPath = state.CurrentDir
	m.Transmit(responseData)
}

// execShellCommand executes a shell command in the client's current directory context.
func (m *MqttServerChat) execShellCommand(cmd string, state *ClientState) string {
	// Handle the "cd" command to change directory
	if strings.HasPrefix(cmd, "cd ") {
		dir := strings.TrimSpace(cmd[3:])
		err := os.Chdir(dir)
		if err != nil {
			return fmt.Sprintf("error: %v\n", err)
		}
		// Update the client's current directory
		state.CurrentDir, _ = os.Getwd()
		log.Printf("Client %s changed directory to: %s\n", state.ClientUUID, state.CurrentDir)
		return fmt.Sprintf("Changed directory to %s\n", state.CurrentDir)
	}

	// Execute the command using the shell
	err, out := shell.Shellout(cmd, m.timeoutCmdShell)
	if err != nil {
		log.Printf("error: %v\n", err)
	}

	return out
}

// GetOutputChan returns the output message channel.
func (m *MqttServerChat) GetOutputChan() chan OutMessage {
	return m.outputChan
}

// NewOutMessage creates a new OutMessage.
func NewOutMessage(msg, clientUUID, cmdUUID string) OutMessage {
	return OutMessage{msg: msg, clientUUID: clientUUID, cmdUUID: cmdUUID}
}

// NewOutMessageWithPrompt creates a new OutMessage with a custom prompt.
func NewOutMessageWithPrompt(msg, clientUUID, cmdUUID, prompt string) OutMessage {
	return OutMessage{
		msg:        msg,
		clientUUID: clientUUID,
		cmdUUID:    cmdUUID,
		prompt:     prompt,
	}
}

// OutMessage represents an outgoing message to a client.
type OutMessage struct {
	msg        string // The message content
	clientUUID string // The UUID of the client
	cmdUUID    string // The UUID of the command
	prompt     string // The custom prompt
}

// NewServerChat creates a new MQTT server chat instance.
func NewServerChat(mqttOpts *MQTT.ClientOptions, topics ServerTopic, version string, opts ...MqttServerChatOption) *MqttServerChat {
	sc := MqttServerChat{}
	chat := NewChat(mqttOpts, topics.RxTopic, topics.TxTopic, version, WithOptionBeaconTopic(topics.BeaconRxTopic, topics.BeaconTxTopic))
	chat.SetDataCallback(sc.OnDataRx)
	sc.MqttChat = chat
	sc.outputChan = make(chan OutMessage, outputMsgSize)
	sc.inactivityTimeout = inactivityTimeout
	// Initialize the server's current directory
	sc.currentDir, _ = os.Getwd()

	// Apply additional options
	for _, opt := range opts {
		opt(&sc)
	}

	if sc.netInterface != "" {
		sc.MqttChat.netInterface = sc.netInterface
	}

	// Start the MQTT transmit loop and inactivity monitor
	go sc.mqttTransmit()
	go sc.monitorInactivity()

	return &sc
}

// mqttTransmit handles outgoing messages to clients.
func (m *MqttServerChat) mqttTransmit() {
	for {
		select {
		case out := <-m.outputChan:
			outMsg := out.msg
			if outMsg != "" {
				// Retrieve the client state
				if state, ok := m.clientStates.Load(out.clientUUID); ok {
					clientState := state.(*ClientState)
					data := NewMqttJsonDataEmpty()
					data.Data = outMsg
					data.CmdUUID = out.cmdUUID
					data.ClientUUID = clientState.ClientUUID
					data.CurrentPath = clientState.CurrentDir
					data.CustomPrompt = out.prompt
					m.Transmit(data)

					//m.TransmitWithPath(outMsg, out.cmdUUID, clientState.ClientUUID, clientState.CurrentDir, 0, out.prompt)
				}
			}
		}
	}
}

// monitorInactivity checks for inactive clients and removes them.
func (m *MqttServerChat) monitorInactivity() {
	for {
		time.Sleep(1 * time.Minute) // Check every minute
		now := time.Now()
		m.clientStates.Range(func(key, value interface{}) bool {
			state := value.(*ClientState)
			if now.Sub(state.LastActive) > m.inactivityTimeout {
				// Remove inactive client
				m.clientStates.Delete(state.ClientUUID)
				log.Printf("Client %s removed due to inactivity\n", state.ClientUUID)
			}
			return true
		})
	}
}

// ServerTopic defines the MQTT topics for the server.
type ServerTopic struct {
	RxTopic       string // Topic for receiving messages
	TxTopic       string // Topic for sending messages
	BeaconRxTopic string // Topic for beacon requests
	BeaconTxTopic string // Topic for beacon responses
}

// WithOptionBeaconTopic sets the beacon topic for the MQTT chat.
func WithOptionBeaconTopic(topic string, topicRequest string) MqttChatOption {
	return func(h *MqttChat) {
		h.beaconTopic = topic
		h.beaconRequestTopic = topicRequest
	}
}

// WithOptionNetworkInterface sets the network interface for the MQTT server chat.
func WithOptionNetworkInterface(netI string) MqttServerChatOption {
	return func(m *MqttServerChat) {
		m.netInterface = netI
	}
}

// generateAutocompleteOptions generates autocomplete options for a given input.
func (m *MqttServerChat) generateAutocompleteOptions(partialInput string, currentDir string) string {
	if partialInput == "" {
		return m.listFilesInDir(currentDir, "")
	}

	dir, prefix := m.parseInputPath(partialInput, currentDir)
	out := m.listFilesInDir(dir, prefix)
	return out
}

// listFilesInDir lists files in a directory with a given prefix.
func (m *MqttServerChat) listFilesInDir(dir, prefix string) string {
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Sprintf("Error reading directory: %s", err)
	}

	var options []string
	var foundDir bool

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), ".") && strings.HasPrefix(file.Name(), prefix) {
			filePath := filepath.Join(dir, file.Name())
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				continue
			}

			if fileInfo.IsDir() {
				if prefix == file.Name() {
					return "/"
				}
				options = append(options, strings.TrimPrefix(file.Name(), prefix)+"/")
				foundDir = true
			} else {
				options = append(options, strings.TrimPrefix(file.Name(), prefix))
			}

			if len(options) >= MaxOptionsSize {
				options = append(options, "...")
				break
			}
		}
	}

	if len(options) == 1 && foundDir {
		return options[0]
	}

	return strings.Join(options, "\n")
}

// parseInputPath parses the input path for autocomplete.
func (m *MqttServerChat) parseInputPath(partialInput string, currentDir string) (dir, prefix string) {
	if strings.HasPrefix(partialInput, "/") {
		// Handle absolute paths
		dir = filepath.Dir(partialInput)
		prefix = filepath.Base(partialInput)

		// Check if the path exists and is a directory
		if fileInfo, err := os.Stat(partialInput); err == nil && fileInfo.IsDir() {
			dir = partialInput
			prefix = ""
		}
	} else if strings.Contains(partialInput, "/") {
		// Handle relative paths
		lastSlashIndex := strings.LastIndex(partialInput, "/")
		dir = filepath.Join(currentDir, partialInput[:lastSlashIndex])
		prefix = partialInput[lastSlashIndex+1:]

		// Check if the directory exists
		if fileInfo, err := os.Stat(dir); err != nil || !fileInfo.IsDir() {
			dir = currentDir
			prefix = partialInput
		}
	} else if strings.Contains(partialInput, " ") {
		// Handle commands with relative paths
		parts := strings.SplitN(partialInput, " ", 2)
		if len(parts) < 2 {
			dir = "./"
		} else {
			dir = filepath.Join(currentDir, filepath.Dir(parts[1]))
			prefix = filepath.Base(parts[1])
		}
	} else {
		// Handle local file completion
		dir = currentDir
		prefix = partialInput
	}

	if dir == "" {
		dir = "./"
	}

	return dir, prefix
}

// GetClientState retrieves the state of a client.
func (m *MqttServerChat) GetClientState(clientUUID string) (*ClientState, bool) {
	state, ok := m.clientStates.Load(clientUUID)
	if !ok {
		return nil, false
	}
	return state.(*ClientState), true
}

// SetClientState sets the state of a client.
func (m *MqttServerChat) SetClientState(clientUUID string, state *ClientState) {
	m.clientStates.Store(clientUUID, state)
}

// GetClientsConnected restituisce una mappa dei client attualmente connessi.
func (m *MqttServerChat) GetClientsConnected() map[string]*ClientState {
	clients := make(map[string]*ClientState)
	m.clientStates.Range(func(key, value interface{}) bool {
		clientUUID := key.(string)
		state := value.(*ClientState)
		if time.Since(state.LastActive) <= inactivityTimeout {
			clients[clientUUID] = state
		}
		return true
	})
	return clients
}

// GetClients restituisce l'intera mappa dei client (connessi e non).
func (m *MqttServerChat) GetClients() *sync.Map {
	return &m.clientStates
}
