package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/msutara/config-manager-core/plugin"
)

// mockScheduler implements JobTriggerer for testing.
type mockScheduler struct {
	triggerFunc func(id string) error
	existsFunc  func(id string) bool
}

func (m *mockScheduler) TriggerJob(id string) error {
	if m.triggerFunc != nil {
		return m.triggerFunc(id)
	}
	return nil
}

func (m *mockScheduler) JobExists(id string) bool {
	if m.existsFunc != nil {
		return m.existsFunc(id)
	}
	return true
}

func TestHandleHealth(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	handleHealth(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("got status %q, want %q", body["status"], "ok")
	}
}

func TestHandleNode(t *testing.T) {
	plugin.ResetForTesting()
	srv := NewServer("localhost", 0, nil, "", nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	srv.handleNode(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := body["hostname"]; !ok {
		t.Fatal("response missing 'hostname' field")
	}
	if _, ok := body["arch"]; !ok {
		t.Fatal("response missing 'arch' field")
	}
}

func TestHandleListPlugins(t *testing.T) {
	plugin.ResetForTesting()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	handleListPlugins(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusOK)
	}

	var body []plugin.Metadata
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("got %d plugins, want 0", len(body))
	}
}

func TestHandleGetPluginNotFound(t *testing.T) {
	plugin.ResetForTesting()

	srv := NewServer("localhost", 0, nil, "", nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/missing", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleListJobs(t *testing.T) {
	plugin.ResetForTesting()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	handleListJobs(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleTriggerJobInvalidJSON(t *testing.T) {
	srv := &Server{scheduler: &mockScheduler{}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/trigger",
		bytes.NewBufferString("not json"))
	srv.handleTriggerJob(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTriggerJobNoScheduler(t *testing.T) {
	srv := &Server{scheduler: nil}
	body := `{"job_id": "test.job"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/trigger",
		bytes.NewBufferString(body))
	srv.handleTriggerJob(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleTriggerJobAccepted(t *testing.T) {
	sched := &mockScheduler{triggerFunc: func(_ string) error { return nil }}
	srv := &Server{scheduler: sched}
	body := `{"job_id": "test.job"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/trigger",
		bytes.NewBufferString(body))
	srv.handleTriggerJob(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "test_code", "test message")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Error.Code != "test_code" {
		t.Fatalf("got code %q, want %q", resp.Error.Code, "test_code")
	}
}

func TestNewServerIntegration(t *testing.T) {
	plugin.ResetForTesting()

	sched := &mockScheduler{
		triggerFunc: func(_ string) error {
			return errors.New("not found")
		},
		existsFunc: func(_ string) bool {
			return true
		},
	}
	srv := NewServer("localhost", 0, sched, "", nil)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}

	// Test health endpoint through the full router
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestNewServerAuthIntegration(t *testing.T) {
	plugin.ResetForTesting()
	srv := NewServer("localhost", 0, nil, "integ-secret", nil)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}

	// Health should be accessible without token.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("health without token: got %d, want %d", w.Code, http.StatusOK)
	}

	// Node without token should be 401.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("node without token: got %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Node with valid token should be 200.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	r.Header.Set("Authorization", "Bearer integ-secret")
	srv.httpServer.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("node with token: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleTriggerJobNotFound(t *testing.T) {
	sched := &mockScheduler{
		existsFunc: func(_ string) bool {
			return false
		},
	}
	srv := &Server{scheduler: sched}
	body := `{"job_id": "no.such.job"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/trigger",
		bytes.NewBufferString(body))
	srv.handleTriggerJob(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerJobEmptyID(t *testing.T) {
	sched := &mockScheduler{}
	srv := &Server{scheduler: sched}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/trigger",
		bytes.NewBufferString(`{"job_id": ""}`))
	srv.handleTriggerJob(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWebHandlerMountRouting(t *testing.T) {
	plugin.ResetForTesting()

	// Stub web handler that returns 299 for any request.
	stub := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(299)
	})
	srv := NewServer("localhost", 0, nil, "", stub)

	// "/" should be routed to the web handler stub.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)
	if w.Code != 299 {
		t.Fatalf("GET /: got %d, want 299 (web handler)", w.Code)
	}

	// "/api/v1/health" should still be served by the API, not the stub.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/health: got %d, want %d", w.Code, http.StatusOK)
	}
}
