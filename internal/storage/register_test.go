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
	called := 0
	factory := func(dataDir string, maxRuns int) (JobStore, error) {
		called++
		return nil, nil
	}

	Register("test-dup", factory)
	Register("test-dup", factory) // should overwrite

	f, ok := backends["test-dup"]
	if !ok {
		t.Fatal("expected test-dup to be registered")
	}
	// Call the factory to verify it's the latest one.
	_, _ = f(t.TempDir(), 10)
	if called != 1 {
		t.Errorf("factory called %d times, expected 1", called)
	}

	// Clean up to avoid polluting other tests.
	delete(backends, "test-dup")
}

func TestAvailableBackends(t *testing.T) {
	// The json backend is registered via init() in json.go.
	if _, ok := backends["json"]; !ok {
		t.Fatal("expected 'json' backend to be registered after init()")
	}
}

func TestAvailableBackends_Empty(t *testing.T) {
	saved := backends
	backends = map[string]Factory{}
	defer func() { backends = saved }()

	got := availableBackends()
	if got != "none" {
		t.Errorf("availableBackends() = %q, want %q", got, "none")
	}
}

func TestAvailableBackends_Multiple(t *testing.T) {
	saved := backends
	backends = map[string]Factory{}
	defer func() { backends = saved }()

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
