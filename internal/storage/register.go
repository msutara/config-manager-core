package storage

import (
	"fmt"
	"sort"
)

// Factory creates a JobStore of a specific backend type.
type Factory func(dataDir string, maxRuns int) (JobStore, error)

var backends = map[string]Factory{}

// Register adds a storage backend factory. Called from init() in each
// backend file (json.go, and optionally sqlite.go when built with
// -tags sqlite).
func Register(name string, f Factory) {
	backends[name] = f
}

// New creates a JobStore using the named backend. Returns an error if the
// backend is not registered (e.g., requesting "sqlite" on a slim build).
func New(name, dataDir string, maxRuns int) (JobStore, error) {
	f, ok := backends[name]
	if !ok {
		return nil, fmt.Errorf("unknown storage backend: %q (available: %s)", name, availableBackends())
	}
	return f(dataDir, maxRuns)
}

func availableBackends() string {
	if len(backends) == 0 {
		return "none"
	}
	names := make([]string, 0, len(backends))
	for k := range backends {
		names = append(names, k)
	}
	sort.Strings(names)
	return fmt.Sprintf("%v", names)
}
