// Package plugin defines the Plugin interface that all CM plugins must
// implement and provides a global registry for plugin registration.
package plugin

import "net/http"

// RouteBase is the common URL prefix under which all plugin routes are mounted.
const RouteBase = "/api/v1/plugins/"

// Endpoint describes a single HTTP endpoint exposed by a plugin.
// Plugins declare their endpoints so UIs can render generic pages
// for plugins that lack a custom template or TUI handler.
type Endpoint struct {
	Method      string `json:"method"`      // "GET" or "POST"
	Path        string `json:"path"`        // e.g. "/status", "/run"
	Description string `json:"description"` // human-readable label
}

// Plugin is the interface that all Config Manager plugins must implement.
// Plugins are registered explicitly in the core binary's main.go via
// plugin.Register().
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

	// Endpoints returns the list of HTTP endpoints this plugin exposes.
	// UIs use this to build generic pages and menus for plugins that
	// lack a custom template. Return nil or empty if not applicable.
	Endpoints() []Endpoint
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
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Description string     `json:"description"`
	RoutePrefix string     `json:"route_prefix"`
	Endpoints   []Endpoint `json:"endpoints"`
}

// MetadataFrom extracts Metadata from a Plugin.
func MetadataFrom(p Plugin) Metadata {
	eps := p.Endpoints()
	if eps == nil {
		eps = []Endpoint{}
	}
	return Metadata{
		Name:        p.Name(),
		Version:     p.Version(),
		Description: p.Description(),
		RoutePrefix: RouteBase + p.Name(),
		Endpoints:   eps,
	}
}
