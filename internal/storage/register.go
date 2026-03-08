package storage

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Factory creates a JobStore of a specific backend type.
type Factory func(dataDir string, maxRuns int) (JobStore, error)

var (
	mu       sync.RWMutex
	backends = map[string]Factory{}
)

// Register adds a storage backend factory. Called from init() in each
// backend file (json.go, and optionally sqlite.go when built with
// -tags sqlite). Duplicate names silently overwrite the previous factory.
// Panics if name is empty or f is nil to catch misuse at init time.
func Register(name string, f Factory) {
	if strings.TrimSpace(name) == "" {
		panic("storage.Register: name must not be empty")
	}
	if f == nil {
		panic("storage.Register: factory must not be nil")
	}
	mu.Lock()
	defer mu.Unlock()
	backends[name] = f
}

// New creates a JobStore using the named backend. Returns an error if the
// backend is not registered (e.g., requesting "sqlite" on a slim build).
func New(name, dataDir string, maxRuns int) (JobStore, error) {
	mu.RLock()
	f, ok := backends[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown storage backend: %q (available: %s)", name, availableBackends())
	}
	store, err := f(dataDir, maxRuns)
	if err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("storage backend %q returned nil store without error", name)
	}
	return store, nil
}

func availableBackends() string {
	mu.RLock()
	defer mu.RUnlock()
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
