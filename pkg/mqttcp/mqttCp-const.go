package mqttcp

import (
	"fmt"
	"time"
)

const (
	defaultHandshakeTimeout    = 3 * time.Second
	defaultTransmissionTimeout = 10 * time.Second
)

const (
	defaultServerMaxConnections          = 5
	defaultServerTimeoutConnection       = time.Hour
	defaultServerCheckConnectionInterval = 10 * time.Minute
)

type MqttCpCommand string

const (
	MqttCpCommand_CopyLocalToRemote = "local2remote"
	MqttCpCommand_CopyRemoteToLocal = "remote2local"
)

type MqttCpStep string

const (
	MqttCpStep_Handshake1 = "handshake-p1"
	MqttCpStep_Handshake2 = "handshake-p2"
	MqttCpStep_Start      = "start"
	MqttCpStep_End        = "end"
)

const (
	MqttCpMftTopic = "/mft/%s/%s"
)

func mftTopic(clientUUID, transferUUID string) string {
	return fmt.Sprintf(MqttCpMftTopic, clientUUID, transferUUID)
}
