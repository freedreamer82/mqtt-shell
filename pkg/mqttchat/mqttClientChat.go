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

type ClientChatIO struct {
	io.Reader
	io.Writer
}

type MqttClientChat struct {
	*MqttChat
	waitServerChan      chan bool
	rl                  *readline.Instance // Istanza di readline per gestire l'input
	uuid                string
	customPrompt        string
	enableColor         bool
	printPromptManually bool
}

func (m *MqttClientChat) print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(m.rl.Stdout(), a...)
}

func (m *MqttClientChat) println() (n int, err error) {
	return fmt.Fprintln(m.rl.Stdout())
}

func (m *MqttClientChat) printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(m.rl.Stdout(), format, a...)
}

func (m *MqttClientChat) printWithoutLn(a ...interface{}) (n int, err error) {
	return fmt.Fprint(m.rl.Stdout(), a...)
}

func (m *MqttClientChat) IsDataInvalid(data MqttJsonData) bool {
	return data.CmdUUID == "" || data.Cmd == "" || data.Data == "" || data.ClientUUID != m.uuid
}

func (m *MqttClientChat) OnDataRx(data MqttJsonData) {
	if m.IsDataInvalid(data) {
		return
	}
	out := strings.TrimSuffix(data.Data, "\n") // Rimuove il newline
	m.customPrompt = data.CustomPrompt
	m.print(out)
	m.println()
	m.printPrompt()
}

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

func (m *MqttClientChat) printLogin(ip string, serverVersion string) {
	log.Info("Connected")
	m.printf(login, ip, serverVersion, m.version, m.uuid, m.txTopic, m.rxTopic)
	//m.printPrompt()
}

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

func (m *MqttClientChat) clearScreen() {
	m.print("\033[2J") // Clear dell'intero schermo
	m.print("\033[H")  // Riposiziona il cursore in alto a sinistra
}

func (m *MqttClientChat) clientTask() {
	m.waitServer()

	// Aggiungi un listener per intercettare il tasto Tab
	m.rl.Config.SetListener(func(line []rune, pos int, key rune) (newLine []rune, newPos int, ok bool) {
		if key == '\t' { // Intercetta il tasto Tab
			fmt.Print("Tab not supported , sorry")
			return nil, 0, false // Interrompi l'input
		}
		return line, pos, true // Continua l'input
	})

	for {
		m.printPrompt()
		line, err := m.rl.Readline()
		if err != nil { // Ctrl+D o Ctrl+C per uscire
			if err.Error() == "EOF" || err.Error() == "interrupt" {
				log.Info("Uscita...")
				break
			}
			fmt.Println("Goodbye!")
			os.Exit(0)
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue // Ignora le linee vuote
		} else if line == "clear" {
			m.clearScreen() // Usa la funzione clearScreen
			continue        // Non inviare il comando al server
		}

		// Invia il comando via MQTT
		m.Transmit(line, "", m.uuid)
	}
}

func (m *MqttClientChat) Close() {
	if m.rl != nil {
		m.rl.Close()
	}
}

func NewClientChat(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string,
	version string, opts ...MqttChatOption) *MqttClientChat {
	enableColor := true
	promptColor := prompt
	if enableColor {
		promptColor = fmt.Sprintf("%s%s%s", RED, prompt, NC) // Applica il colore rosso al prompt
	}

	// Configura readline per gestire l'input
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          promptColor,                 // Usa il prompt colorato
		HistoryFile:     "/tmp/mqttchat_history.txt", // Salva la cronologia in un file
		AutoComplete:    nil,                         // Disabilita il completamento automatico
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		FuncIsTerminal: func() bool {
			return true // Indica che siamo in un terminale
		},
	})
	if err != nil {
		log.Fatalf("Errore durante l'inizializzazione di readline: %v", err)
	}

	// Crea l'istanza di MqttClientChat
	cc := MqttClientChat{
		rl:                  rl, // Assegna l'istanza di readline
		uuid:                shortuuid.New(),
		customPrompt:        "",
		enableColor:         enableColor,
		printPromptManually: false,
	}

	// Configura il chat MQTT
	chat := NewChat(mqttOpts, rxTopic, txTopic, version, opts...)
	chat.SetDataCallback(cc.OnDataRx)
	cc.MqttChat = chat
	chat.worker.GetOpts().SetOrderMatters(true)
	cc.waitServerChan = make(chan bool)

	// Avvia il task principale
	go cc.clientTask()

	return &cc
}

type readCloserWrapper struct {
	io.Reader
}

func (r readCloserWrapper) Close() error {
	// Se il Reader sottostante implementa io.Closer, chiama Close()
	if closer, ok := r.Reader.(io.Closer); ok {
		return closer.Close()
	}
	// Altrimenti, non fare nulla
	return nil
}

func NewClientChatWithCustomIO(mqttOpts *MQTT.ClientOptions, rxTopic string, txTopic string, version string,
	customIO ClientChatIO, opts ...MqttChatOption) *MqttClientChat {

	// Wrapper per rendere l'io.Reader compatibile con io.ReadCloser
	wrappedReader := readCloserWrapper{Reader: customIO.Reader}

	// Configura readline con l'io.Reader e io.Writer forniti
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt, // Puoi personalizzare il prompt se necessario
		HistoryFile:     "/tmp/mqttchat_history.txt",
		AutoComplete:    nil,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		Stdin:           wrappedReader, // Usa il wrapper
		Stdout:          customIO.Writer,
		EnableMask:      false,
		FuncIsTerminal: func() bool {
			return false // Indica che non siamo in un terminale
		},
	})
	if err != nil {
		log.Fatalf("Errore durante l'inizializzazione di readline: %v", err)
	}

	cc := MqttClientChat{
		rl:                  rl,
		uuid:                shortuuid.New(),
		enableColor:         false,
		printPromptManually: true,
	}
	chat := NewChat(mqttOpts, rxTopic, txTopic, version, opts...)
	chat.SetDataCallback(cc.OnDataRx)
	cc.MqttChat = chat
	cc.waitServerChan = make(chan bool)
	go cc.clientTask()

	return &cc
}
