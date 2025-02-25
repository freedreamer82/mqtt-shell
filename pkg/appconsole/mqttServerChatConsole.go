package appconsole

import (
	"log"
	"time"

	"github.com/freedreamer82/go-console/pkg/console"
	appconsole "github.com/freedreamer82/mqtt-shell/pkg/appconsole/handlers"
	chat "github.com/freedreamer82/mqtt-shell/pkg/mqttchat" // Import mqttchat
)

// MqttServerChatConsole manages an interactive console for the MQTT server.
type MqttServerChatConsole struct {
	mqttChat      *chat.MqttServerChat // Instance of MqttServerChat
	console       *console.SSHConsole  // Instance of the console
	user          (map[string]string)  // User map user and password
	port          int                  // Port for the console
	host          string               // Host for the console
	maxConnection int                  // Max connection for the console
	cmds          []*console.ConsoleCommand
}

// NewMqttServerChatConsole creates a new instance of MqttServerChatConsole.
func NewMqttServerChatConsole(mqttChat *chat.MqttServerChat, host string, port int, maxconn int, pathkey string,
	timeoutSec int, password string) *MqttServerChatConsole {
	handler := &MqttServerChatConsole{
		mqttChat:      mqttChat,
		host:          host,
		port:          port,
		maxConnection: maxconn,
		user:          make(map[string]string),
	}

	handler.user["root"] = password

	info := appconsole.NewInfoCommand()
	handler.cmds = append(handler.cmds, info)

	var err error
	// Configure the console
	handler.console, err = console.NewSSHConsoleWithPassword(
		pathkey,
		handler.user,
		console.WithOptionConsoleTimeout(time.Duration(timeoutSec)*time.Second))
	if err != nil {
		return nil
	}

	handler.console.AddCallbackOnNewConsole(func(c *console.Console) {
		commands := handler.cmds
		for _, comm := range commands {
			c.AddConsoleCommand(comm)
		}
	})

	return handler
}

// Start starts the interactive console.
func (c *MqttServerChatConsole) Start() {
	log.Printf("MQTT console started on port %s. Type 'help' for a list of commands.\n", c.port)
	c.console.Start(c.host, c.port, c.maxConnection)
}
