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
	outputMsgSize     = 1000            // Size of the output message channel (ridotto)
	inactivityTimeout = 3 * time.Minute // Timeout for client inactivity
	MaxOptionsSize    = 90              // Max number of autocomplete options
)

// ClientState represents the state of a connected client.
type ClientState struct {
	ClientUUID string    // UUID of the client
	CurrentDir string    // Current directory of the client
	PluginId   string    // Active plugin ID (if any)
	LastActive time.Time // Last activity time of the client
}

// OutMessage represents an outgoing message to a client.
type OutMessage struct {
	msg        string // The message content
	clientUUID string // The UUID of the client
	cmdUUID    string // The UUID of the command
	prompt     string // The custom prompt
}

// ServerTopic defines the MQTT topics for the server.
type ServerTopic struct {
	RxTopic       string // Topic for receiving messages
	TxTopic       string // Topic for sending messages
	BeaconRxTopic string // Topic for beacon requests
	BeaconTxTopic string // Topic for beacon responses
}

// MqttServerChat represents the MQTT server chat with plugin support and directory context.
type MqttServerChat struct {
	*MqttChat
	plugins           []MqttSeverChatPlugin // List of plugins , each client can have 1 plugin at time (stored in client state)
	outputChan        chan OutMessage       // Channel for outgoing messages
	clientStates      sync.Map              // Map to store client states
	currentDir        string                // Default directory for the server
	inactivityTimeout time.Duration         // Timeout for client inactivity
	netInterface      string                // Network interface to use
	shutdown          chan struct{}         // Channel to signal shutdown
}

type MqttServerChatOption func(*MqttServerChat)

// NewServerChat creates a new MQTT server chat instance.
func NewServerChat(mqttOpts *MQTT.ClientOptions, topics ServerTopic, version string, opts ...MqttServerChatOption) *MqttServerChat {
	// Ottieni la directory corrente una sola volta all'inizio
	currentDir, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting current directory: %v. Using '.'", err)
		currentDir = "."
	}

	sc := MqttServerChat{
		outputChan:        make(chan OutMessage, outputMsgSize),
		inactivityTimeout: inactivityTimeout,
		currentDir:        currentDir,
		shutdown:          make(chan struct{}),
	}

	chat := NewChat(mqttOpts, topics.RxTopic, topics.TxTopic, version, WithOptionBeaconTopic(topics.BeaconRxTopic, topics.BeaconTxTopic))
	chat.SetDataCallback(sc.OnDataRx)
	sc.MqttChat = chat

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

// Cleanup releases resources and stops goroutines
func (m *MqttServerChat) Cleanup() {
	close(m.shutdown)
}

// GetInactivityTimeout returns the inactivity timeout duration.
func (m *MqttServerChat) GetInactivityTimeout() time.Duration {
	return m.inactivityTimeout
}

// SetInactivityTimeout sets the inactivity timeout duration.
func (m *MqttServerChat) SetInactivityTimeout(timeout time.Duration) {
	m.inactivityTimeout = timeout
}

// sendPong sends a PONG response to a client.
func (m *MqttServerChat) sendPong(cmduuid, clientuuid string) {
	pingData := NewMqttJsonDataEmpty()
	pingData.Cmd = MSG_DATA_TYPE_CMD_PONG
	pingData.CmdUUID = cmduuid
	pingData.ClientUUID = clientuuid
	m.Transmit(pingData)
}

// OnDataRx handles incoming data from clients.
func (m *MqttServerChat) OnDataRx(data MqttJsonData) {
	if data.CmdUUID == "" || data.Cmd == "" || data.ClientUUID == "" {
		log.Printf("Invalid message received: missing essential fields")
		return
	}

	// Check if the client UUID is already being managed or create a new state
	clientState := m.getOrCreateClientState(data.ClientUUID)

	// Update the last activity time for the client
	clientState.LastActive = time.Now()

	// Handle the incoming message based on its type
	switch data.Cmd {
	case MSG_DATA_TYPE_CMD_PING:
		m.handlePing(data, clientState)
	case MSG_DATA_TYPE_CMD_AUTOCOMPLETE:
		m.handleAutocomplete(data, clientState)
	default:
		m.handleCommand(data, clientState)
	}
}

// getOrCreateClientState gets an existing client state or creates a new one
func (m *MqttServerChat) getOrCreateClientState(clientUUID string) *ClientState {
	state, exists := m.clientStates.Load(clientUUID)
	if !exists {
		// If the UUID is not managed, create a new client state
		log.Printf("UUID %s is not managed. Creating new client state.", clientUUID)
		newState := &ClientState{
			ClientUUID: clientUUID,
			CurrentDir: m.currentDir,
			LastActive: time.Now(),
		}
		m.clientStates.Store(clientUUID, newState)
		return newState
	}
	return state.(*ClientState)
}

// handlePing handles PING messages from clients.
func (m *MqttServerChat) handlePing(data MqttJsonData, state *ClientState) {
	m.sendPong(data.CmdUUID, data.ClientUUID)
}

// handleAutocomplete handles autocomplete requests from clients.
func (m *MqttServerChat) handleAutocomplete(data MqttJsonData, state *ClientState) {
	partialInput := fmt.Sprintf("%v", data.Data)

	options := m.generateAutocompleteOptions(partialInput, state.CurrentDir)
	responseData := NewMqttJsonDataEmpty()
	responseData.Data = options
	responseData.CmdUUID = data.CmdUUID
	responseData.ClientUUID = state.ClientUUID
	responseData.CurrentPath = state.CurrentDir
	responseData.Cmd = MSG_DATA_TYPE_CMD_AUTOCOMPLETE
	m.Transmit(responseData)
}

// handleCommand handles generic commands from clients.
func (m *MqttServerChat) handleCommand(data MqttJsonData, state *ClientState) {
	cmdStr := fmt.Sprintf("%v", data.Data)

	// Check if the command is a plugin configuration command
	isPlugin, args, argsNo := m.isPluginConfigCmd(cmdStr)
	if isPlugin && data.ClientUUID != "" {
		res, p := m.handlePluginConfigCmd(state, args, argsNo)
		select {
		case m.outputChan <- NewOutMessageWithPrompt(res, data.ClientUUID, data.CmdUUID, p):
			// Message sent successfully
		default:
			log.Printf("Output channel full, dropping message for client %s", data.ClientUUID)
		}
		return
	}

	// Check if the client has an active plugin
	if state.PluginId != "" {
		m.execPluginCommand(state.PluginId, data)
		return
	}

	// Execute the command in the client's current directory context
	out := m.execShellCommand(cmdStr, state)
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

		// Se è una directory relativa, uniscila alla directory corrente
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(state.CurrentDir, dir)
		}

		// Espandi la directory (risolvi ".." e ".")
		dir = filepath.Clean(dir)

		err := os.Chdir(dir)
		if err != nil {
			return fmt.Sprintf("error: %v\n", err)
		}

		// Update the client's current directory
		currentDir, err := os.Getwd()
		if err != nil {
			log.Printf("Error getting current directory: %v", err)
			return fmt.Sprintf("Changed directory but failed to get path: %v\n", err)
		}

		state.CurrentDir = currentDir
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

// mqttTransmit handles outgoing messages to clients.
func (m *MqttServerChat) mqttTransmit() {
	for {
		select {
		case out := <-m.outputChan:
			if out.msg == "" || out.clientUUID == "" {
				continue
			}

			// Retrieve the client state without creating a new one if it doesn't exist
			clientState, exists := m.GetClientState(out.clientUUID)
			if !exists {
				log.Printf("Client %s not found for message delivery, skipping", out.clientUUID)
				continue
			}
			data := NewMqttJsonDataEmpty()
			data.Data = out.msg
			data.CmdUUID = out.cmdUUID
			data.ClientUUID = clientState.ClientUUID
			data.CurrentPath = clientState.CurrentDir
			data.CustomPrompt = out.prompt
			m.Transmit(data)
		case <-m.shutdown:
			return
		}
	}
}

// monitorInactivity checks for inactive clients and removes them.
func (m *MqttServerChat) monitorInactivity() {
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
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
		case <-m.shutdown:
			return
		}
	}
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
	return m.listFilesInDir(dir, prefix)
}

// listFilesInDir lists files in a directory with a given prefix.
func (m *MqttServerChat) listFilesInDir(dir string, prefix string) string {
	// Verifica validità della directory
	if dir == "" {
		dir = "./"
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Sprintf("Error reading directory: %s", err)
	}

	var options []string
	var foundDir bool

	for _, file := range files {
		// Salta i file nascosti e verifica il prefisso
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

			// Limita il numero di opzioni
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
	// Assicurati che currentDir sia valido
	if currentDir == "" {
		var err error
		currentDir, err = os.Getwd()
		if err != nil {
			currentDir = "./"
		}
	}

	if strings.HasPrefix(partialInput, "/") {
		// Handle absolute paths
		lastSlash := strings.LastIndex(partialInput, "/")
		if lastSlash == len(partialInput)-1 {
			// If the input ends with a slash, we're looking in that directory
			dir = partialInput
			prefix = ""
		} else {
			dir = filepath.Dir(partialInput)
			prefix = filepath.Base(partialInput)
		}
	} else if strings.Contains(partialInput, "/") {
		// Handle relative paths
		lastSlashIndex := strings.LastIndex(partialInput, "/")
		relativeDir := partialInput[:lastSlashIndex+1]

		// Unisci la directory corrente con la parte relativa
		dir = filepath.Join(currentDir, relativeDir)
		prefix = partialInput[lastSlashIndex+1:]
	} else if strings.Contains(partialInput, " ") {
		// Handle commands with arguments
		parts := strings.Fields(partialInput)
		if len(parts) <= 1 {
			dir = currentDir
			prefix = partialInput
		} else {
			// Analizza l'ultimo argomento
			lastArg := parts[len(parts)-1]
			if strings.Contains(lastArg, "/") {
				lastSlashIndex := strings.LastIndex(lastArg, "/")
				if lastSlashIndex >= 0 {
					relativePath := lastArg[:lastSlashIndex+1]
					dir = filepath.Join(currentDir, relativePath)
					prefix = lastArg[lastSlashIndex+1:]
				} else {
					dir = currentDir
					prefix = lastArg
				}
			} else {
				dir = currentDir
				prefix = lastArg
			}
		}
	} else {
		// Handle local file completion
		dir = currentDir
		prefix = partialInput
	}

	// Verifica che la directory esista
	if fileInfo, err := os.Stat(dir); err != nil || !fileInfo.IsDir() {
		dir = currentDir
	}

	return dir, prefix
}

// GetClientState retrieves the state of a client without creating it if it doesn't exist.
// Returns the client state and a boolean indicating if the client exists.
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

// GetClientsConnected returns a map of currently connected clients.
func (m *MqttServerChat) GetClientsConnected() map[string]*ClientState {
	clients := make(map[string]*ClientState)
	m.clientStates.Range(func(key, value interface{}) bool {
		clientUUID := key.(string)
		state := value.(*ClientState)
		if time.Since(state.LastActive) <= m.inactivityTimeout {
			clients[clientUUID] = state
		}
		return true
	})
	return clients
}

// GetClients returns the entire map of clients (connected or not).
func (m *MqttServerChat) GetClients() *sync.Map {
	return &m.clientStates
}
