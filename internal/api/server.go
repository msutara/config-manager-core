package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/msutara/config-manager-core/internal/plugin"
)

// Server wraps an HTTP server that serves the CM Core API.
type Server struct {
	httpServer *http.Server
}

// NewServer creates a new API server with core and plugin routes mounted.
func NewServer(host string, port int) *Server {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Core routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", handleHealth)
		r.Get("/node", handleNode)
		r.Get("/plugins", handleListPlugins)
		r.Get("/plugins/{name}", handleGetPlugin)
		r.Get("/jobs", handleListJobs)
		r.Post("/jobs/trigger", handleTriggerJob)
	})

	// Plugin routes
	for name, p := range plugin.AllRoutes() {
		if handler := p.Routes(); handler != nil {
			r.Mount(fmt.Sprintf("/api/v1/plugins/%s", name), handler)
		}
	}

	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf("%s:%d", host, port),
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Start begins listening in a goroutine. Call Shutdown to stop.
func (s *Server) Start() {
	go func() {
		slog.Info("API server starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("API server error", "error", err)
		}
	}()
}

// Shutdown gracefully stops the API server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("API server shutting down")
	return s.httpServer.Shutdown(ctx)
}
