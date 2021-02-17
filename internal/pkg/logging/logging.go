package logging

import (
	"io"
	"mqtt-shell/internal/pkg/config"
	"os"
	"time"

	"github.com/rotisserie/eris"
	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

func lumberjackLogger(c *config.LoggingFileConfig) io.Writer {
	// Lumberjack file size parameter should be expressed in Megabytes
	maxSizeMb := int(c.MaxSize / 1024 / 1024)

	return &lumberjack.Logger{
		Filename:   c.Filename,
		MaxSize:    maxSizeMb,
		MaxAge:     c.MaxAgeDays,
		MaxBackups: c.MaxBackups,
		LocalTime:  c.UseLocalTime,
		Compress:   c.Compress,
	}
}

func Setup(conf *config.LoggingConfig) {
	if !conf.Enabled {
		return
	}

	var formatter log.Formatter

	if conf.FormatAsJson {
		formatter = &log.JSONFormatter{}
	} else {
		formatter = &log.TextFormatter{
			FullTimestamp:   false,
			ForceColors:     conf.ForceColors,
			ForceQuote:      false,
			TimestampFormat: time.Stamp,
		}
	}

	log.SetFormatter(formatter)
	log.SetLevel(conf.Level)
	log.SetReportCaller(conf.ReportCaller)

	var outputs []io.Writer

	if conf.ToStderr {
		outputs = append(outputs, os.Stderr)
	} else {
		outputs = append(outputs, os.Stdout)
	}

	if conf.File.Enabled && conf.File.Filename != "" {
		outputs = append(outputs, lumberjackLogger(&conf.File))
	}

	log.SetOutput(io.MultiWriter(outputs...))
}

func LogError(err error, args ...interface{}) {
	fields := log.Fields{}
	fields["error"] = eris.ToJSON(err, true)
	log.WithFields(fields).Error(args...)
}
