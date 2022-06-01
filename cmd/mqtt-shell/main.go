package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/bundle"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/constant"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/locale"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/screens"
	"github.com/freedreamer82/mqtt-shell/pkg/mqttchat"

	"github.com/freedreamer82/mqtt-shell/internal/pkg/config"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/logging"

	"github.com/alecthomas/kong"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var CLI config.CLI

func rungui() {

	myApp := app.NewWithID(constant.APP_ID)
	window := myApp.NewWindow(locale.AppWindowName)

	window.CenterOnScreen()
	window.SetIcon(bundle.ResourceMqttShellMidResolutionPng)
	window.Resize(fyne.NewSize(constant.MainWindowW, constant.MainWindowH))

	app := screens.NewMainScreen(myApp, window)

	window.SetContent(app.GetContainer())

	window.ShowAndRun()

}

func main() {

	kong.Parse(&CLI,
		kong.Name("mqtt-shell"),
		kong.Description("A simple mqtt client/server terminal"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"version": constant.INFO + " - " + constant.VERSION,
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

	if conf.Mode == "gui" {
		rungui()
	}

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
		chat := mqttchat.NewServerChat(&mqttOpts, conf.RxTopic, conf.TxTopic, constant.VERSION, mqttchat.WithOptionBeaconTopic(conf.BeaconTopic, conf.BeaconRequestTopic))
		chat.Start()
	} else if conf.Mode == "client" {

		log.Info("Starting client..")
		chat := mqttchat.NewClientChat(&mqttOpts, conf.TxTopic, conf.RxTopic, constant.VERSION)
		chat.Start()
	} else if conf.Mode == "beacon" {

		log.Info("Starting beacon discovery..")
		discovery := mqttchat.NewBeaconDiscovery(&mqttOpts, conf.BeaconRequestTopic, conf.BeaconResponseTopic, conf.TimeoutBeaconSec,
			config.BeaconConverter)
		discovery.Run(nil)
		return
	}

	select {} //wait forever
}
