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
	currentServerPath   string             // Current server path
	autocompleteChan    chan []string      // Channel for autocompletion options

}

// print prints the given arguments to the readline output.
func (m *MqttClientChat) print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(m.rl.Stdout(), a...)
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
	// Check if the message is for this client
	if data.ClientUUID != m.uuid {
		return true
	}

	// Check if the message has a valid CmdUUID
	if data.CmdUUID == "" {
		return true
	}

	return false
}

// IsServerConnected checks if the MQTT client is connected to the server.
func (m *MqttClientChat) IsServerConnected() bool {
	return m.worker.GetMqttClient().IsConnected()
}

// OnDataRx handles incoming MQTT data.
func (m *MqttClientChat) OnDataRx(data MqttJsonData) {
	if !m.IsServerConnected() {
		m.print("Server disconnected. Please reconnect.\n")
		return
	}

	if m.IsDataInvalid(data) {
		log.Debugf("Invalid message received: ClientUUID=%s, CmdUUID=%s", data.ClientUUID, data.CmdUUID)
		return
	}

	out := strings.TrimSuffix(data.Data, "\n") // Remove newline
	out = strings.TrimSpace(out)
	m.customPrompt = data.CustomPrompt
	m.currentServerPath = data.CurrentPath

	if strings.HasPrefix(out, "autocomplete:") {
		// Handle autocompletion response
		options := strings.TrimPrefix(out, "autocomplete:")
		optionList := strings.Split(options, "\n")

		// Send the options to the autocompletion channel
		m.autocompleteChan <- optionList
	} else {
		// Handle normal output
		m.print(out + "\n")
		m.printPrompt()
	}

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
	m.currentServerPath = data.CurrentPath
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
	} else {
		rlprompt := m.currentServerPath + " "
		if m.customPrompt == "" {
			rlprompt += prompt + " "
		} else {
			rlprompt += m.customPrompt + " "
		}
		if m.enableColor {
			rlprompt = fmt.Sprintf("%s%s%s", RED, rlprompt, NC) // Apply red color to the prompt

		}
		m.rl.SetPrompt(rlprompt)
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
			log.Info("Server not responding. Retrying...")
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
	/* 	m.rl.Config.SetListener(func(line []rune, pos int, key rune) (newLine []rune, newPos int, ok bool) {
		if key == '\t' { // Intercept the Tab key
			//	fmt.Print("Tab not supported , sorry")
			m.Transmit(fmt.Sprintf("autocomplete %s", string(line)), "", m.uuid)
			return nil, 0, false // Stop input
		}
		return line, pos, true // Continue input
	}) */

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
		AutoComplete:    cc.setupDynamicAutocompletion(),
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

/* func (m *MqttClientChat) setupDynamicAutocompletion() readline.AutoCompleter {
	m.autocompleteChan = make(chan []string, 100)

	completer := readline.NewPrefixCompleter(
		readline.PcItemDynamic(func(line string) []string {
			// Estrai la parte del percorso dopo il comando (ad esempio, dopo "ls ")
			parts := strings.SplitN(line, " ", 2)
			if len(parts) < 2 {
				// Se non c'è uno spazio, considera il percorso come vuoto (directory corrente)
				parts = append(parts, ".")
			}
			pathPart := strings.TrimSpace(parts[1])

			// Invia solo la parte del percorso al server
			m.Transmit(fmt.Sprintf("autocomplete %s", pathPart), "", m.uuid)

			// Attendi le opzioni dal server
			select {
			case options := <-m.autocompleteChan:
				return options
			case <-time.After(2 * time.Second): // Timeout dopo 2 secondi
				return []string{}
			}
		}),
	)

	return completer
}
*/

type dynamicCompleter struct {
	getOptions func(line string) []string
}

/*
	 func (d *dynamicCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
		options := d.getOptions(string(line))
		var result [][]rune
		for _, opt := range options {
			result = append(result, []rune(opt))
		}
		return result, 0 //len(line)
	}
*/

func (d *dynamicCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	if pos < 0 || pos > len(line) { // Ensure pos is valid
		return nil, 0
	}

	options := d.getOptions(string(line))
	var result [][]rune
	for _, opt := range options {
		result = append(result, []rune(opt))
	}

	if len(result) > 0 {
		commonPrefix := result[0]
		for _, opt := range result {
			commonPrefix = commonPrefix[:commonPrefixLength(commonPrefix, opt)]
		}
		length = len(commonPrefix)
	} else {
		length = 0
	}

	return result, length
}

// Funzione per calcolare la lunghezza del prefisso comune tra due slice di rune
func commonPrefixLength(a, b []rune) int {
	minLength := len(a)
	if len(b) < minLength {
		minLength = len(b)
	}
	for i := 0; i < minLength; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return minLength
}

/*
func (d *dynamicCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	// Converte il runes in string
	input := string(line)
	// Ottiene le opzioni di completamento
	options := d.getOptions(input)
	var result [][]rune

	// Elimina il comando dalla riga di input
	parts := strings.Fields(input)
	if len(parts) > 1 {
		// Solo se ci sono più parti, consideriamo il percorso
		pathPart := strings.TrimSpace(parts[1])
		// Confronta le opzioni con il percorso parziale
		for _, opt := range options {
			if strings.HasPrefix(opt, pathPart) { // Controlla se l'opzione inizia con il percorso
				result = append(result, []rune(opt)) // Aggiungi solo l'opzione se corrisponde
			}
		}
	}

	return result, len(line)
}
*/

func (m *MqttClientChat) setupDynamicAutocompletion() readline.AutoCompleter {
	m.autocompleteChan = make(chan []string, 100)

	completer := &dynamicCompleter{
		getOptions: func(line string) []string {
			// Estrai la parte del percorso dopo il comando (ad esempio, dopo "ls ")
			parts := strings.SplitN(line, " ", 2)
			//last word
			pathPart := strings.TrimSpace(parts[len(parts)-1])

			// Svuota il canale prima di inviare una nuova richiesta
			for len(m.autocompleteChan) > 0 {
				<-m.autocompleteChan
			}

			// Invia solo la parte del percorso al server
			m.Transmit(fmt.Sprintf("autocomplete %s", pathPart), "", m.uuid)

			// Attendi le opzioni dal server
			select {
			case options := <-m.autocompleteChan:
				return options
			case <-time.After(5 * time.Second): // Timeout dopo 2 secondi
				return []string{}
			}
		},
	}

	return completer
}
