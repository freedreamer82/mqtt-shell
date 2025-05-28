package sshbridge

import "strings"

const helpText = "%s <user@host> [password] [-p port] [-i keyfile] [--raw]\n" +
	"- Connect via SSH. Options:\n" +
	"  -p <port>     : specify port (default 22)\n" +
	"  -i <keyfile>  : use private key authentication\n" +
	"  --raw         : enable raw mode (interactive shell, line-by-line reading)\n" +
	"Examples:\n" +
	"  %s user@host password\n" +
	"  %s user@host -i /path/to/keyfile --raw\n" +
	"  %s user@host password -p 2222 --raw"

const errorText = "***: command not valid, try > *** help"

func getSSHHelpText(pluginName string) string {
	return strings.Replace(helpText, "***", pluginName, -1)
}

func getSSHErrorText(pluginName string) string {
	return strings.Replace(errorText, "***", pluginName, -1)
}
