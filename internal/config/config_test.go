package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenHost != "localhost" {
		t.Fatalf("got host %q, want %q", cfg.ListenHost, "localhost")
	}
	if cfg.ListenPort != 7788 {
		t.Fatalf("got port %d, want %d", cfg.ListenPort, 7788)
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
	t.Setenv("CM_THEME", "")
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
	if cfg.ListenPort != 7788 {
		t.Fatalf("got port %d, want default 7788", cfg.ListenPort)
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
	if cfg.ListenPort != 7788 {
		t.Fatalf("got port %d, want default 7788", cfg.ListenPort)
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
	if cfg.ListenPort != 7788 {
		t.Errorf("port: got %d, want default 7788", cfg.ListenPort)
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
			if cfg.ListenPort != 7788 {
				t.Errorf("port %s: got %d, want default 7788", port, cfg.ListenPort)
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

func TestPluginConfig(t *testing.T) {
	cfg := DefaultConfig()

	// No plugins section — returns nil
	if got := cfg.PluginConfig("update"); got != nil {
		t.Fatalf("expected nil for missing plugin, got %v", got)
	}

	// Set a value
	cfg.SetPluginConfig("update", "schedule", "0 3 * * *")
	cfg.SetPluginConfig("update", "auto_security", true)

	pc := cfg.PluginConfig("update")
	if pc == nil {
		t.Fatal("expected non-nil plugin config after SetPluginConfig")
	}
	if pc["schedule"] != "0 3 * * *" {
		t.Errorf("schedule: got %v, want %q", pc["schedule"], "0 3 * * *")
	}
	if pc["auto_security"] != true {
		t.Errorf("auto_security: got %v, want true", pc["auto_security"])
	}

	// Non-existent plugin still returns nil
	if got := cfg.PluginConfig("network"); got != nil {
		t.Fatalf("expected nil for network, got %v", got)
	}
}

func TestLoadYAMLWithPluginSections(t *testing.T) {
	clearCMEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlData := `listen_host: "0.0.0.0"
listen_port: 7788
plugins:
  update:
    schedule: "0 5 * * *"
    auto_security: true
    security_source: "available"
  network:
    dns_override: "8.8.8.8"
`
	if err := os.WriteFile(path, []byte(yamlData), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	up := cfg.PluginConfig("update")
	if up == nil {
		t.Fatal("expected update plugin config")
	}
	if up["schedule"] != "0 5 * * *" {
		t.Errorf("schedule: got %v, want %q", up["schedule"], "0 5 * * *")
	}
	if up["auto_security"] != true {
		t.Errorf("auto_security: got %v, want true", up["auto_security"])
	}
	if up["security_source"] != "available" {
		t.Errorf("security_source: got %v, want %q", up["security_source"], "available")
	}

	net := cfg.PluginConfig("network")
	if net == nil {
		t.Fatal("expected network plugin config")
	}
	if net["dns_override"] != "8.8.8.8" {
		t.Errorf("dns_override: got %v, want %q", net["dns_override"], "8.8.8.8")
	}
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")

	cfg := DefaultConfig()
	cfg.ListenHost = "0.0.0.0"
	cfg.EnabledPlugins = []string{"update", "network"}
	cfg.SetPluginConfig("update", "schedule", "0 3 * * *")
	cfg.SetPluginConfig("update", "auto_security", true)

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists and is readable
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("saved config is empty")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	clearCMEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.yaml")

	original := DefaultConfig()
	original.ListenHost = "0.0.0.0"
	original.ListenPort = 9090
	original.LogLevel = "debug"
	original.EnabledPlugins = []string{"update"}
	original.SetPluginConfig("update", "schedule", "0 5 * * *")
	original.SetPluginConfig("update", "auto_security", true)
	original.SetPluginConfig("update", "security_source", "available")

	if err := original.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.ListenHost != original.ListenHost {
		t.Errorf("host: got %q, want %q", loaded.ListenHost, original.ListenHost)
	}
	if loaded.ListenPort != original.ListenPort {
		t.Errorf("port: got %d, want %d", loaded.ListenPort, original.ListenPort)
	}
	if loaded.LogLevel != original.LogLevel {
		t.Errorf("log_level: got %q, want %q", loaded.LogLevel, original.LogLevel)
	}
	if len(loaded.EnabledPlugins) != 1 || loaded.EnabledPlugins[0] != "update" {
		t.Errorf("enabled_plugins: got %v, want [update]", loaded.EnabledPlugins)
	}

	up := loaded.PluginConfig("update")
	if up == nil {
		t.Fatal("expected update plugin config after round-trip")
	}
	if up["schedule"] != "0 5 * * *" {
		t.Errorf("schedule: got %v, want %q", up["schedule"], "0 5 * * *")
	}
	if up["auto_security"] != true {
		t.Errorf("auto_security: got %v, want true", up["auto_security"])
	}
	if up["security_source"] != "available" {
		t.Errorf("security_source: got %v, want %q", up["security_source"], "available")
	}
}

func TestSaveEmptyPath(t *testing.T) {
	// Empty path falls back to defaultConfigPath (/etc/cm/config.yaml).
	// Skip if that path is writable (e.g., root in CI container).
	if _, err := os.Stat("/etc/cm"); err == nil {
		t.Skip("default config directory exists; skipping to avoid side effects")
	}
	cfg := DefaultConfig()
	err := cfg.Save("")
	if err == nil {
		t.Fatal("expected error writing to default path in test env")
	}
}

func TestSaveBadPath(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Save("/nonexistent/dir/config.yaml")
	if err == nil {
		t.Fatal("expected error for bad path")
	}
}

func TestSetPluginConfigOverwrite(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetPluginConfig("update", "schedule", "0 3 * * *")
	cfg.SetPluginConfig("update", "schedule", "0 5 * * *")

	pc := cfg.PluginConfig("update")
	if pc["schedule"] != "0 5 * * *" {
		t.Errorf("schedule: got %v, want %q (overwritten value)", pc["schedule"], "0 5 * * *")
	}
}

func TestSaveNilPlugins(t *testing.T) {
	clearCMEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "noplugins.yaml")

	cfg := DefaultConfig()
	// Plugins is nil — omitempty should omit it from YAML
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if strings.Contains(string(data), "\nplugins:") {
		t.Errorf("YAML should not contain 'plugins:' section when nil, got:\n%s", data)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Plugins != nil {
		t.Errorf("expected nil Plugins after round-trip, got %v", loaded.Plugins)
	}
}

func TestSaveFilePermissions(t *testing.T) {
	if os.Getenv("CI") != "" || runtime.GOOS == "windows" {
		t.Skip("file permission test only reliable on local Linux/macOS")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file permissions: got %o, want 0600", perm)
	}
}

func TestSaveIntRoundTrip(t *testing.T) {
	clearCMEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "types.yaml")

	cfg := DefaultConfig()
	cfg.SetPluginConfig("update", "retries", 5)
	cfg.SetPluginConfig("update", "enabled", true)

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	up := loaded.PluginConfig("update")
	// yaml.v3 preserves int (not float64) for integer values
	if v, ok := up["retries"].(int); !ok || v != 5 {
		t.Errorf("retries: got %v (%T), want 5 (int)", up["retries"], up["retries"])
	}
	if v, ok := up["enabled"].(bool); !ok || v != true {
		t.Errorf("enabled: got %v (%T), want true (bool)", up["enabled"], up["enabled"])
	}
}

func TestPathDefault(t *testing.T) {
	cfg := DefaultConfig()
	got := cfg.Path()
	if got != defaultConfigPath {
		t.Errorf("Path() = %q, want default %q", got, defaultConfigPath)
	}
}

func TestPathFromLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-config.yaml")
	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Path() != path {
		t.Errorf("Path() = %q, want %q", loaded.Path(), path)
	}
}

func TestDefaultConfig_ThemeEmpty(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Theme != "" {
		t.Errorf("Theme: got %q, want empty", cfg.Theme)
	}
}

func TestLoadYAML_WithTheme(t *testing.T) {
	clearCMEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlData := `listen_port: 7788
theme: "solarized"
`
	if err := os.WriteFile(path, []byte(yamlData), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "solarized" {
		t.Errorf("Theme: got %q, want %q", cfg.Theme, "solarized")
	}
}

func TestApplyEnv_ThemeOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlData := `theme: "solarized"
`
	if err := os.WriteFile(path, []byte(yamlData), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv("CM_THEME", "dracula")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "dracula" {
		t.Errorf("Theme: got %q, want %q (from env)", cfg.Theme, "dracula")
	}
}

func TestSaveLoadRoundTrip_Theme(t *testing.T) {
	clearCMEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "theme-roundtrip.yaml")

	original := DefaultConfig()
	original.Theme = "high-contrast"

	if err := original.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Theme != "high-contrast" {
		t.Errorf("Theme: got %q, want %q", loaded.Theme, "high-contrast")
	}
}

func TestSave_EmptyThemeOmitted(t *testing.T) {
	clearCMEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "notheme.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), "theme:") {
		t.Errorf("YAML should not contain 'theme:' when empty, got:\n%s", data)
	}
}
