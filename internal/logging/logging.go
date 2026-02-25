package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Setup initializes structured logging with the given level string.
// Valid levels: "debug", "info", "warn", "error". Defaults to "info".
// An optional writer may be supplied; when nil or absent, logs go to os.Stdout.
// Only the first writer is used; additional values are ignored.
func Setup(level string, writers ...io.Writer) {
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

	w := io.Writer(os.Stdout)
	if len(writers) > 0 && writers[0] != nil {
		w = writers[0]
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
}

// ForPlugin returns a logger with the plugin name attached.
func ForPlugin(name string) *slog.Logger {
	return slog.With("plugin", name)
}
