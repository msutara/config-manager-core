package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/msutara/config-manager-core/plugin"
)

// Version is set by the main package at startup.
var Version = "0.1.0"

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody holds error details.
type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{
		Error: ErrorBody{Code: code, Message: message, Details: struct{}{}},
	})
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": Version,
	})
}

// procUptimePath is the file read by systemUptime. Tests override this to
// inject failures or custom content.
var procUptimePath = "/proc/uptime"

// systemUptime reads /proc/uptime and returns the system uptime in seconds.
// Falls back to service uptime (from startTime) if /proc/uptime cannot be read
// or parsed, or if it contains an invalid uptime value.
func systemUptime(startTime time.Time) int {
	fallback := int(time.Since(startTime).Seconds())

	data, err := os.ReadFile(procUptimePath)
	if err != nil {
		return fallback
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return fallback
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || math.IsNaN(secs) || math.IsInf(secs, 0) || secs < 0 {
		return fallback
	}
	return int(secs)
}

func (s *Server) handleNode(w http.ResponseWriter, _ *http.Request) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	osRelease := "unknown"
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				osRelease = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
		}
	}

	kernel := "unknown"
	if data, err := os.ReadFile("/proc/version"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			kernel = parts[2]
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"hostname":       hostname,
		"os":             osRelease,
		"kernel":         kernel,
		"uptime_seconds": systemUptime(s.startTime),
		"arch":           runtime.GOARCH,
	})
}

func handleListPlugins(w http.ResponseWriter, _ *http.Request) {
	plugins := plugin.List()
	meta := make([]plugin.Metadata, 0, len(plugins))
	for _, p := range plugins {
		meta = append(meta, plugin.MetadataFrom(p))
	}
	writeJSON(w, http.StatusOK, meta)
}

func handleGetPlugin(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	p, ok := plugin.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "plugin_not_found",
			"Plugin '"+name+"' not found")
		return
	}
	writeJSON(w, http.StatusOK, plugin.MetadataFrom(p))
}

func handleListJobs(w http.ResponseWriter, _ *http.Request) {
	jobs := plugin.AllJobs()
	type jobResponse struct {
		ID          string  `json:"id"`
		Plugin      string  `json:"plugin"`
		Description string  `json:"description"`
		Schedule    *string `json:"schedule"`
		NextRunTime *string `json:"next_run_time"`
	}

	result := make([]jobResponse, 0, len(jobs))
	for _, j := range jobs {
		parts := strings.SplitN(j.ID, ".", 2)
		pluginName := ""
		if len(parts) > 0 {
			pluginName = parts[0]
		}
		var sched *string
		if j.Cron != "" {
			s := j.Cron
			sched = &s
		}
		result = append(result, jobResponse{
			ID:          j.ID,
			Plugin:      pluginName,
			Description: j.Description,
			Schedule:    sched,
			NextRunTime: nil, // Phase 2: computed from cron expression
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleTriggerJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobID string `json:"job_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxTriggerBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		}
		return
	}
	// Reject trailing data so MaxBytesReader cannot be bypassed.
	if err := dec.Decode(&json.RawMessage{}); err != io.EOF {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", "Request body must contain exactly one JSON object")
		}
		return
	}

	if s.scheduler == nil {
		writeError(w, http.StatusInternalServerError, "scheduler_unavailable", "Scheduler not configured")
		return
	}

	if req.JobID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "job_id is required")
		return
	}

	if !s.scheduler.JobExists(req.JobID) {
		writeError(w, http.StatusNotFound, "job_not_found",
			"Job '"+req.JobID+"' not found")
		return
	}

	if err := s.scheduler.TriggerJobAsync(req.JobID); err != nil {
		slog.Error("failed to trigger job", "job_id", req.JobID, "error", err)
		writeError(w, http.StatusInternalServerError, "trigger_failed",
			"Failed to trigger job; see server logs")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "accepted",
		"job_id": req.JobID,
	})
}

func (s *Server) handleGetLatestRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.scheduler == nil {
		writeError(w, http.StatusInternalServerError, "scheduler_unavailable", "Scheduler not configured")
		return
	}
	if !s.scheduler.JobExists(id) {
		writeError(w, http.StatusNotFound, "job_not_found",
			"Job '"+id+"' not found")
		return
	}
	run := s.scheduler.LatestRun(id)
	if run == nil {
		writeError(w, http.StatusNotFound, "no_runs", "No runs recorded for job '"+id+"'")
		return
	}
	// Sanitize the error field to avoid leaking internal details from plugin jobs.
	sanitized := *run
	if sanitized.Error != "" {
		sanitized.Error = "job failed; see server logs"
	}
	writeJSON(w, http.StatusOK, &sanitized)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.scheduler == nil {
		writeError(w, http.StatusInternalServerError, "scheduler_unavailable", "Scheduler not configured")
		return
	}
	if !s.scheduler.JobExists(id) {
		writeError(w, http.StatusNotFound, "job_not_found",
			"Job '"+id+"' not found")
		return
	}

	limit := 20
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid_parameter", "limit must be a positive integer")
			return
		}
		if n > 100 {
			n = 100
		}
		limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_parameter", "offset must be a non-negative integer")
			return
		}
		offset = n
	}

	runs, err := s.scheduler.ListRuns(id, limit, offset)
	if err != nil {
		slog.Error("failed to list job runs", "job_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to retrieve job history")
		return
	}
	// Sanitize error fields to avoid leaking internal details from plugin jobs.
	for i := range runs {
		if runs[i].Error != "" {
			runs[i].Error = "job failed; see server logs"
		}
	}
	writeJSON(w, http.StatusOK, runs)
}

// maxConfigBody is the maximum request body for config updates (64 KB).
const maxConfigBody = 64 << 10

// maxTriggerBody is the maximum request body for job trigger requests (1 KB).
const maxTriggerBody = 1 << 10

// getConfigurablePlugin resolves a plugin by name and asserts it implements
// Configurable. Returns the Configurable and true on success; writes an
// error response and returns false otherwise.
func getConfigurablePlugin(w http.ResponseWriter, name string) (plugin.Configurable, bool) {
	p, ok := plugin.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "plugin_not_found",
			"Plugin '"+name+"' not found")
		return nil, false
	}
	c, ok := p.(plugin.Configurable)
	if !ok {
		writeError(w, http.StatusNotImplemented, "not_configurable",
			"Plugin '"+name+"' does not support configuration")
		return nil, false
	}
	return c, true
}

// pluginConfigHandler returns a GET handler for a specific plugin's config.
// Used to inject /settings into plugin routers where chi.URLParam is unavailable.
func (s *Server) pluginConfigHandler(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.getPluginConfig(w, r, name)
	}
}

// pluginConfigUpdateHandler returns a PUT handler for a specific plugin's config.
// Used to inject /settings into plugin routers where chi.URLParam is unavailable.
func (s *Server) pluginConfigUpdateHandler(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.updatePluginConfig(w, r, name)
	}
}

func (s *Server) handleGetPluginConfig(w http.ResponseWriter, r *http.Request) {
	s.getPluginConfig(w, r, chi.URLParam(r, "name"))
}

func (s *Server) handleUpdatePluginConfig(w http.ResponseWriter, r *http.Request) {
	s.updatePluginConfig(w, r, chi.URLParam(r, "name"))
}

// getPluginConfig is the shared implementation for GET /settings.
func (s *Server) getPluginConfig(w http.ResponseWriter, _ *http.Request, name string) {
	c, ok := getConfigurablePlugin(w, name)
	if !ok {
		return
	}
	cfg := func() map[string]any {
		s.cfgMu.RLock()
		defer s.cfgMu.RUnlock()
		return copyMap(c.CurrentConfig())
	}()
	writeJSON(w, http.StatusOK, map[string]any{"config": cfg})
}

// updatePluginConfig is the shared implementation for PUT /settings.
func (s *Server) updatePluginConfig(w http.ResponseWriter, r *http.Request, name string) {
	c, ok := getConfigurablePlugin(w, name)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxConfigBody)
	var req struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		}
		return
	}
	// Reject trailing data so MaxBytesReader cannot be bypassed.
	if err := dec.Decode(&json.RawMessage{}); err != io.EOF {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", "Request body must contain exactly one JSON object")
		}
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "key is required")
		return
	}

	// Validate that schedule values are strings before accepting.
	if req.Key == "schedule" {
		if _, ok := req.Value.(string); !ok {
			slog.Warn("schedule value type mismatch",
				"plugin", name, "type", fmt.Sprintf("%T", req.Value))
			writeError(w, http.StatusBadRequest, "invalid_config",
				"Invalid configuration value; see server logs for details")
			return
		}
	}

	// Serialize the entire update (plugin state + persistence + response read)
	// to prevent concurrent map writes on the plugin's in-memory config.
	cfg, err := func() (map[string]any, error) {
		s.cfgMu.Lock()
		defer s.cfgMu.Unlock()
		return s.applyConfigUpdate(name, c, req.Key, req.Value)
	}()
	if err != nil {
		if err == errSaveFailed {
			writeError(w, http.StatusInternalServerError, "save_failed",
				"Config applied but failed to persist; see server logs for details")
		} else {
			slog.Warn("plugin config validation failed",
				"plugin", name, "key", req.Key, "error", err)
			writeError(w, http.StatusBadRequest, "invalid_config",
				"Invalid configuration value; see server logs for details")
		}
		return
	}

	// If schedule changed, notify the scheduler.
	var warning string
	if req.Key == "schedule" && s.scheduler != nil {
		cron := req.Value.(string)  // safe: validated above
		jobID := name + ".security" // convention: {plugin}.{job}
		if err := s.scheduler.Reschedule(jobID, cron); err != nil {
			slog.Warn("reschedule failed after config update",
				"job_id", jobID, "cron", cron, "error", err)
			warning = "config saved but scheduler update failed; see server logs for details"
		}
	}

	slog.Info("plugin config updated", "plugin", name, "key", req.Key)

	resp := map[string]any{"config": cfg}
	if warning != "" {
		resp["warning"] = warning
	}
	writeJSON(w, http.StatusOK, resp)
}

// sentinel used to distinguish save failures from plugin validation errors.
var errSaveFailed = errors.New("save failed")

// applyConfigUpdate performs the plugin update, persistence, and config
// snapshot. Must be called while holding cfgMu.
func (s *Server) applyConfigUpdate(name string, c plugin.Configurable, key string, value any) (map[string]any, error) {
	if err := c.UpdateConfig(key, value); err != nil {
		return nil, err
	}

	if s.cfg != nil {
		s.cfg.SetPluginConfig(name, key, value)
		if err := s.cfg.Save(s.cfg.Path()); err != nil {
			slog.Error("failed to save config", "error", err)
			return nil, errSaveFailed
		}
	}

	return copyMap(c.CurrentConfig()), nil
}

// copyMap returns a shallow copy of the map, safe for use outside a lock.
// Shallow copy is sufficient: config values are JSON primitives (string,
// bool, float64) — no nested mutable objects in current design.
func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
