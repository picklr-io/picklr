package logging

import (
	"log/slog"
	"os"
	"strings"
)

var logger *slog.Logger

// Init initializes the global structured logger.
func Init(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: lvl,
	})
	logger = slog.New(handler)
	slog.SetDefault(logger)
}

// Logger returns the global logger instance.
func Logger() *slog.Logger {
	if logger == nil {
		Init("info")
	}
	return logger
}

// Debug logs a debug message.
func Debug(msg string, args ...any) {
	Logger().Debug(msg, args...)
}

// Info logs an info message.
func Info(msg string, args ...any) {
	Logger().Info(msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	Logger().Warn(msg, args...)
}

// Error logs an error message.
func Error(msg string, args ...any) {
	Logger().Error(msg, args...)
}
