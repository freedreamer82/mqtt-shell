package mqttchat

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/lithammer/shortuuid/v3"
	log "github.com/sirupsen/logrus"
)

const prompt = ">"
const login = "-------------------------------------------------\r\n|  Mqtt-shell client \r\n|\r\n|  IP: %s \r\n|  SERVER VER: %s - CLIENT VER: %s\r\n|  CLIENT UUID: %s\r\n|  TX: %s\r\n|  RX: %s\r\n|\r\n-------------------------------------------------\r\n"

const RED = "\033[1;31m"
const NC = "\033[0m"

// ClientChatIO is a struct that wraps an io.Reader and io.Writer for custom input/output.
type ClientChatIO struct {
	io.Reader
	io.Writer
}

// MqttClientChat represents the MQTT chat client.
type MqttClientChat struct {
	*MqttChat
	waitServerChan      chan bool          // Channel to wait for server response
	rl                  *readline.Instance // Readline instance for input handling
	uuid                string             // Unique client UUID
	customPrompt        string             // Custom prompt for the client
	enableColor         bool               // Enable colored output
	printPromptManually bool               // Whether to print the prompt manually
	historyFile         string             // Path to the history file
	historyLimit        int                // Maximum number of entries in the history file
}

// print prints the given arguments to the readline output.
func (m *MqttClientChat) print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(m.rl.Stdout(), a...)
}

// println prints a newline to the readline output.
func (m *MqttClientChat) println() (n int, err error) {
	return fmt.Fprintln(m.rl.Stdout())
}

// printf prints formatted text to the readline output.
func (m *MqttClientChat) printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(m.rl.Stdout(), format, a...)
}

// printWithoutLn prints the given arguments without a newline.
func (m *MqttClientChat) printWithoutLn(a ...interface{}) (n int, err error) {
	return fmt.Fprint(m.rl.Stdout(), a...)
}

// IsDataInvalid checks if the received MQTT data is invalid.
func (m *MqttClientChat) IsDataInvalid(data MqttJsonData) bool {
	return data.CmdUUID == "" || data.Cmd == "" || data.Data == "" || data.ClientUUID != m.uuid
}

// OnDataRx handles incoming MQTT data.
func (m *MqttClientChat) OnDataRx(data MqttJsonData) {
	if m.IsDataInvalid(data) {
		return
	}
	out := strings.TrimSuffix(data.Data, "\n") // Remove newline
	m.customPrompt = data.CustomPrompt
	m.print(out)
	m.println()
	m.printPrompt()
}

// waitServerCb is the callback for waiting for the server response.
func (m *MqttClientChat) waitServerCb(data MqttJsonData) {
	if m.IsDataInvalid(data) {
		log.Debug()
		return
	}
	m.waitServerChan <- true
	ip := data.Ip
	serverVersion := data.Version
	m.printLogin(ip, serverVersion)
}

// printPrompt prints the prompt to the terminal.
func (m *MqttClientChat) printPrompt() {
	if m.printPromptManually {
		p := prompt
		if m.customPrompt != "" {
			p = fmt.Sprintf("<%s%s", m.customPrompt, prompt)
		}
		if m.enableColor {
			m.printWithoutLn(fmt.Sprintf("%s%s%s", RED, p, NC))
		} else {
			m.printWithoutLn(p)
		}
	}
}

// printLogin prints the login message with server and client details.
func (m *MqttClientChat) printLogin(ip string, serverVersion string) {
	log.Info("Connected")
	m.printf(login, ip, serverVersion, m.version, m.uuid, m.txTopic, m.rxTopic)
}

// waitServer waits for the server to respond and handles retries.
func (m *MqttClientChat) waitServer() {
	m.SetDataCallback(m.waitServerCb)
	for {
		log.Info("Connecting to server...")
		m.Transmit("whoami", "", m.uuid)
		select {
		case ok := <-m.waitServerChan:
			if ok {
				m.SetDataCallback(m.OnDataRx)
				return
			}
		case <-time.After(5 * time.Second):
			log.Info("TIMEOUT , retry...")
		}
	}
}

// clearScreen clears the terminal screen.
func (m *MqttClientChat) clearScreen() {
	m.print("\033[2J") // Clear the entire screen
	m.print("\033[H")  // Move the cursor to the top-left corner
}

// clientTask is the main task for handling client input and output.
func (m *MqttClientChat) clientTask() {
	m.waitServer()

	// Add a listener to intercept the Tab key
	m.rl.Config.SetListener(func(line []rune, pos int, key rune) (newLine []rune, newPos int, ok bool) {
		if key == '\t' { // Intercept the Tab key
			fmt.Print("Tab not supported , sorry")
			return nil, 0, false // Stop input
		}
		return line, pos, true // Continue input
	})

	for {
		m.printPrompt()
		line, err := m.rl.Readline()
		if err != nil { // Ctrl+D or Ctrl+C to exit
			if err.Error() == "EOF" || err.Error() == "interrupt" {
				log.Info("Exiting...")
				break
			}
			fmt.Println("Goodbye!")
			os.Exit(0)
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue // Ignore empty lines
		} else if line == "clear" {
			m.clearScreen() // Use the clearScreen function
			continue        // Do not send the command to the server
		}

		// Send the command via MQTT
		m.Transmit(line, "", m.uuid)
	}
}

// Close closes the readline instance.
func (m *MqttClientChat) Close() {
	if m.rl != nil {
		m.rl.Close()
	}
}

// SetHistoryFile sets the history file and its maximum length.
func (m *MqttClientChat) SetHistoryFile(file string, limit int) {
	m.historyFile = file
	m.historyLimit = limit

	// Reinitialize readline with the new history file and limit
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt,
		HistoryFile:     m.historyFile,
		HistoryLimit:    m.historyLimit,
		AutoComplete:    nil,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		FuncIsTerminal: func() bool {
			return true
		},
	})
	if err != nil {
		log.Fatalf("Error reinitializing readline: %v", err)
	}

	// Close the old readline instance and replace it with the new one
	if m.rl != nil {
		m.rl.Close()
	}
	m.rl = rl
}

// NewClientChat creates a new MQTT chat client.
func NewClientChat(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string,
	version string) *MqttClientChat {
	enableColor := true
	promptColor := prompt
	if enableColor {
		promptColor = fmt.Sprintf("%s%s%s", RED, prompt, NC) // Apply red color to the prompt
	}

	// Default values for the history file
	defaultHistoryFile := "/tmp/mqttchat_history.txt"
	defaultHistoryLimit := 1000

	// Create the MqttClientChat instance
	cc := MqttClientChat{
		rl:                  nil, // Will be initialized later
		uuid:                shortuuid.New(),
		customPrompt:        "",
		enableColor:         enableColor,
		printPromptManually: false,
		historyFile:         defaultHistoryFile,
		historyLimit:        defaultHistoryLimit,
	}

	// Configure the MQTT chat
	chat := NewChat(mqttOpts, rxTopic, txTopic, version)
	cc.MqttChat = chat
	chat.SetDataCallback(cc.OnDataRx)
	chat.worker.GetOpts().SetOrderMatters(true)
	cc.waitServerChan = make(chan bool)

	// Initialize readline with default values
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          promptColor,
		HistoryFile:     cc.historyFile,
		HistoryLimit:    cc.historyLimit,
		AutoComplete:    nil,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		FuncIsTerminal: func() bool {
			return true
		},
	})
	if err != nil {
		log.Fatalf("Error initializing readline: %v", err)
	}

	cc.rl = rl

	// Start the main task
	go cc.clientTask()

	return &cc
}

// readCloserWrapper wraps an io.Reader to make it compatible with io.ReadCloser.
type readCloserWrapper struct {
	io.Reader
}

// Close closes the underlying reader if it implements io.Closer.
func (r readCloserWrapper) Close() error {
	if closer, ok := r.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// NewClientChatWithCustomIO creates a new MQTT chat client with custom input/output.
func NewClientChatWithCustomIO(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string, version string,
	customIO ClientChatIO) *MqttClientChat {

	// Default values for the history file
	defaultHistoryFile := "/tmp/mqttchat_history.txt"
	defaultHistoryLimit := 1000

	// Create the MqttClientChat instance
	cc := MqttClientChat{
		uuid:                shortuuid.New(),
		enableColor:         false,
		printPromptManually: true,
		historyFile:         defaultHistoryFile,
		historyLimit:        defaultHistoryLimit,
	}

	// Configure the MQTT chat
	chat := NewChat(mqttOpts, rxTopic, txTopic, version)
	cc.MqttChat = chat
	chat.SetDataCallback(cc.OnDataRx)
	cc.waitServerChan = make(chan bool)

	// Wrap the io.Reader to make it compatible with io.ReadCloser
	wrappedReader := readCloserWrapper{Reader: customIO.Reader}

	// Configure readline with the provided io.Reader and io.Writer
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt,
		HistoryFile:     cc.historyFile,
		HistoryLimit:    cc.historyLimit,
		AutoComplete:    nil,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		Stdin:           wrappedReader,
		Stdout:          customIO.Writer,
		EnableMask:      false,
		FuncIsTerminal: func() bool {
			return false
		},
	})
	if err != nil {
		log.Fatalf("Error initializing readline: %v", err)
	}

	cc.rl = rl

	// Start the main task
	go cc.clientTask()

	return &cc
}
