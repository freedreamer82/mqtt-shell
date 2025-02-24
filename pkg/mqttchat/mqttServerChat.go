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
	inactivityTimeout = 5 * time.Minute // Timeout for client inactivity
)

// MqttServerChat represents the MQTT server chat with plugin support and directory context.
type MqttServerChat struct {
	*MqttChat
	plugins      []MqttSeverChatPlugin // List of plugins
	pluginMap    sync.Map              // Map to store plugin states
	outputChan   chan OutMessage       // Channel for outgoing messages
	currentDir   string                // Current directory of the server
	lastActivity sync.Map              // Map to store last activity time for each client
}

// MqttServerChatOption defines a function type for configuring the MQTT server chat.
type MqttServerChatOption func(*MqttServerChat)

// OnDataRx handles incoming data from clients.
func (m *MqttServerChat) OnDataRx(data MqttJsonData) {
	if data.CmdUUID == "" || data.Cmd == "" || data.Data == "" {
		return
	}

	// Update the last activity time for the client
	m.lastActivity.Store(data.ClientUUID, time.Now())

	str := fmt.Sprintf("%s", data.Data)
	if str != "" {
		// Check if the command is a plugin configuration command
		isPlugin, args, argsNo := m.isPluginConfigCmd(str)
		if isPlugin && data.ClientUUID != "" {
			res, p := m.handlePluginConfigCmd(data.ClientUUID, args, argsNo)
			m.outputChan <- NewOutMessageWithPrompt(res, data.ClientUUID, data.CmdUUID, p)
			return
		}

		// Check if the command is an autocomplete request
		if strings.HasPrefix(str, "autocomplete ") {
			// Handle autocomplete request
			partialInput := strings.TrimPrefix(str, "autocomplete ")
			options := m.generateAutocompleteOptions(partialInput)
			m.TransmitWithPath(fmt.Sprintf("autocomplete:%s", options), data.CmdUUID, data.ClientUUID, m.currentDir)
			return
		}

		// Check if the client has an active plugin
		pluginId, hasPluginActive := m.hasActivePlugin(data.ClientUUID)
		if hasPluginActive {
			m.execPluginCommand(pluginId, data)
			return
		}

		// Execute the command in the server's current directory context
		out := m.execShellCommand(str)

		// Send the response with the server's current path
		m.TransmitWithPath(out, data.CmdUUID, data.ClientUUID, m.currentDir)
	}
}

// execShellCommand executes a shell command in the server's current directory context.
func (m *MqttServerChat) execShellCommand(cmd string) string {
	// Log the current directory for debugging
	//log.Printf("Current directory: %s\n", m.currentDir)

	// Handle the "cd" command to change directory
	if strings.HasPrefix(cmd, "cd ") {
		dir := strings.TrimSpace(cmd[3:])
		err := os.Chdir(dir)
		if err != nil {
			return fmt.Sprintf("error: %v\n", err)
		}
		// Update the server's current directory
		m.currentDir, _ = os.Getwd()
		log.Printf("Changed directory to: %s\n", m.currentDir)
		return fmt.Sprintf("Changed directory to %s\n", m.currentDir)
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
		msg:        fmt.Sprintf("[%s] %s", prompt, msg), // Include the prompt in the message
		clientUUID: clientUUID,
		cmdUUID:    cmdUUID,
	}
}

// OutMessage represents an outgoing message to a client.
type OutMessage struct {
	msg        string // The message content
	clientUUID string // The UUID of the client
	cmdUUID    string // The UUID of the command
}

// NewServerChat creates a new MQTT server chat instance.
func NewServerChat(mqttOpts *MQTT.ClientOptions, topics ServerTopic, version string, opts ...MqttServerChatOption) *MqttServerChat {
	sc := MqttServerChat{}
	chat := NewChat(mqttOpts, topics.RxTopic, topics.TxTopic, version, WithOptionBeaconTopic(topics.BeaconRxTopic, topics.BeaconTxTopic))
	chat.SetDataCallback(sc.OnDataRx)
	sc.MqttChat = chat
	sc.outputChan = make(chan OutMessage, outputMsgSize)

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
				m.Transmit(outMsg, out.cmdUUID, out.clientUUID)
			}
		}
	}
}

// monitorInactivity checks for inactive clients and removes them.
func (m *MqttServerChat) monitorInactivity() {
	for {
		time.Sleep(1 * time.Minute) // Check every minute
		now := time.Now()
		m.lastActivity.Range(func(key, value interface{}) bool {
			clientUUID := key.(string)
			lastActive := value.(time.Time)
			if now.Sub(lastActive) > inactivityTimeout {
				// Remove inactive client
				m.lastActivity.Delete(clientUUID)
				log.Printf("Client %s removed due to inactivity\n", clientUUID)
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

/*
func (m *MqttServerChat) generateAutocompleteOptions(partialInput string) string {
	var pathPart string

	// Check if the input starts with a slash (absolute path)
	if strings.HasPrefix(partialInput, "/") {
		// Treat the entire input as an absolute path
		pathPart = partialInput
	} else {
		// Split the input on the first space to separate the command from the path
		parts := strings.SplitN(partialInput, " ", 2)
		if len(parts) < 2 {
			// If there is no space, assume the path is empty (current directory)
			pathPart = "."
		} else {
			// Use the part after the space as the path
			pathPart = strings.TrimSpace(parts[1])
		}
	}

	// If pathPart is empty, default to the current directory
	if pathPart == "" {
		pathPart = "."
	}

	var dir, prefix string
	var isLocalPath bool

	// Determine if the path is absolute or relative
	if strings.HasPrefix(pathPart, "/") {
		// Absolute path: use the path as-is
		dir = pathPart
		prefix = ""
		isLocalPath = false
	} else {
		// Relative path: prepend the current directory
		dir = filepath.Join(m.currentDir, filepath.Dir(pathPart))
		prefix = filepath.Base(pathPart)
		prefix = ""
		isLocalPath = true
	}

	// Read files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		return ""
		//fmt.Sprintf("error: %v", err)
	}

	var options []string
	for _, file := range files {
		// If prefix is empty, include all files (except hidden files if desired)
		if prefix == "" {
			// Optionally exclude hidden files (those starting with ".")
			if !strings.HasPrefix(file.Name(), ".") {
				pre := ""
				if partialInput != "" && partialInput[len(partialInput)-1] != '/' {
					pre = "/"
				}
				options = append(options, pre+file.Name())
			}
		} else if strings.HasPrefix(file.Name(), prefix) {
			// If prefix is not empty, include only files that match the prefix
			if isLocalPath {
				// Local path: add only the file/directory name
				options = append(options, file.Name())
			} else {
				// Absolute path: add the full path
				fullPath := filepath.Join(dir, file.Name())
				options = append(options, fullPath)
			}
		}
	}

	return strings.Join(options, "\n")
}
*/

const (
	MaxOptionsSize       = 90    // Dimensione massima di options
	MoreOptionsIndicator = "..." // Indicatore per opzioni aggiuntive
)

/* func (m *MqttServerChat) generateAutocompleteOptions(partialInput string) string {
	var pathPart string

	if partialInput == "" {
		dir := m.currentDir
		files, err := os.ReadDir(dir)
		if err != nil {
			return ""
		}
		var options []string
		for _, file := range files {
			if !strings.HasPrefix(file.Name(), ".") {
				options = append(options, file.Name())
				if len(options) >= MaxOptionsSize {
					options = append(options, MoreOptionsIndicator) // Usa la costante
					break
				}
			}
		}
		return strings.Join(options, "\n")
	}

	if strings.HasPrefix(partialInput, "/") {
		pathPart = partialInput
	} else {
		parts := strings.SplitN(partialInput, " ", 2)
		if len(parts) < 2 {
			pathPart = "./"
		} else {
			pathPart = strings.TrimSpace(parts[1])
		}
	}

	if pathPart == "" {
		pathPart = "./"
	}

	var dir, prefix string

	if strings.HasSuffix(pathPart, "/") {
		dir = pathPart
		prefix = ""
	} else {
		if strings.HasPrefix(pathPart, "/") {
			dir = filepath.Dir(pathPart)
			prefix = filepath.Base(pathPart)
		} else {
			dir = filepath.Join(m.currentDir, filepath.Dir(pathPart))
			prefix = filepath.Base(pathPart)
		}
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	var options []string
	var foundDir bool

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), ".") && strings.HasPrefix(file.Name(), prefix) {
			fileInfo, err := os.Stat(filepath.Join(dir, file.Name()))
			if err != nil {
				continue
			}
			if fileInfo.IsDir() {
				if prefix == file.Name() {
					fmt.Print("adding /")
					return "/"
				}
				options = append(options, strings.TrimPrefix(file.Name(), prefix)+"/")
				foundDir = true
			} else {
				options = append(options, strings.TrimPrefix(file.Name(), prefix))
			}

			if len(options) >= MaxOptionsSize {
				options = append(options, MoreOptionsIndicator) // Usa la costante
				break
			}
		}
	}

	if len(options) == 1 && foundDir {
		fmt.Print(strings.Join(options, "\n"))
		return options[0]
	}

	print(strings.Join(options, "\n"))
	return strings.Join(options, "\n")
}
*/

func (m *MqttServerChat) generateAutocompleteOptions(partialInput string) string {
	if partialInput == "" {
		return m.listFilesInDir(m.currentDir, "")
	}

	dir, prefix := m.parseInputPath(partialInput)
	return m.listFilesInDir(dir, prefix)
}

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
				options = append(options, MoreOptionsIndicator)
				break
			}
		}
	}

	if len(options) == 1 && foundDir {
		return options[0]
	}

	return strings.Join(options, "\n")
}
func (m *MqttServerChat) parseInputPath(partialInput string) (dir, prefix string) {
	if strings.HasPrefix(partialInput, "/") {
		// Handle absolute paths
		dir = filepath.Dir(partialInput)
		prefix = filepath.Base(partialInput)

		// Check if the path exists and is a directory
		if fileInfo, err := os.Stat(partialInput); err == nil && fileInfo.IsDir() {
			// If the path is a directory, list its contents without filtering by prefix
			dir = partialInput
			prefix = ""
		}
	} else {
		// Handle relative paths
		parts := strings.SplitN(partialInput, " ", 2)
		if len(parts) < 2 {
			dir = "./" // Default to current directory
		} else {
			dir = filepath.Join(m.currentDir, filepath.Dir(parts[1]))
			prefix = filepath.Base(parts[1])
		}
	}

	if dir == "" {
		dir = "./" // Fallback to current directory
	}

	return dir, prefix
}
