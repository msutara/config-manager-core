package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/msutara/config-manager-core/plugin"
)

// mockScheduler implements JobTriggerer for testing.
type mockScheduler struct {
	triggerFunc    func(id string) error
	existsFunc     func(id string) bool
	rescheduleFunc func(id, cron string) error
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

func (m *mockScheduler) Reschedule(id, cron string) error {
	if m.rescheduleFunc != nil {
		return m.rescheduleFunc(id, cron)
	}
	return nil
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

func TestSystemUptime_Fallback(t *testing.T) {
	// Force fallback by pointing at a guaranteed-nonexistent path.
	old := procUptimePath
	procUptimePath = t.TempDir() + "/nonexistent"
	defer func() { procUptimePath = old }()

	start := time.Now().Add(-5 * time.Minute)
	got := systemUptime(start)
	// Fallback should return ~300s (5 min of service uptime).
	if got < 295 || got > 305 {
		t.Fatalf("systemUptime fallback = %d, want ~300", got)
	}
}

func TestSystemUptime_ParsesFile(t *testing.T) {
	f := t.TempDir() + "/uptime"
	if err := os.WriteFile(f, []byte("86400.55 12345.67\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	old := procUptimePath
	procUptimePath = f
	defer func() { procUptimePath = old }()

	got := systemUptime(time.Now())
	if got != 86400 {
		t.Fatalf("systemUptime = %d, want 86400", got)
	}
}

func TestSystemUptime_NaN(t *testing.T) {
	f := t.TempDir() + "/uptime"
	if err := os.WriteFile(f, []byte("NaN 0.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	old := procUptimePath
	procUptimePath = f
	defer func() { procUptimePath = old }()

	start := time.Now().Add(-3 * time.Minute)
	got := systemUptime(start)
	if got < 175 || got > 185 {
		t.Fatalf("systemUptime NaN fallback = %d, want ~180", got)
	}
}

func TestSystemUptime_Negative(t *testing.T) {
	f := t.TempDir() + "/uptime"
	if err := os.WriteFile(f, []byte("-100.0 0.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	old := procUptimePath
	procUptimePath = f
	defer func() { procUptimePath = old }()

	start := time.Now().Add(-1 * time.Minute)
	got := systemUptime(start)
	if got < 55 || got > 65 {
		t.Fatalf("systemUptime negative fallback = %d, want ~60", got)
	}
}

func TestSystemUptime_Inf(t *testing.T) {
	f := t.TempDir() + "/uptime"
	if err := os.WriteFile(f, []byte("Inf 0.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	old := procUptimePath
	procUptimePath = f
	defer func() { procUptimePath = old }()

	start := time.Now().Add(-1 * time.Minute)
	got := systemUptime(start)
	if got < 55 || got > 65 {
		t.Fatalf("systemUptime Inf fallback = %d, want ~60", got)
	}
}

func TestSystemUptime_MalformedFile(t *testing.T) {
	f := t.TempDir() + "/uptime"
	if err := os.WriteFile(f, []byte("not-a-number idle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	old := procUptimePath
	procUptimePath = f
	defer func() { procUptimePath = old }()

	start := time.Now().Add(-2 * time.Minute)
	got := systemUptime(start)
	// Should fall back to service uptime (~120s).
	if got < 115 || got > 125 {
		t.Fatalf("systemUptime malformed fallback = %d, want ~120", got)
	}
}

func TestSystemUptime_EmptyFile(t *testing.T) {
	f := t.TempDir() + "/uptime"
	if err := os.WriteFile(f, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	old := procUptimePath
	procUptimePath = f
	defer func() { procUptimePath = old }()

	start := time.Now().Add(-1 * time.Minute)
	got := systemUptime(start)
	if got < 55 || got > 65 {
		t.Fatalf("systemUptime empty fallback = %d, want ~60", got)
	}
}

func TestHandleNode(t *testing.T) {
	plugin.ResetForTesting()
	srv := NewServer("localhost", 0, nil, nil, "", nil)
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

	srv := NewServer("localhost", 0, nil, nil, "", nil)
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
	srv := NewServer("localhost", 0, sched, nil, "", nil)
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
	srv := NewServer("localhost", 0, nil, nil, "integ-secret", nil)
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
	srv := NewServer("localhost", 0, nil, nil, "", stub)

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

// mockConfigPlugin implements both plugin.Plugin and plugin.Configurable.
type mockConfigPlugin struct {
	cfg map[string]any
}

func (m *mockConfigPlugin) Name() string                          { return "mockconfig" }
func (m *mockConfigPlugin) Version() string                       { return "0.1.0" }
func (m *mockConfigPlugin) Description() string                   { return "mock configurable plugin" }
func (m *mockConfigPlugin) Routes() http.Handler                  { return nil }
func (m *mockConfigPlugin) ScheduledJobs() []plugin.JobDefinition { return nil }
func (m *mockConfigPlugin) Endpoints() []plugin.Endpoint          { return nil }

func (m *mockConfigPlugin) Configure(cfg map[string]any) {
	m.cfg = make(map[string]any)
	for k, v := range cfg {
		m.cfg[k] = v
	}
}

func (m *mockConfigPlugin) UpdateConfig(key string, value any) error {
	if key == "bad_key" {
		return errors.New("unknown config key: bad_key")
	}
	m.cfg[key] = value
	return nil
}

func (m *mockConfigPlugin) CurrentConfig() map[string]any {
	return m.cfg
}

// routedConfigPlugin is like mockConfigPlugin but returns a Chi router from
// Routes(), simulating a real plugin (e.g. update) that mounts its own routes.
// Without the injected /settings routes, the plugin mount shadows the core's
// parameterized /plugins/{name}/settings route.
type routedConfigPlugin struct {
	mockConfigPlugin
}

func (r *routedConfigPlugin) Name() string { return "routed" }

func (r *routedConfigPlugin) Routes() http.Handler {
	rr := chi.NewRouter()
	rr.Get("/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return rr
}

// mockConfigProvider implements ConfigProvider for testing.
type mockConfigProvider struct {
	plugins map[string]map[string]any
	saved   bool
	saveErr error
}

func (m *mockConfigProvider) PluginConfig(name string) map[string]any {
	if m.plugins == nil {
		return nil
	}
	return m.plugins[name]
}

func (m *mockConfigProvider) SetPluginConfig(pluginName, key string, value any) {
	if m.plugins == nil {
		m.plugins = make(map[string]map[string]any)
	}
	if m.plugins[pluginName] == nil {
		m.plugins[pluginName] = make(map[string]any)
	}
	m.plugins[pluginName][key] = value
}

func (m *mockConfigProvider) Save(_ string) error {
	m.saved = true
	return m.saveErr
}

func (m *mockConfigProvider) Path() string { return "/tmp/test-config.yaml" }

func TestHandleGetPluginConfig(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(map[string]any{"schedule": "0 3 * * *"})
	plugin.Register(mp)

	srv := NewServer("localhost", 0, nil, nil, "", nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/mockconfig/settings", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	cfg, ok := body["config"].(map[string]any)
	if !ok {
		t.Fatal("response missing 'config' envelope")
	}
	if cfg["schedule"] != "0 3 * * *" {
		t.Errorf("schedule: got %v, want '0 3 * * *'", cfg["schedule"])
	}
}

func TestHandleGetPluginConfig_NotFound(t *testing.T) {
	plugin.ResetForTesting()
	srv := NewServer("localhost", 0, nil, nil, "", nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/nope/settings", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", w.Code)
	}
}

// simplePlugin implements plugin.Plugin but NOT plugin.Configurable.
type simplePlugin struct{ name string }

func (s *simplePlugin) Name() string                          { return s.name }
func (s *simplePlugin) Version() string                       { return "0.1.0" }
func (s *simplePlugin) Description() string                   { return "simple" }
func (s *simplePlugin) Routes() http.Handler                  { return nil }
func (s *simplePlugin) ScheduledJobs() []plugin.JobDefinition { return nil }
func (s *simplePlugin) Endpoints() []plugin.Endpoint          { return nil }

func TestHandleGetPluginConfig_NotConfigurable(t *testing.T) {
	plugin.ResetForTesting()
	plugin.Register(&simplePlugin{name: "nocfg"})

	srv := NewServer("localhost", 0, nil, nil, "", nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/nocfg/settings", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("got %d, want 501", w.Code)
	}
}

func TestHandleUpdatePluginConfig(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(map[string]any{"schedule": "0 3 * * *"})
	plugin.Register(mp)

	cfgProv := &mockConfigProvider{}
	sched := &mockScheduler{}

	srv := NewServer("localhost", 0, sched, cfgProv, "", nil)

	body := `{"key":"schedule","value":"0 4 * * *"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", w.Code, w.Body.String())
	}

	// Verify plugin received the update.
	if mp.cfg["schedule"] != "0 4 * * *" {
		t.Errorf("plugin config not updated: %v", mp.cfg)
	}

	// Verify config was persisted.
	if !cfgProv.saved {
		t.Error("config was not saved")
	}
	if cfgProv.plugins["mockconfig"]["schedule"] != "0 4 * * *" {
		t.Error("config provider not updated")
	}

	// Verify response envelope.
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["config"] == nil {
		t.Error("response missing 'config' key")
	}
}

func TestHandleUpdatePluginConfig_InvalidKey(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(nil)
	plugin.Register(mp)

	srv := NewServer("localhost", 0, nil, nil, "", nil)

	body := `{"key":"bad_key","value":"x"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
}

func TestHandleUpdatePluginConfig_EmptyKey(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(nil)
	plugin.Register(mp)

	srv := NewServer("localhost", 0, nil, nil, "", nil)

	body := `{"key":"","value":"x"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
}

func TestHandleUpdatePluginConfig_ScheduleReschedule(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(nil)
	plugin.Register(mp)

	var rescheduledID, rescheduledCron string
	sched := &mockScheduler{
		rescheduleFunc: func(id, cron string) error {
			rescheduledID = id
			rescheduledCron = cron
			return nil
		},
	}
	cfgProv := &mockConfigProvider{}

	srv := NewServer("localhost", 0, sched, cfgProv, "", nil)

	body := `{"key":"schedule","value":"0 5 * * *"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}

	if rescheduledID != "mockconfig.security" {
		t.Errorf("reschedule job_id: got %q, want 'mockconfig.security'", rescheduledID)
	}
	if rescheduledCron != "0 5 * * *" {
		t.Errorf("reschedule cron: got %q, want '0 5 * * *'", rescheduledCron)
	}
}

func TestHandleUpdatePluginConfig_NotFound(t *testing.T) {
	plugin.ResetForTesting()
	srv := NewServer("localhost", 0, &mockScheduler{}, &mockConfigProvider{}, "", nil)

	body := `{"key":"schedule","value":"0 4 * * *"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/nonexistent/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdatePluginConfig_NotConfigurable(t *testing.T) {
	plugin.ResetForTesting()
	plugin.Register(&simplePlugin{name: "simple"})
	srv := NewServer("localhost", 0, &mockScheduler{}, &mockConfigProvider{}, "", nil)

	body := `{"key":"schedule","value":"0 4 * * *"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/simple/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("got %d, want 501; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdatePluginConfig_MalformedJSON(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(nil)
	plugin.Register(mp)

	srv := NewServer("localhost", 0, &mockScheduler{}, &mockConfigProvider{}, "", nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(`{invalid json`))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdatePluginConfig_SaveFails(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(map[string]any{"schedule": "0 3 * * *"})
	plugin.Register(mp)

	cfgProv := &mockConfigProvider{saveErr: errors.New("disk full")}
	srv := NewServer("localhost", 0, &mockScheduler{}, cfgProv, "", nil)

	body := `{"key":"schedule","value":"0 4 * * *"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500; body: %s", w.Code, w.Body.String())
	}

	// Verify the error message does NOT leak internal details.
	if bytes.Contains(w.Body.Bytes(), []byte("disk full")) {
		t.Error("error message leaked internal details")
	}
}

func TestHandleUpdatePluginConfig_NonScheduleKey(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(nil)
	plugin.Register(mp)

	var rescheduleCalled bool
	sched := &mockScheduler{
		rescheduleFunc: func(_, _ string) error {
			rescheduleCalled = true
			return nil
		},
	}
	cfgProv := &mockConfigProvider{}
	srv := NewServer("localhost", 0, sched, cfgProv, "", nil)

	body := `{"key":"auto_security","value":true}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", w.Code, w.Body.String())
	}

	if rescheduleCalled {
		t.Error("reschedule should not be called for non-schedule keys")
	}
}

func TestHandleUpdatePluginConfig_ScheduleNonString(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(nil)
	plugin.Register(mp)

	srv := NewServer("localhost", 0, &mockScheduler{}, &mockConfigProvider{}, "", nil)

	body := `{"key":"schedule","value":42}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdatePluginConfig_RescheduleWarning(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(nil)
	plugin.Register(mp)

	sched := &mockScheduler{
		rescheduleFunc: func(_, _ string) error {
			return errors.New("job not found")
		},
	}
	cfgProv := &mockConfigProvider{}
	srv := NewServer("localhost", 0, sched, cfgProv, "", nil)

	body := `{"key":"schedule","value":"0 6 * * *"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["warning"] == nil {
		t.Error("response should include warning when reschedule fails")
	}
}

func TestHandleUpdatePluginConfig_NilConfigProvider(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(map[string]any{"schedule": "0 3 * * *"})
	plugin.Register(mp)

	// No ConfigProvider — config changes are in-memory only.
	srv := NewServer("localhost", 0, &mockScheduler{}, nil, "", nil)

	body := `{"key":"schedule","value":"0 4 * * *"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", w.Code, w.Body.String())
	}

	if mp.cfg["schedule"] != "0 4 * * *" {
		t.Errorf("plugin not updated: got %v", mp.cfg["schedule"])
	}
}

func TestHandleUpdatePluginConfig_BodyTooLarge(t *testing.T) {
	plugin.ResetForTesting()
	mp := &mockConfigPlugin{}
	mp.Configure(nil)
	plugin.Register(mp)

	srv := NewServer("localhost", 0, &mockScheduler{}, &mockConfigProvider{}, "", nil)

	// Build a body larger than maxConfigBody (64 KB).
	bigValue := bytes.Repeat([]byte("x"), 70*1024)
	body := `{"key":"schedule","value":"` + string(bigValue) + `"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/mockconfig/settings",
		bytes.NewBufferString(body))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

// TestSettingsRouteWithPluginMount verifies that GET/PUT /settings works even
// when the plugin provides its own Routes() handler (which Chi mounts and
// would otherwise shadow the parameterized /plugins/{name}/settings route).
func TestSettingsRouteWithPluginMount_GET(t *testing.T) {
	plugin.ResetForTesting()
	mp := &routedConfigPlugin{}
	mp.Configure(map[string]any{"schedule": "0 5 * * *"})
	plugin.Register(mp)

	srv := NewServer("localhost", 0, nil, nil, "", nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/routed/settings", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	cfg, ok := body["config"].(map[string]any)
	if !ok {
		t.Fatal("response missing 'config' envelope")
	}
	if cfg["schedule"] != "0 5 * * *" {
		t.Errorf("schedule: got %v, want '0 5 * * *'", cfg["schedule"])
	}
}

func TestSettingsRouteWithPluginMount_PUT(t *testing.T) {
	plugin.ResetForTesting()
	mp := &routedConfigPlugin{}
	mp.Configure(map[string]any{"schedule": "0 5 * * *"})
	plugin.Register(mp)

	cfgProv := &mockConfigProvider{
		plugins: map[string]map[string]any{"routed": {"schedule": "0 5 * * *"}},
	}
	sched := &mockScheduler{}
	srv := NewServer("localhost", 0, sched, cfgProv, "", nil)

	payload := `{"key":"schedule","value":"0 6 * * *"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/plugins/routed/settings",
		bytes.NewBufferString(payload))
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	cfg := body["config"].(map[string]any)
	if cfg["schedule"] != "0 6 * * *" {
		t.Errorf("schedule: got %v, want '0 6 * * *'", cfg["schedule"])
	}
	if !cfgProv.saved {
		t.Error("config should have been persisted")
	}
	if v := cfgProv.plugins["routed"]["schedule"]; v != "0 6 * * *" {
		t.Errorf("persisted schedule: got %v, want '0 6 * * *'", v)
	}
}

// TestSettingsRouteWithPluginMount_PluginRouteStillWorks ensures the plugin's
// own routes (e.g. /status) remain functional after /settings injection.
func TestSettingsRouteWithPluginMount_PluginRouteStillWorks(t *testing.T) {
	plugin.ResetForTesting()
	mp := &routedConfigPlugin{}
	mp.Configure(map[string]any{"schedule": "0 5 * * *"})
	plugin.Register(mp)

	srv := NewServer("localhost", 0, nil, nil, "", nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/routed/status", nil)
	srv.httpServer.Handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != `{"status":"ok"}` {
		t.Errorf("body: got %q, want %q", w.Body.String(), `{"status":"ok"}`)
	}
}
