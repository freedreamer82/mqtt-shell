package main

import (
	"fmt"
	mqttshell "github.com/freedreamer82/mqtt-shell/internal/app/mqtt-shell"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/config"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/logging"
	"github.com/freedreamer82/mqtt-shell/pkg/info"

	"github.com/alecthomas/kong"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var CLI config.CLI

func main() {

	ctx := kong.Parse(&CLI,
		kong.Name("mqtt-shell"),
		kong.Description("A simple mqtt client/server terminal"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"version": info.INFO + " - " + info.VERSION,
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

	errConf := mqttshell.ValidateConf(ctx.Command(), conf)
	if errConf != nil {
		fmt.Println(errConf.Error())
		return
	}

	if CLI.Verbose {
		conf.Logging.Level = log.TraceLevel
	}
	logging.Setup(&conf.Logging)

	mqttOpts, errOpts := mqttshell.BuildMqttOpts(conf)
	if errOpts != nil {
		fmt.Println(errOpts.Error())
		return
	}

	if ctx.Command() == "server" {
		mqttshell.RunServer(mqttOpts, conf)
	} else if ctx.Command() == "client" {
		mqttshell.RunClient(mqttOpts, conf)
	} else if ctx.Command() == "beacon" {
		mqttshell.RunBeacon(mqttOpts, conf)
		return
	} else if ctx.Command() == "copy local-2-remote" {
		mqttshell.RunCopyLocalToRemote(mqttOpts, conf)
		return
	} else if ctx.Command() == "copy remote-2-local" {
		mqttshell.RunCopyRemoteToLocal(mqttOpts, conf)
		return
	}

	select {} //wait forever
}
