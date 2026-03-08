package storage

import (
	"strings"
	"testing"
)

func TestNew_ValidBackend(t *testing.T) {
	dir := t.TempDir()
	store, err := New("json", dir, 50)
	if err != nil {
		t.Fatalf("New(json): %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNew_UnknownBackend(t *testing.T) {
	_, err := New("unknown", t.TempDir(), 50)
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestRegister_Duplicate(t *testing.T) {
	var called1, called2 int
	factory1 := func(dataDir string, maxRuns int) (JobStore, error) {
		called1++
		return nil, nil
	}
	factory2 := func(dataDir string, maxRuns int) (JobStore, error) {
		called2++
		return nil, nil
	}

	Register("test-dup", factory1)
	Register("test-dup", factory2) // should overwrite factory1

	mu.RLock()
	f, ok := backends["test-dup"]
	mu.RUnlock()
	if !ok {
		t.Fatal("expected test-dup to be registered")
	}
	// Call the factory to verify it's the latest one (factory2).
	_, _ = f(t.TempDir(), 10)
	if called1 != 0 {
		t.Errorf("factory1 called %d times, expected 0", called1)
	}
	if called2 != 1 {
		t.Errorf("factory2 called %d times, expected 1", called2)
	}

	// Clean up to avoid polluting other tests.
	mu.Lock()
	delete(backends, "test-dup")
	mu.Unlock()
}

func TestAvailableBackends(t *testing.T) {
	// The json backend is registered via init() in json.go.
	mu.RLock()
	_, ok := backends["json"]
	mu.RUnlock()
	if !ok {
		t.Fatal("expected 'json' backend to be registered after init()")
	}
}

func TestAvailableBackends_Empty(t *testing.T) {
	mu.Lock()
	saved := backends
	backends = map[string]Factory{}
	mu.Unlock()
	defer func() {
		mu.Lock()
		backends = saved
		mu.Unlock()
	}()

	got := availableBackends()
	if got != "none" {
		t.Errorf("availableBackends() = %q, want %q", got, "none")
	}
}

func TestAvailableBackends_Multiple(t *testing.T) {
	mu.Lock()
	saved := backends
	backends = map[string]Factory{}
	mu.Unlock()
	defer func() {
		mu.Lock()
		backends = saved
		mu.Unlock()
	}()

	noop := func(string, int) (JobStore, error) { return nil, nil }
	Register("alpha", noop)
	Register("beta", noop)

	got := availableBackends()
	if !strings.Contains(got, "alpha") {
		t.Errorf("availableBackends() = %q, missing 'alpha'", got)
	}
	if !strings.Contains(got, "beta") {
		t.Errorf("availableBackends() = %q, missing 'beta'", got)
	}
}

func TestNew_UnknownBackendErrorMessage(t *testing.T) {
	_, err := New("nosuchbackend", t.TempDir(), 50)
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown storage backend") {
		t.Errorf("error should contain 'unknown storage backend', got: %q", msg)
	}
	if !strings.Contains(msg, "available:") {
		t.Errorf("error should contain 'available:', got: %q", msg)
	}
}

func TestNew_FactoryReturnsNil(t *testing.T) {
	Register("nil-store", func(string, int) (JobStore, error) {
		return nil, nil
	})
	defer func() {
		mu.Lock()
		delete(backends, "nil-store")
		mu.Unlock()
	}()

	_, err := New("nil-store", t.TempDir(), 10)
	if err == nil {
		t.Fatal("expected error when factory returns nil store")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil, got: %q", err.Error())
	}
}

func TestRegister_PanicEmptyName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty name")
		}
	}()
	Register("", func(string, int) (JobStore, error) { return nil, nil })
}

func TestRegister_PanicNilFactory(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil factory")
		}
	}()
	Register("test-nil", nil)
}
