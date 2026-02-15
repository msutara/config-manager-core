package plugin

import (
	"fmt"
	"log/slog"
	"sync"
)

var (
	mu       sync.RWMutex
	registry = make(map[string]Plugin)
)

// Register adds a plugin to the global registry. It is intended to be called
// from a plugin's init() function. Panics if a plugin with the same name is
// already registered.
func Register(p Plugin) {
	mu.Lock()
	defer mu.Unlock()

	name := p.Name()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("plugin %q already registered", name))
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

// List returns all registered plugins.
func List() []Plugin {
	mu.RLock()
	defer mu.RUnlock()

	plugins := make([]Plugin, 0, len(registry))
	for _, p := range registry {
		plugins = append(plugins, p)
	}
	return plugins
}

// AllRoutes returns a map of plugin name to HTTP handler for all plugins
// that provide routes.
func AllRoutes() map[string]Plugin {
	mu.RLock()
	defer mu.RUnlock()

	routes := make(map[string]Plugin)
	for name, p := range registry {
		if p.Routes() != nil {
			routes[name] = p
		}
	}
	return routes
}

// AllJobs returns all job definitions from all registered plugins.
func AllJobs() []JobDefinition {
	mu.RLock()
	defer mu.RUnlock()

	var jobs []JobDefinition
	for _, p := range registry {
		jobs = append(jobs, p.ScheduledJobs()...)
	}
	return jobs
}
