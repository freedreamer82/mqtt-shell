package appconsole

// import (
// 	"os"
// 	"time"

// 	"github.com/freedreamer82/go-console/pkg/console"
// 	"github.com/olekukonko/tablewriter"
// )

// const CommandInfo = "clients"
// const CommandInfoHelp = "clients   - List all connected clients in a table"

// type ClientsCommand struct {
// 	console.ConsoleCommand
// }

// func (c *ClientsCommand) handler() string {
// 	clients := c.mqttChat.GetClientsConnected()

// 	// Create a new table
// 	table := tablewriter.NewWriter(os.Stdout)
// 	table.SetHeader([]string{"UUID", "Directory", "Status", "Last Activity"})

// 	// Add rows to the table
// 	for clientUUID, state := range clients {
// 		status := "Connected"
// 		if time.Since(state.LastActive) > c.mqttChat.GetInactivityTimeout() {
// 			status = "Timeout"
// 		}
// 		table.Append([]string{
// 			clientUUID,
// 			state.CurrentDir,
// 			status,
// 			state.LastActive.Format(time.RFC3339),
// 		})
// 	}

// 	// Render the table
// 	table.Render()
// }

// func NewClientsCommandCommand() *console.ConsoleCommand {

// 	info := ClientsCommand{}
// 	var c = console.NewConsoleCommand(
// 		CommandInfo,
// 		func(c *console.Console, command *console.ConsoleCommand, args []string) console.CommandError {
// 			c.Print(info.handler())
// 			return console.N0_ERR
// 		},
// 		CommandInfoHelp,
// 	)
// 	info.ConsoleCommand = *c
// 	return &info.ConsoleCommand
// }
