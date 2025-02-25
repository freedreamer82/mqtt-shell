package appconsole

import (
	"fmt"

	"github.com/freedreamer82/go-console/pkg/console"
	"github.com/freedreamer82/mqtt-shell/pkg/info"
)

const CommandInfo = "info"
const CommandInfoHelp = "#info -> print info"

type InfoCommand struct {
	console.ConsoleCommand
}

func (i *InfoCommand) handler() string {
	return fmt.Sprintf("%s version: %s ", info.INFO, info.VERSION)
}

func NewInfoCommand() *console.ConsoleCommand {

	info := InfoCommand{}
	var c = console.NewConsoleCommand(
		CommandInfo,
		func(c *console.Console, command *console.ConsoleCommand, args []string) console.CommandError {
			c.Print(info.handler())
			return console.N0_ERR
		},
		CommandInfoHelp,
	)
	info.ConsoleCommand = *c
	return &info.ConsoleCommand
}
