package config

import (
	"log/slog"
	"os"

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
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
