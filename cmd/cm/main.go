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
	"github.com/msutara/config-manager-core/internal/scheduler"
	"github.com/msutara/config-manager-core/plugin"
	// Plugins are registered explicitly below in main().
	// Uncomment when plugin modules are added to go.mod:
	// update "github.com/msutara/cm-plugin-update"
	// network "github.com/msutara/cm-plugin-network"
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

	// Register plugins explicitly.
	// Uncomment when plugin modules are added to go.mod:
	// plugin.Register(update.NewUpdatePlugin())
	// plugin.Register(network.NewNetworkPlugin())

	// Apply enabled_plugins filter from config
	plugin.DisableExcept(cfg.EnabledPlugins)

	// Log registered plugins (after filtering)
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

	// Wait for interrupt signal or server error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// TODO: Start TUI here (Phase 2)
	// For now, block until signal or fatal server error
	slog.Info("cm is running (TUI not yet implemented, press Ctrl+C to stop)",
		"api", fmt.Sprintf("http://%s:%d", cfg.ListenHost, cfg.ListenPort),
	)
	select {
	case <-sigCh:
		slog.Info("received shutdown signal")
	case err := <-srv.Err():
		slog.Error("API server failed to start", "error", err)
		os.Exit(1)
	}

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
