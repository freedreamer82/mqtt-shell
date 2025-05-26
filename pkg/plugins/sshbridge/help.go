package sshbridge

import "strings"

const helpText = "Mqtt 2 SSH Bridge:\n" +
	" *** list -> show all active SSH connections\n" +
	" *** {user}@{host} [password] [-p {port}] -> open SSH connection with password (default port 22, password will be requested if omitted)\n" +
	" *** {user}@{host} -i {key_path} [-p {port}] -> open SSH connection with private key\n" +
	" *** disconnect -> close SSH connection\n" +
	" *** help -> show this help message"

const errorText = "***: command not valid, try > *** help"

func getSSHHelpText(pluginName string) string {
	return strings.Replace(helpText, "***", pluginName, -1)
}

func getSSHErrorText(pluginName string) string {
	return strings.Replace(errorText, "***", pluginName, -1)
}
