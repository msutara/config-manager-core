package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "/etc/cm/config.yaml"

// Config holds the global configuration for CM Core.
type Config struct {
	ListenHost     string   `yaml:"listen_host"`
	ListenPort     int      `yaml:"listen_port"`
	EnabledPlugins []string `yaml:"enabled_plugins"` // empty = all enabled
	LogLevel       string   `yaml:"log_level"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ListenHost: "localhost",
		ListenPort: 8080,
		LogLevel:   "info",
	}
}

// Load reads configuration from a YAML file. If the file does not exist,
// it returns the default configuration.
func Load(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath
	}

	cfg := DefaultConfig()

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
// CM_ENABLED_PLUGINS (comma-separated).
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
}
