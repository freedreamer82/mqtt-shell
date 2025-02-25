package appconsole

import (
	"fmt"
	"log"
	"os"
	"time"

	console "github.com/freedreamer82/go-console/pkg/console"
	"github.com/freedreamer82/mqtt-shell/pkg/info"
	"github.com/freedreamer82/mqtt-shell/pkg/mqttchat" // Import mqttchat
	"github.com/olekukonko/tablewriter"                // Import tablewriter
)

// MqttServerChatConsole manages an interactive console for the MQTT server.
type MqttServerChatConsole struct {
	mqttChat      *mqttchat.MqttServerChat // Instance of MqttServerChat
	console       *console.SSHConsole      // Instance of the console
	user          (map[string]string)      // User map user and password
	port          int                      // Port for the console
	host          string                   // Host for the console
	maxConnection int                      // Max connection for the console

}

// NewMqttServerChatConsole creates a new instance of MqttServerChatConsole.
func NewMqttServerChatConsole(mqttChat *mqttchat.MqttServerChat, host string, port int, maxconn int, pathkey string,
	timeoutSec int, password string) *MqttServerChatConsole {
	handler := &MqttServerChatConsole{
		mqttChat:      mqttChat,
		host:          host,
		port:          port,
		maxConnection: maxconn,
	}

	handler.user["root"] = password

	// Configure the console
	handler.console, _ = console.NewSSHConsoleWithPassword(
		pathkey,
		handler.user,
		console.WithOptionConsoleTimeout(time.Duration(timeoutSec)*time.Second))

	return handler
}

// Start starts the interactive console.
func (c *MqttServerChatConsole) Start() {
	log.Printf("MQTT console started on port %s. Type 'help' for a list of commands.\n", c.port)
	c.console.Start(c.host, c.port, c.maxConnection)
}

// handleCommand handles console commands.
func (c *MqttServerChatConsole) handleCommand(cmd string, args []string) {
	switch cmd {
	case "info":
		c.handleInfoCommand()
	case "clients":
		c.handleClientsCommand()
	case "help":
		c.handleHelpCommand()
	default:
		fmt.Println("Unrecognized command. Type 'help' for a list of commands.")
	}
}

// handleInfoCommand handles the "info" command.
func (c *MqttServerChatConsole) handleInfoCommand() {
	clients := c.mqttChat.GetClientsConnected()
	fmt.Printf("Mqtt-shell Server\nVERSION: %s,INFO: %s.\nConnected clients: %d\n",
		info.VERSION, info.INFO, len(clients))
}

// handleClientsCommand handles the "clients" command.
func (c *MqttServerChatConsole) handleClientsCommand() {
	clients := c.mqttChat.GetClientsConnected()

	// Create a new table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"UUID", "Directory", "Status", "Last Activity"})

	// Add rows to the table
	for clientUUID, state := range clients {
		status := "Connected"
		if time.Since(state.LastActive) > c.mqttChat.GetInactivityTimeout() {
			status = "Timeout"
		}
		table.Append([]string{
			clientUUID,
			state.CurrentDir,
			status,
			state.LastActive.Format(time.RFC3339),
		})
	}

	// Render the table
	table.Render()
}

// handleHelpCommand handles the "help" command.
func (c *MqttServerChatConsole) handleHelpCommand() {
	fmt.Println("Available commands:")
	fmt.Println("  info      - Show general server information")
	fmt.Println("  clients   - List all connected clients in a table")
	fmt.Println("  help      - Show this help message")
}
