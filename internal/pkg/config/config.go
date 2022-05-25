package config

import (
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/dustin/go-humanize"
	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	// General
	envPrefix         = "MQTT_SHELL_"
	defaultConfigName = "config"

	// Logging
	defaultLogFileMaxSize = 10 * 1024 * 1024 // 10 Megabytes

)
const INFO = "mqtt-shell\r\nSw Engineer: Marco Garzola"
const VERSION = "0.0.5"

type CLI struct {
	ConfigFile     string           `short:"c" xor:"config" type:"existingfile"`
	Verbose        bool             `short:"d" help:"verbose log"`
	Broker         string           `short:"b" xor:"flags" help:"Broker URL"`
	BrokerUser     string           `short:"u" help:"broker user" `
	BrokerPassword string           `short:"P" help:"broker password" `
	BrokerPort     int              `short:"p" help:"broker port"`
	Version        kong.VersionFlag `short:"v" xor:"flags"`
	Id             string           `short:"i" help:"node id"`
	Mode           string           `short:"m" enum:"client,server,beacon" default:"client" help:"client, server or beacon,default client"`
}

var (
	defaultConfigPaths = []string{}
)

type FileSize uint64

type LoggingFileConfig struct {
	// Enabled determines if file logging should be enabled.
	Enabled bool

	// Filename is the file to write logs to. Backup logs will be retained in
	// the same directory.
	Filename string

	// MaxSize is the maximum size of the log file before it gets rotated.
	MaxSize FileSize

	// MaxBackups is the maximum number of old log files to retain.
	MaxBackups int

	// MaxAgeDays is the maximum number of days to retain old log files.
	MaxAgeDays int

	// UseLocalTime determines if the time used for formatting the timestamps in
	// backup files is the computer's local time. If false, UTC is used.
	UseLocalTime bool

	// Compress determines if the rotated log files should be compressed
	// using gzip.
	Compress bool
}

type LoggingConfig struct {
	// Enabled determines if logging should be enabled.
	Enabled bool

	// ToStderr determines if log output should be directed to
	// standard error. If false, standard output is used instead.
	ToStderr bool

	// Level is the logger level.
	Level logrus.Level

	// ReportCaller sets whether the standard logger will include the calling
	// method as a field.
	ReportCaller bool

	// FormatAsJson determines if the logging output should be formatted as
	// parsable JSON.
	FormatAsJson bool

	// ForceColors disables checking for a TTY before outputting colors.
	// This will force all output to be colored.
	ForceColors bool

	// File is the logging file configuration
	File LoggingFileConfig
}

type Config struct {
	CLI
	// Logging is the logging configuration
	Logging             LoggingConfig
	TxTopic             string
	RxTopic             string
	BeaconTopic         string
	BeaconRequestTopic  string
	BeaconResponseTopic string
	TimeoutBeaconSec    uint64
}

/// NewConfig creates a new configuration structure
/// filled with default options
func NewConfig() Config {
	_, addr := getNetInfo()
	return Config{
		CLI:                 CLI{BrokerPort: 1883, Mode: "client"},
		Logging:             NewLoggingConfig(),
		TxTopic:             getTxTopic(addr),
		RxTopic:             getRxTopic(addr),
		BeaconTopic:         getBeaconTopic(addr),
		BeaconRequestTopic:  BeaconRequestTopic,
		BeaconResponseTopic: BeaconReplyTopic, //getBeaconTopic("+"),
		TimeoutBeaconSec:    10,
	}
}

/// NewLoggingConfig creates a new logging configuration structure
/// filled with default options
func NewLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Enabled:      true,
		ToStderr:     true,
		Level:        logrus.InfoLevel,
		ReportCaller: false,
		FormatAsJson: false,
		File:         NewLoggingFileConfig(),
	}
}

/// NewLoggingFileConfig creates a new logging file config structure
/// filed with default parameters
func NewLoggingFileConfig() LoggingFileConfig {
	return LoggingFileConfig{
		Enabled:      false,
		Filename:     "",
		MaxSize:      defaultLogFileMaxSize,
		MaxBackups:   3,
		MaxAgeDays:   14,
		UseLocalTime: false,
		Compress:     false,
	}
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func mergeCliandConfig(config *Config, cli *CLI) {
	mergo.Merge(&config.CLI, cli, mergo.WithOverride)
}

/// Parse loads the configuration
/// using a pre initialized viper object
func Parse(v *viper.Viper, configFile string, cli *CLI) (*Config, error) {
	var err error

	v.SetEnvPrefix(envPrefix)
	v.AllowEmptyEnv(true)
	v.AutomaticEnv()

	if configFile != "" && fileExists(configFile) {
		v.SetConfigFile(configFile)
		err = v.ReadInConfig()
		if err != nil {
			return nil, eris.Wrap(err, "failed to read configuration file")
		}
	} else if configFile == "" {
		//do nothing fill the strut

	} else {
		return nil, eris.New("file not valid")
	}

	config := NewConfig()

	// Create decode hooks to parse custom configuration types such as
	// logrus LogLevel or FileSize
	decodeHook := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		stringToLogLevelHookFunc(),
		stringToFileSizeHookFunc(),
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToIPHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	))

	err = v.UnmarshalExact(&config, decodeHook)
	// err = v.Unmarshal(&config)

	if err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal configuration file")
	}

	mergeCliandConfig(&config, cli)

	if config.Id != "" {
		config.TxTopic = getTxTopic(config.Id)
		config.RxTopic = getRxTopic(config.Id)
		config.BeaconTopic = getBeaconTopic(config.Id)
		config.BeaconRequestTopic = BeaconRequestTopic
		config.BeaconResponseTopic = getBeaconTopic("+")
	}

	return &config, nil
}

/// stringToFileSizeHookFunc is a mapstructure decode hook
/// which decodes strings to file sizes
func stringToFileSizeHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String || t != reflect.TypeOf(FileSize(0)) {
			return data, nil
		}
		size, err := humanize.ParseBytes(data.(string))
		if err != nil {
			return nil, err
		}
		return FileSize(size), nil
	}
}

/// stringToLogLevelHookFunc is a mapstructure decode hook
/// which decodes strings to log levels
func stringToLogLevelHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String || t != reflect.TypeOf(logrus.DebugLevel) {
			return data, nil
		}
		return logrus.ParseLevel(data.(string))
	}
}

const topicPrefix = "/mqtt-shell/"

var TemplateSubTopic = topicPrefix + "%s/cmd"
var TemplateSubTopicreply = topicPrefix + "%s/cmd/res"
var TemplateBeaconTopic = topicPrefix + "%s/event"

const BeaconRequestTopic = topicPrefix + "whoami"
const BeaconReplyTopic = topicPrefix + "+/event"

func getNetInfo() (string, string) {

	interfaces, _ := net.Interfaces()
	for _, interf := range interfaces {

		if addrs, err := interf.Addrs(); err == nil {
			for index, addr := range addrs {
				log.Debug("[", index, "]", interf.Name, ">", addr)
				if interf.Name != "lo" {
					name := interf.Name
					macAddress := interf.HardwareAddr

					log.Debug("Hardware name : ", name)
					log.Debug("MAC address : ", macAddress)
					netI := name
					nodeID := strings.ReplaceAll(macAddress.String(), ":", "")
					return netI, nodeID
				}
			}
		}
	}
	return "", ""
}

func getTxTopic(nodeID string) string {
	topic := fmt.Sprintf(TemplateSubTopicreply, nodeID)
	return topic
}

func getRxTopic(nodeID string) string {
	topic := fmt.Sprintf(TemplateSubTopic, nodeID)
	return topic
}

func getBeaconTopic(nodeID string) string {
	topic := fmt.Sprintf(TemplateBeaconTopic, nodeID)
	return topic
}

func BeaconConverter(topic string) string {
	res := strings.ReplaceAll(topic, topicPrefix, "")
	split := strings.Split(res, "/")
	if len(split) > 0 {
		return split[0]
	}
	return ""
}
