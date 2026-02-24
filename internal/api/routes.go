package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"runtime"
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
		"uptime_seconds": int(time.Since(s.startTime).Seconds()),
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
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

	jid := req.JobID
	go func() {
		if err := s.scheduler.TriggerJob(jid); err != nil {
			slog.Error("triggered job failed", "job_id", jid, "error", err)
		} else {
			slog.Info("triggered job completed", "job_id", jid)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "accepted",
		"job_id": req.JobID,
	})
}
