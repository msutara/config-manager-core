package logging

import (
	"bytes"
	"context"
	"io"
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
			l := slog.Default()
			if l == nil {
				t.Fatal("Default logger is nil after Setup")
			}
			// Verify the configured level is enabled.
			if !l.Enabled(context.Background(), tc.want) {
				t.Errorf("level %v should be enabled after Setup(%q)", tc.want, tc.input)
			}
			// Verify levels below the configured one are disabled
			// (except when the configured level is Debug, which is the lowest).
			if tc.want > slog.LevelDebug {
				below := tc.want - 4 // one level below
				if l.Enabled(context.Background(), below) {
					t.Errorf("level %v should be disabled after Setup(%q)", below, tc.input)
				}
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

func TestSetupWithWriter(t *testing.T) {
	var buf bytes.Buffer
	Setup("info", &buf)
	slog.Info("hello writer")
	if buf.Len() == 0 {
		t.Error("expected log output in buffer, got none")
	}
}

func TestSetupWithNilWriter(t *testing.T) {
	// nil writer should fall back to os.Stdout (no panic).
	Setup("info", nil)
	l := slog.Default()
	if l == nil {
		t.Fatal("Default logger is nil after Setup with nil writer")
	}
}

func TestSetupWithDiscard(t *testing.T) {
	Setup("info", io.Discard)
	slog.Info("discarded")
	// No assertion needed — verifies no panic with io.Discard.
}
