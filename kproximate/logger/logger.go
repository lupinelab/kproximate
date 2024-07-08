package logger

import (
	"log/slog"
	"os"
)

// A default logger for tests to use, this will be replaced when
// ConfigureLogger is called by each component.
var Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
var logArgs []any

func init() {
}

func ConfigureLogger(component string, debug bool) {
	var err error
	hostname, err := os.Hostname()
	if err != nil {
		Logger.Error("Could not get hostname", "error", err)
	}

	logArgs = []any{
		"host", hostname,
		"component", component,
	}

	var level slog.Level
	if debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}

	Logger = slog.New(slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{
			Level: level,
		},
	))

	slog.SetDefault(Logger)
}

func DebugLog(msg string, args ...any) {
	args = append(logArgs, args...)
	Logger.Debug(msg, args...)
}

func InfoLog(msg string, args ...any) {
	args = append(logArgs, args...)
	Logger.Info(msg, args...)
}

func WarnLog(msg string, args ...any) {
	args = append(logArgs, args...)
	Logger.Warn(msg, args...)
}

func ErrorLog(msg string, args ...any) {
	args = append(logArgs, args...)
	Logger.Error(msg, args...)
}

func FatalLog(msg string, err error) {
	Logger.Error(msg, "error", err)
	panic(err)
}
