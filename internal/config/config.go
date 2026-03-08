package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "/etc/cm/config.yaml"

// Config holds the global configuration for CM Core.
type Config struct {
	ListenHost        string                    `yaml:"listen_host"`
	ListenPort        int                       `yaml:"listen_port"`
	EnabledPlugins    []string                  `yaml:"enabled_plugins"` // empty = all enabled
	LogLevel          string                    `yaml:"log_level"`
	Theme             string                    `yaml:"theme,omitempty"` // built-in name or file path
	DataDir           string                    `yaml:"data_dir,omitempty"`
	StorageBackend    string                    `yaml:"storage_backend,omitempty"`
	JobHistoryMaxRuns int                       `yaml:"job_history_max_runs,omitempty"`
	Plugins           map[string]map[string]any `yaml:"plugins,omitempty"`

	path string `yaml:"-"` // file path used by Load, not serialized
}

// Path returns the file path from which this config was loaded.
// Returns the default path if Load was not called.
func (c *Config) Path() string {
	if c.path == "" {
		return defaultConfigPath
	}
	return c.path
}

// PluginConfig returns the config map for a specific plugin.
// Returns nil if no config exists for that plugin.
// NOTE: The returned map is a live reference to internal state.
// Mutations are equivalent to calling SetPluginConfig.
func (c *Config) PluginConfig(name string) map[string]any {
	if c.Plugins == nil {
		return nil
	}
	return c.Plugins[name]
}

// SetPluginConfig sets a single key in a plugin's config section.
// Creates the plugin section if it doesn't exist.
// NOTE: Config is not goroutine-safe. Callers must serialize access
// when concurrent reads/writes are possible (e.g., from an API handler).
func (c *Config) SetPluginConfig(plugin, key string, value any) {
	if c.Plugins == nil {
		c.Plugins = make(map[string]map[string]any)
	}
	if c.Plugins[plugin] == nil {
		c.Plugins[plugin] = make(map[string]any)
	}
	c.Plugins[plugin][key] = value
}

// Save writes the config to a YAML file at the given path.
// Uses atomic write (temp file + rename) to prevent corruption on crash.
// TODO: Add path validation when Save is wired to an API endpoint.
func (c *Config) Save(path string) error {
	if path == "" {
		path = defaultConfigPath
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".cm-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp config: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ListenHost:        "localhost",
		ListenPort:        7788,
		LogLevel:          "info",
		DataDir:           "/var/lib/cm",
		StorageBackend:    "json",
		JobHistoryMaxRuns: 50,
	}
}

// Load reads configuration from a YAML file. If the file does not exist,
// it returns the default configuration.
func Load(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath
	}

	cfg := DefaultConfig()
	cfg.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("config file not found, using defaults", "path", path)
			applyEnv(cfg)
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	applyEnv(cfg)
	return cfg, nil
}

// applyEnv overrides config fields with environment variables when set.
// Supported variables: CM_LISTEN_HOST, CM_LISTEN_PORT, CM_LOG_LEVEL,
// CM_ENABLED_PLUGINS (comma-separated), CM_THEME.
func applyEnv(cfg *Config) {
	if v := os.Getenv("CM_LISTEN_HOST"); v != "" {
		cfg.ListenHost = v
	}
	if v := os.Getenv("CM_LISTEN_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			if port < 1 || port > 65535 {
				slog.Warn("ignoring out-of-range CM_LISTEN_PORT", "value", v)
			} else {
				cfg.ListenPort = port
			}
		} else {
			slog.Warn("ignoring invalid CM_LISTEN_PORT", "value", v)
		}
	}
	if v := os.Getenv("CM_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("CM_ENABLED_PLUGINS"); v != "" {
		var plugins []string
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				plugins = append(plugins, p)
			}
		}
		if len(plugins) > 0 {
			cfg.EnabledPlugins = plugins
		} else {
			slog.Warn("ignoring CM_ENABLED_PLUGINS with no valid entries", "value", v)
		}
	}
	if v := os.Getenv("CM_THEME"); v != "" {
		cfg.Theme = v
	}
	if v := os.Getenv("CM_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("CM_STORAGE_BACKEND"); v != "" {
		cfg.StorageBackend = v
	}
	if v := os.Getenv("CM_JOB_HISTORY_MAX_RUNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.JobHistoryMaxRuns = n
		} else {
			slog.Warn("ignoring invalid CM_JOB_HISTORY_MAX_RUNS", "value", v)
		}
	}
}
