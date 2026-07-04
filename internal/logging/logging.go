// Package logging configures the global zerolog logger with colored console output.
package logging

import (
	"os"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

func Setup(level string, noColor bool) {
	var l zerolog.Level
	switch level {
	case "debug":
		l = zerolog.DebugLevel
	case "warn":
		l = zerolog.WarnLevel
	case "error":
		l = zerolog.ErrorLevel
	default:
		l = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(l)
	Log = newLogger(noColor)
}

func newLogger(noColor bool) zerolog.Logger {
	output := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		NoColor:    noColor,
		TimeFormat: "\033[90m15:04:05\033[0m",
		PartsOrder: []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
			zerolog.MessageFieldName,
		},
		FormatLevel: func(i interface{}) string {
			if noColor {
				if s, ok := i.(string); ok {
					return s
				}
				return "????"
			}
			level := "????"
			if s, ok := i.(string); ok {
				switch s {
				case "debug":
					level = "\033[36mDEBUG\033[0m"
				case "info":
					level = "\033[32mINFO \033[0m"
				case "warn":
					level = "\033[33mWARN \033[0m"
				case "error":
					level = "\033[31mERROR\033[0m"
				case "fatal":
					level = "\033[31mFATAL\033[0m"
				}
			}
			return level
		},
		FormatMessage: func(i interface{}) string {
			if s, ok := i.(string); ok {
				return "  " + s
			}
			return ""
		},
	}
	return zerolog.New(output).With().Timestamp().Logger()
}

func init() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	Log = newLogger(false)
}
