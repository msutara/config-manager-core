package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/msutara/config-manager-core/plugin"
)

// Server wraps an HTTP server that serves the CM Core API.
type Server struct {
	httpServer *http.Server
	scheduler  JobTriggerer
	cfg        ConfigProvider
	cfgMu      sync.RWMutex // serializes config mutations; readers use RLock
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
	Reschedule(id, cron string) error
}

// ConfigProvider abstracts config persistence so the API can update
// plugin config and save to disk without importing the config package.
type ConfigProvider interface {
	PluginConfig(name string) map[string]any
	SetPluginConfig(plugin, key string, value any)
	Save(path string) error
	Path() string
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
// When authToken is non-empty, all endpoints except /api/v1/health require
// a valid Bearer token. If webHandler is non-nil it is mounted at the root
// for the browser-based dashboard.
func NewServer(host string, port int, sched JobTriggerer, cfg ConfigProvider, authToken string, webHandler http.Handler) *Server {
	r := chi.NewRouter()
	r.Use(slogLogger)
	r.Use(middleware.Recoverer)

	s := &Server{scheduler: sched, cfg: cfg, startTime: time.Now(), errCh: make(chan error, 1)}

	// Health endpoint — public, no auth required (used for auto-detection).
	r.Get("/api/v1/health", handleHealth)

	// All other routes require Bearer token (when configured).
	r.Group(func(r chi.Router) {
		r.Use(BearerAuth(authToken))

		r.Route("/api/v1", func(r chi.Router) {
			r.Get("/node", s.handleNode)
			r.Get("/plugins", handleListPlugins)
			r.Get("/plugins/{name}", handleGetPlugin)
			r.Get("/plugins/{name}/settings", s.handleGetPluginConfig)
			r.Put("/plugins/{name}/settings", s.handleUpdatePluginConfig)
			r.Get("/jobs", handleListJobs)
			r.Post("/jobs/trigger", s.handleTriggerJob)
		})

		// Plugin routes — compute handlers once, outside the registry lock.
		// Wrap each plugin handler in a Chi router so /settings GET/PUT
		// is always reachable regardless of the underlying handler type.
		// Without this, r.Mount's literal prefix shadows the parameterized
		// /plugins/{name}/settings route.
		pluginRoutes := plugin.AllRoutes()
		for name, handler := range pluginRoutes {
			n := name
			wrapper := chi.NewRouter()
			wrapper.Mount("/", handler)
			wrapper.Get("/settings", s.pluginConfigHandler(n))
			wrapper.Put("/settings", s.pluginConfigUpdateHandler(n))
			r.Mount(plugin.RouteBase+name, wrapper)
		}
	})

	// Web UI dashboard — mounted after API routes so /api/v1/* takes priority.
	if webHandler != nil {
		r.Mount("/", webHandler)
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
