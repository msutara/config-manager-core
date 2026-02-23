package plugin

import (
	"net/http"
	"testing"
)

// fakePlugin implements Plugin for testing.
type fakePlugin struct {
	name    string
	version string
	desc    string
	routes  http.Handler
	jobs    []JobDefinition
}

func (f *fakePlugin) Name() string                   { return f.name }
func (f *fakePlugin) Version() string                { return f.version }
func (f *fakePlugin) Description() string            { return f.desc }
func (f *fakePlugin) Routes() http.Handler           { return f.routes }
func (f *fakePlugin) ScheduledJobs() []JobDefinition { return f.jobs }

func newFake(name string) *fakePlugin {
	return &fakePlugin{name: name, version: "1.0.0", desc: name + " plugin"}
}

func TestRegister(t *testing.T) {
	ResetForTesting()

	p := newFake("test")
	Register(p)

	got, ok := Get("test")
	if !ok {
		t.Fatal("expected plugin to be registered")
	}
	if got.Name() != "test" {
		t.Fatalf("got name %q, want %q", got.Name(), "test")
	}
}

func TestRegisterNil(t *testing.T) {
	ResetForTesting()

	Register(nil)

	if len(List()) != 0 {
		t.Fatal("nil plugin should not be registered")
	}
}

func TestRegisterEmptyName(t *testing.T) {
	ResetForTesting()

	Register(&fakePlugin{name: ""})

	if len(List()) != 0 {
		t.Fatal("empty-name plugin should not be registered")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	ResetForTesting()

	Register(newFake("dup"))
	Register(newFake("dup"))

	if len(List()) != 1 {
		t.Fatal("duplicate should be skipped")
	}
}

func TestGetNotFound(t *testing.T) {
	ResetForTesting()

	_, ok := Get("nonexistent")
	if ok {
		t.Fatal("expected false for missing plugin")
	}
}

func TestList(t *testing.T) {
	ResetForTesting()

	Register(newFake("a"))
	Register(newFake("b"))

	plugins := List()
	if len(plugins) != 2 {
		t.Fatalf("got %d plugins, want 2", len(plugins))
	}
}

func TestAllRoutes(t *testing.T) {
	ResetForTesting()

	mux := http.NewServeMux()
	Register(&fakePlugin{name: "with-routes", version: "1.0.0", routes: mux})
	Register(&fakePlugin{name: "no-routes", version: "1.0.0", routes: nil})

	routes := AllRoutes()
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}
	if _, ok := routes["with-routes"]; !ok {
		t.Fatal("expected routes for 'with-routes'")
	}
}

func TestAllJobs(t *testing.T) {
	ResetForTesting()

	jobs := []JobDefinition{
		{ID: "p.job1", Description: "job one"},
		{ID: "p.job2", Description: "job two"},
	}
	Register(&fakePlugin{name: "jobby", version: "1.0.0", jobs: jobs})

	got := AllJobs()
	if len(got) != 2 {
		t.Fatalf("got %d jobs, want 2", len(got))
	}
}

func TestMetadataFrom(t *testing.T) {
	p := newFake("meta")
	m := MetadataFrom(p)
	if m.Name != "meta" || m.Version != "1.0.0" || m.Description != "meta plugin" {
		t.Fatalf("unexpected metadata: %+v", m)
	}
}
