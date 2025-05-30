package mqtt_shell

import (
	"errors"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/config"
	"github.com/freedreamer82/mqtt-shell/pkg/appconsole"
	"github.com/freedreamer82/mqtt-shell/pkg/info"
	"github.com/freedreamer82/mqtt-shell/pkg/mqttchat"
	"github.com/freedreamer82/mqtt-shell/pkg/mqttcp"
	"github.com/freedreamer82/mqtt-shell/pkg/mqttcp/mft"
	"github.com/freedreamer82/mqtt-shell/pkg/plugins/sshbridge"
	"github.com/freedreamer82/mqtt-shell/pkg/plugins/telnetbridge"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

func RunServer(mqttOpts *MQTT.ClientOptions, conf *config.Config) {
	log.Info("Starting server..")

	netIOpt := mqttchat.WithOptionNetworkInterface(conf.Network.Interface)

	topic := mqttchat.ServerTopic{RxTopic: conf.RxTopic, TxTopic: conf.TxTopic, BeaconRxTopic: conf.BeaconTopic, BeaconTxTopic: conf.BeaconRequestTopic}
	var chat *mqttchat.MqttServerChat

	if conf.TelnetBridgePlugin.Enabled {
		chat = mqttchat.NewServerChat(mqttOpts, topic, info.VERSION, netIOpt,
			telnetbridge.WithTelnetBridge(conf.TelnetBridgePlugin.MaxConnections, conf.TelnetBridgePlugin.Keyword),
			sshbridge.WithSSHBridge(conf.SSHBridgePlugin.MaxConnections, conf.SSHBridgePlugin.Keyword),
		)
		//mqttchat.WithOptionAutoCompleteDirs([]string{"/usr"}))
	} else {
		chat = mqttchat.NewServerChat(mqttOpts, topic, info.VERSION, netIOpt)
	}
	chat.Start()

	if conf.Cp.CpServerEnabled {
		time.Sleep(time.Second)
		mqttCpServer := mqttcp.NewMqttServerCp(mqttOpts, conf.Cp.Local2ServerTopic, conf.Cp.Server2LocalTopic, mqttcp.WithOptionMqttWorker(chat.Worker()))
		mqttCpServer.Start()
	}

	if conf.SSHConsole.Privatekey != "" {
		sshConsole := appconsole.NewMqttServerChatConsole(chat, conf.SSHConsole.Host, conf.SSHConsole.Port,
			conf.SSHConsole.Maxconns, conf.SSHConsole.Privatekey, conf.SSHConsole.TimeoutSec, conf.SSHConsole.Password)
		sshConsole.Start()
	}
}

func RunClient(mqttOpts *MQTT.ClientOptions, conf *config.Config) {
	log.Info("Starting client..")
	chat := mqttchat.NewClientChat(mqttOpts, conf.TxTopic, conf.RxTopic, info.VERSION)
	chat.Start()
}

func RunBeacon(mqttOpts *MQTT.ClientOptions, conf *config.Config) {
	log.Info("Starting beacon discovery..")
	discovery := mqttchat.NewBeaconDiscovery(mqttOpts, conf.BeaconRequestTopic,
		conf.BeaconResponseTopic, conf.TimeoutBeaconSec,
		config.BeaconConverter)
	discovery.Run(nil)
}

func printProgress(progressChan chan mft.MftProgress, mqttCpClient *mqttcp.MqttClientCp) {
	var lastProgress mft.MftProgress
	for p := range progressChan {
		lastProgress = p
		mqttCpClient.Printf("\rProgress: %d/%d frames (%.2f%%)", p.FrameReceived, p.FrameTotal, p.Percent)
	}
	mqttCpClient.Printf("\rProgress: %d/%d frames (%.2f%%)\nTransfer complete.\n", lastProgress.FrameReceived, lastProgress.FrameTotal, lastProgress.Percent)
}

func RunCopyLocalToRemote(mqttOpts *MQTT.ClientOptions, conf *config.Config) {
	mqttCpClient := mqttcp.NewMqttClientCp(mqttOpts, conf.Cp.Server2LocalTopic, conf.Cp.Local2ServerTopic)
	progressChan := make(chan mft.MftProgress, 200)

	go printProgress(progressChan, mqttCpClient)

	mqttCpClient.CopyLocalToRemote(conf.Copy.Local2Remote.Source, conf.Copy.Local2Remote.Destination, &progressChan)
}

func RunCopyRemoteToLocal(mqttOpts *MQTT.ClientOptions, conf *config.Config) {
	mqttCpClient := mqttcp.NewMqttClientCp(mqttOpts, conf.Cp.Server2LocalTopic, conf.Cp.Local2ServerTopic)
	progressChan := make(chan mft.MftProgress, 200)

	go printProgress(progressChan, mqttCpClient)

	mqttCpClient.CopyRemoteToLocal(conf.Copy.Remote2Local.Source, conf.Copy.Remote2Local.Destination, &progressChan)
}

func BuildMqttOpts(conf *config.Config) (*MQTT.ClientOptions, error) {
	if conf.Broker == "" {
		return nil, fmt.Errorf("broker required")
	} else if conf.BrokerPort == 0 {
		return nil, fmt.Errorf("broker port required")
	}

	brokerurl := conf.Broker
	var mqttOpts = MQTT.NewClientOptions()
	addr := fmt.Sprintf("tcp://%s:%d", brokerurl, conf.BrokerPort)
	log.Info("Connecting to : " + addr)
	mqttOpts.AddBroker(addr)
	user := conf.BrokerUser
	password := conf.BrokerPassword
	if user != "" && password != "" {
		mqttOpts.SetUsername(user)
		mqttOpts.SetPassword(password)
	}
	return mqttOpts, nil
}

func ValidateConf(command string, conf *config.Config) error {
	if command == "client" && conf.Id == "" {
		return errors.New("ID is necessary in client Mode")
	} else if strings.Contains(command, "copy") && conf.Id == "" {
		return errors.New("ID is necessary in copy Mode")
	}
	return nil
}
