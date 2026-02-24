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

// clearCMEnv ensures no CM_* environment variables leak into tests that
// call Load() and assert on default/YAML values.
func clearCMEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CM_LISTEN_HOST", "")
	t.Setenv("CM_LISTEN_PORT", "")
	t.Setenv("CM_LOG_LEVEL", "")
	t.Setenv("CM_ENABLED_PLUGINS", "")
}

func TestLoadMissingFile(t *testing.T) {
	clearCMEnv(t)
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
	clearCMEnv(t)
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
	clearCMEnv(t)
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

func TestApplyEnv_OverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")

	t.Setenv("CM_LISTEN_HOST", "0.0.0.0")
	t.Setenv("CM_LISTEN_PORT", "9090")
	t.Setenv("CM_LOG_LEVEL", "debug")
	t.Setenv("CM_ENABLED_PLUGINS", "update,network")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenHost != "0.0.0.0" {
		t.Errorf("host: got %q, want %q", cfg.ListenHost, "0.0.0.0")
	}
	if cfg.ListenPort != 9090 {
		t.Errorf("port: got %d, want %d", cfg.ListenPort, 9090)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log_level: got %q, want %q", cfg.LogLevel, "debug")
	}
	if len(cfg.EnabledPlugins) != 2 || cfg.EnabledPlugins[0] != "update" || cfg.EnabledPlugins[1] != "network" {
		t.Errorf("enabled_plugins: got %v, want [update network]", cfg.EnabledPlugins)
	}
}

func TestApplyEnv_OverridesYAML(t *testing.T) {
	clearCMEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlData := `listen_host: "127.0.0.1"
listen_port: 3000
log_level: "warn"
`
	if err := os.WriteFile(path, []byte(yamlData), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Setenv("CM_LISTEN_PORT", "4000")
	t.Setenv("CM_LOG_LEVEL", "error")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// YAML value kept when env not set
	if cfg.ListenHost != "127.0.0.1" {
		t.Errorf("host: got %q, want %q (from YAML)", cfg.ListenHost, "127.0.0.1")
	}
	// Env overrides YAML
	if cfg.ListenPort != 4000 {
		t.Errorf("port: got %d, want %d (from env)", cfg.ListenPort, 4000)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("log_level: got %q, want %q (from env)", cfg.LogLevel, "error")
	}
}

func TestApplyEnv_InvalidPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")

	t.Setenv("CM_LISTEN_PORT", "not-a-number")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should keep default when port is invalid
	if cfg.ListenPort != 8080 {
		t.Errorf("port: got %d, want default 8080", cfg.ListenPort)
	}
}

func TestApplyEnv_OutOfRangePort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")

	for _, port := range []string{"0", "-1", "65536", "99999"} {
		t.Run(port, func(t *testing.T) {
			t.Setenv("CM_LISTEN_PORT", port)
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.ListenPort != 8080 {
				t.Errorf("port %s: got %d, want default 8080", port, cfg.ListenPort)
			}
		})
	}
}

func TestApplyEnv_PluginsWithWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")

	t.Setenv("CM_ENABLED_PLUGINS", " update , , network ")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.EnabledPlugins) != 2 || cfg.EnabledPlugins[0] != "update" || cfg.EnabledPlugins[1] != "network" {
		t.Errorf("enabled_plugins: got %v, want [update network]", cfg.EnabledPlugins)
	}
}

func TestApplyEnv_PluginsWhitespaceOnlyPreservesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlData := `enabled_plugins:
  - update
`
	if err := os.WriteFile(path, []byte(yamlData), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Setenv("CM_ENABLED_PLUGINS", " , , ")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Whitespace-only should be ignored, preserving YAML value
	if len(cfg.EnabledPlugins) != 1 || cfg.EnabledPlugins[0] != "update" {
		t.Errorf("enabled_plugins: got %v, want [update]", cfg.EnabledPlugins)
	}
}
