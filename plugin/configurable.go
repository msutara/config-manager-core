package plugin

// Configurable is an optional interface that plugins may implement to
// support runtime configuration via the core API.
//
// Plugins that implement Configurable receive their persisted config
// at startup (via Configure) and can be updated at runtime (via
// UpdateConfig). The core handles persistence — plugins only manage
// in-memory state.
type Configurable interface {
	// Configure is called once at startup with the plugin's section
	// from the YAML config file. Plugins should apply defaults for
	// any missing keys. The map may be nil if no config exists yet.
	Configure(cfg map[string]any)

	// UpdateConfig validates and applies a single config key change.
	// Returns an error if the key is unknown or the value is invalid.
	UpdateConfig(key string, value any) error

	// CurrentConfig returns the plugin's current configuration as a
	// map suitable for JSON serialization and YAML persistence.
	CurrentConfig() map[string]any
}
