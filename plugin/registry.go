package plugin

import (
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

var (
	mu       sync.RWMutex
	registry = make(map[string]Plugin)
)

// Register adds a plugin to the global registry. It is called from the core
// binary's main.go during startup (explicit registration). If a plugin with
// the same name is already registered, the duplicate is logged and skipped.
func Register(p Plugin) {
	if p == nil {
		slog.Warn("plugin registration skipped: nil plugin")
		return
	}

	mu.Lock()
	defer mu.Unlock()

	name := p.Name()
	if name == "" {
		slog.Warn("plugin registration skipped: empty name")
		return
	}
	if !validName.MatchString(name) {
		slog.Warn("plugin registration skipped: invalid name (must match [a-z][a-z0-9-]*)", "plugin", name)
		return
	}
	if _, exists := registry[name]; exists {
		slog.Warn("duplicate plugin registration skipped", "plugin", name)
		return
	}
	registry[name] = p
	slog.Info("plugin registered", "plugin", name, "version", p.Version())
}

// Get returns a plugin by name and a boolean indicating whether it was found.
func Get(name string) (Plugin, bool) {
	mu.RLock()
	defer mu.RUnlock()

	p, ok := registry[name]
	return p, ok
}

// List returns all registered plugins in deterministic (name-sorted) order.
func List() []Plugin {
	mu.RLock()
	type entry struct {
		name   string
		plugin Plugin
	}
	entries := make([]entry, 0, len(registry))
	for name, p := range registry {
		entries = append(entries, entry{name, p})
	}
	mu.RUnlock()

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	plugins := make([]Plugin, len(entries))
	for i, e := range entries {
		plugins[i] = e.plugin
	}
	return plugins
}

// AllRoutes returns a map of plugin name to http.Handler for all plugins
// that provide routes. Handlers are computed once to avoid double evaluation.
func AllRoutes() map[string]http.Handler {
	mu.RLock()
	plugins := make([]struct {
		name   string
		plugin Plugin
	}, 0, len(registry))
	for name, p := range registry {
		plugins = append(plugins, struct {
			name   string
			plugin Plugin
		}{name, p})
	}
	mu.RUnlock()

	routes := make(map[string]http.Handler)
	for _, entry := range plugins {
		if handler := entry.plugin.Routes(); handler != nil {
			routes[entry.name] = handler
		}
	}
	return routes
}

// ResetForTesting clears the global registry. It is exported so that tests in
// other packages (e.g., internal/api) can reset state between test cases.
func ResetForTesting() {
	mu.Lock()
	defer mu.Unlock()
	registry = make(map[string]Plugin)
}

// DisableExcept removes all plugins from the registry whose names are not in
// the provided allowlist. If allowlist is empty (or contains only blank
// entries), all plugins remain active.
func DisableExcept(allowlist []string) {
	allowed := make(map[string]bool, len(allowlist))
	for _, n := range allowlist {
		n = strings.TrimSpace(n)
		if n != "" {
			allowed[n] = true
		}
	}
	if len(allowed) == 0 {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	for name := range registry {
		if !allowed[name] {
			slog.Info("plugin disabled by config", "plugin", name)
			delete(registry, name)
		}
	}
}

// AllJobs returns all job definitions from all registered plugins.
// Jobs are sorted by ID for deterministic ordering.
// Plugin code (ScheduledJobs) is called outside the lock to avoid blocking
// other registry operations.
func AllJobs() []JobDefinition {
	mu.RLock()
	plugins := make([]Plugin, 0, len(registry))
	for _, p := range registry {
		plugins = append(plugins, p)
	}
	mu.RUnlock()

	var jobs []JobDefinition
	for _, p := range plugins {
		jobs = append(jobs, p.ScheduledJobs()...)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].ID < jobs[j].ID
	})
	return jobs
}
