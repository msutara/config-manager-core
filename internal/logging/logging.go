package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Setup initializes structured logging with the given level string.
// Valid levels: "debug", "info", "warn", "error". Defaults to "info".
func Setup(level string) {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
}

// ForPlugin returns a logger with the plugin name attached.
func ForPlugin(name string) *slog.Logger {
	return slog.With("plugin", name)
}
