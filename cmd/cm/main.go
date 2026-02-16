// CM is the Config Manager for headless Debian-based nodes.
// It provides a TUI (raspi-config style) and REST API with a plugin
// architecture for modular system management.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/msutara/config-manager-core/internal/api"
	"github.com/msutara/config-manager-core/internal/config"
	"github.com/msutara/config-manager-core/internal/logging"
	"github.com/msutara/config-manager-core/plugin"
	"github.com/msutara/config-manager-core/internal/scheduler"

	// Import plugins here (build-time registration):
	// _ "github.com/msutara/cm-plugin-update"
	// _ "github.com/msutara/cm-plugin-network"

	// Import TUI:
	// _ "github.com/msutara/config-manager-tui"
)

var version = "0.1.0"

func main() {
	configPath := flag.String("config", "", "path to config file (default: /etc/cm/config.yaml)")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("cm", version)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logging
	logging.Setup(cfg.LogLevel)
	api.Version = version
	slog.Info("starting cm", "version", version)

	// Log registered plugins
	plugins := plugin.List()
	slog.Info("plugins loaded", "count", len(plugins))
	for _, p := range plugins {
		slog.Info("plugin available",
			"name", p.Name(),
			"version", p.Version(),
			"description", p.Description(),
		)
	}

	// Initialize scheduler
	sched := scheduler.New()
	sched.RegisterJobs(plugin.AllJobs())
	sched.Start()

	// Start API server
	srv := api.NewServer(cfg.ListenHost, cfg.ListenPort, sched)
	srv.Start()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// TODO: Start TUI here (Phase 2)
	// For now, block until signal
	slog.Info("cm is running (TUI not yet implemented, press Ctrl+C to stop)",
		"api", fmt.Sprintf("http://%s:%d", cfg.ListenHost, cfg.ListenPort),
	)
	<-sigCh

	// Graceful shutdown
	slog.Info("shutting down")
	sched.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("goodbye")
}
