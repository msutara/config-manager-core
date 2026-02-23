package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenHost != "localhost" {
		t.Fatalf("got host %q, want %q", cfg.ListenHost, "localhost")
	}
	if cfg.ListenPort != 8080 {
		t.Fatalf("got port %d, want %d", cfg.ListenPort, 8080)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("got log_level %q, want %q", cfg.LogLevel, "info")
	}
	if len(cfg.EnabledPlugins) != 0 {
		t.Fatalf("got %d enabled_plugins, want 0", len(cfg.EnabledPlugins))
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return defaults
	if cfg.ListenPort != 8080 {
		t.Fatalf("got port %d, want default 8080", cfg.ListenPort)
	}
}

func TestLoadValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `listen_host: "0.0.0.0"
listen_port: 9090
log_level: "debug"
enabled_plugins:
  - update
  - network
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenHost != "0.0.0.0" {
		t.Fatalf("got host %q, want %q", cfg.ListenHost, "0.0.0.0")
	}
	if cfg.ListenPort != 9090 {
		t.Fatalf("got port %d, want %d", cfg.ListenPort, 9090)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("got log_level %q, want %q", cfg.LogLevel, "debug")
	}
	if len(cfg.EnabledPlugins) != 2 {
		t.Fatalf("got %d enabled_plugins, want 2", len(cfg.EnabledPlugins))
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")

	if err := os.WriteFile(path, []byte("{{invalid: yaml: [}"), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadEmptyPath(t *testing.T) {
	// Empty path falls back to defaultConfigPath (/etc/cm/config.yaml).
	// On most test machines this doesn't exist, so we get defaults.
	// Skip if it happens to exist to keep the test hermetic.
	if _, err := os.Stat("/etc/cm/config.yaml"); err == nil {
		t.Skip("default config path exists; skipping hermetic test")
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenPort != 8080 {
		t.Fatalf("got port %d, want default 8080", cfg.ListenPort)
	}
}
