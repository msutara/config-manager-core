package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/msutara/config-manager-core/plugin"
)

// Server wraps an HTTP server that serves the CM Core API.
type Server struct {
	httpServer *http.Server
	scheduler  JobTriggerer
	startTime  time.Time
	errCh      chan error
}

// Err returns a read-only channel that receives fatal start-up errors.
func (s *Server) Err() <-chan error {
	return s.errCh
}

// JobTriggerer is satisfied by the scheduler to trigger jobs by ID.
type JobTriggerer interface {
	TriggerJob(id string) error
	JobExists(id string) bool
}

// slogLogger is a Chi middleware that logs HTTP requests using log/slog.
func slogLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes_written", ww.BytesWritten(),
			"duration", time.Since(start),
		)
	})
}

// NewServer creates a new API server with core and plugin routes mounted.
func NewServer(host string, port int, sched JobTriggerer) *Server {
	r := chi.NewRouter()
	r.Use(slogLogger)
	r.Use(middleware.Recoverer)

	s := &Server{scheduler: sched, startTime: time.Now(), errCh: make(chan error, 1)}

	// Core routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", handleHealth)
		r.Get("/node", s.handleNode)
		r.Get("/plugins", handleListPlugins)
		r.Get("/plugins/{name}", handleGetPlugin)
		r.Get("/jobs", handleListJobs)
		r.Post("/jobs/trigger", s.handleTriggerJob)
	})

	// Plugin routes — compute handlers once, outside the registry lock.
	pluginRoutes := plugin.AllRoutes()
	for name, handler := range pluginRoutes {
		r.Mount(fmt.Sprintf("/api/v1/plugins/%s", name), handler)
	}

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", host, port),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return s
}

// Start begins listening in a goroutine. Call Shutdown to stop.
// Fatal start-up errors (e.g. port in use) are sent to Err().
func (s *Server) Start() {
	go func() {
		slog.Info("API server starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("API server error", "error", err)
			if s.errCh != nil {
				select {
				case s.errCh <- err:
				default:
				}
			}
		}
	}()
}

// Shutdown gracefully stops the API server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("API server shutting down")
	return s.httpServer.Shutdown(ctx)
}
