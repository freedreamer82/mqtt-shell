package main

import (
	"fmt"

	"github.com/freedreamer82/mqtt-shell/internal/pkg/config"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/logging"

	mqtt "github.com/freedreamer82/mqtt-shell/internal/pkg/mqtt2shell"

	"github.com/alecthomas/kong"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const INFO = "mqtt-shell\r\nSw Engineer: Marco Garzola"
const VERSION = "0.0.5"

var CLI config.CLI

func main() {

	kong.Parse(&CLI,
		kong.Name("mqtt-shell"),
		kong.Description("A simple mqtt client/server terminal"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"version": INFO + " - " + VERSION,
		})

	v := viper.New()

	formatter := &logrus.TextFormatter{
		FullTimestamp: true,
	}
	logrus.SetFormatter(formatter)

	conf, err := config.Parse(v, CLI.ConfigFile, &CLI)
	if err != nil {
		log.Panicf("Failed to parse configuration file: %s", eris.ToString(err, true))
		return
	}

	if conf.Mode == "client" && conf.Id == "" {
		fmt.Println("ID is necessary in client Mode")
		return
	}

	if CLI.Verbose {
		conf.Logging.Level = log.TraceLevel
	}
	logging.Setup(&conf.Logging)

	if conf.Broker == "" {
		fmt.Println("Broker required: ")
		return
	}

	brokerurl := conf.Broker
	var mqttOpts MQTT.ClientOptions
	addr := fmt.Sprintf("tcp://%s:%d", brokerurl, conf.BrokerPort)
	log.Info("Connecting to : " + addr)
	mqttOpts.AddBroker(addr)
	user := conf.BrokerUser
	password := conf.BrokerPassword
	if user != "" && password != "" {
		mqttOpts.SetUsername(user)
		mqttOpts.SetPassword(password)
	}

	if conf.Mode == "server" {
		log.Info("Starting server..")
		mqtt.NewServerChat(&mqttOpts, conf.RxTopic, conf.TxTopic, VERSION, mqtt.WithOptionBeaconTopic(conf.BeaconTopic, conf.BeaconRequestTopic))
	} else if conf.Mode == "client" {

		log.Info("Starting client..")
		mqtt.NewClientChat(&mqttOpts, conf.TxTopic, conf.RxTopic, VERSION)
	} else if conf.Mode == "beacon" {

		log.Info("Starting beacon discovery..")
		discovery := mqtt.NewBeaconDiscovery(&mqttOpts, conf.BeaconRequestTopic, conf.BeaconResponseTopic, conf.TimeoutBeaconSec, config.BeaconConverter)
		discovery.Run()
		return
	}

	select {} //wait forever
}
