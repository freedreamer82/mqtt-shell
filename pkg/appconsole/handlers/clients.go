package appconsole

import (
	"bytes"
	"github.com/freedreamer82/go-console/pkg/console"
	"github.com/freedreamer82/mqtt-shell/pkg/mqttchat"
	"github.com/olekukonko/tablewriter"
	"time"
)

const CommandClients = "clients"
const CommandClientsHelp = "clients   - List all connected clients in a table"

type ClientsCommand struct {
	console.ConsoleCommand
	mqttServer *mqttchat.MqttServerChat
}

func (c *ClientsCommand) handler() string {
	clients := c.mqttServer.GetClientsConnected()

	// Usa un buffer invece di os.Stdout
	var buffer bytes.Buffer
	table := tablewriter.NewWriter(&buffer)
	table.Header([]string{"UUID", "Directory", "Status", "Last Activity"})

	// Aggiungi le righe alla tabella
	for clientUUID, state := range clients {
		status := "Connected"
		if time.Since(state.LastActive) > c.mqttServer.GetInactivityTimeout() {
			status = "Timeout"
		}
		table.Append([]string{
			clientUUID,
			state.CurrentDir,
			status,
			state.LastActive.Format(time.RFC3339),
		})
	}

	// Genera la tabella come stringa
	table.Render()

	return buffer.String() // Restituisce la stringa invece di stamparla
}
func NewClientsCommandCommand(mqttServerChat *mqttchat.MqttServerChat) *console.ConsoleCommand {

	cl := ClientsCommand{mqttServer: mqttServerChat}

	var c = console.NewConsoleCommand(
		CommandClients,
		func(c *console.Console, command *console.ConsoleCommand, args []string) console.CommandError {
			c.Print(cl.handler())
			return console.N0_ERR
		},
		CommandClientsHelp,
	)
	cl.ConsoleCommand = *c
	return &cl.ConsoleCommand
}
