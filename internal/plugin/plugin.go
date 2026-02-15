// Package plugin defines the Plugin interface that all CM plugins must
// implement and provides a global registry for plugin registration.
package plugin

import "net/http"

// Plugin is the interface that all Config Manager plugins must implement.
// Plugins register themselves via init() by calling Register().
type Plugin interface {
	// Name returns the unique plugin identifier (e.g., "update", "network").
	Name() string

	// Version returns the plugin version string (semver recommended).
	Version() string

	// Description returns a human-readable description of the plugin.
	Description() string

	// Routes returns an http.Handler to be mounted under
	// /api/v1/plugins/{Name()}. Return nil if the plugin has no HTTP routes.
	Routes() http.Handler

	// ScheduledJobs returns job definitions for the scheduler.
	// Return nil or an empty slice if no scheduled jobs are needed.
	ScheduledJobs() []JobDefinition
}

// JobDefinition describes a scheduled job provided by a plugin.
type JobDefinition struct {
	// ID is globally unique, conventionally "{plugin_name}.{job_name}".
	ID          string
	Description string
	Cron        string       // cron expression, e.g. "0 3 * * *"
	Func        func() error // the function to execute
}

// Metadata holds plugin metadata returned by API endpoints.
type Metadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// MetadataFrom extracts Metadata from a Plugin.
func MetadataFrom(p Plugin) Metadata {
	return Metadata{
		Name:        p.Name(),
		Version:     p.Version(),
		Description: p.Description(),
	}
}
