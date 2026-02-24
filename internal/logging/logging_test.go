package logging

import (
	"log/slog"
	"testing"
)

func TestSetupLevels(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
		{"DEBUG", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
	} {
		t.Run(tc.input, func(t *testing.T) {
			Setup(tc.input)
			l := ForPlugin("test")
			if l == nil {
				t.Fatal("ForPlugin returned nil")
			}
		})
	}
}

func TestForPluginIncludesName(t *testing.T) {
	Setup("info")
	l := ForPlugin("myplug")
	if l == nil {
		t.Fatal("ForPlugin returned nil")
	}
}
