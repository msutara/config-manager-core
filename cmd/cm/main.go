// CM is the Config Manager for headless Debian-based nodes.
// It provides a TUI (raspi-config style) and REST API with a plugin
// architecture for modular system management.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/msutara/config-manager-core/internal/api"
	"github.com/msutara/config-manager-core/internal/config"
	"github.com/msutara/config-manager-core/internal/logging"
	"github.com/msutara/config-manager-core/internal/scheduler"
	"github.com/msutara/config-manager-core/plugin"

	tea "github.com/charmbracelet/bubbletea"
	tui "github.com/msutara/config-manager-tui"
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

	// Initialize logging — redirect to file so TUI display is not corrupted.
	logFile, err := tea.LogToFile("cm-debug.log", "cm")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open log file: %v\n", err)
		logging.Setup(cfg.LogLevel, io.Discard)
	} else {
		defer logFile.Close()
		logging.Setup(cfg.LogLevel, logFile)
	}
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

	// Build TUI plugin info from registered plugins
	var tuiPlugins []tui.PluginInfo
	for _, p := range plugins {
		tuiPlugins = append(tuiPlugins, tui.PluginInfo{
			Name:        p.Name(),
			Description: p.Description(),
		})
	}

	// Start TUI as the main blocking loop
	slog.Info("starting TUI",
		"api", fmt.Sprintf("http://%s:%d", cfg.ListenHost, cfg.ListenPort),
		"plugins", len(tuiPlugins),
	)
	model := tui.New(tuiPlugins)
	prog := tea.NewProgram(model, tea.WithAltScreen())

	// Track whether a fatal error occurred (API failure or TUI crash).
	var exitFailed atomic.Bool

	// Monitor API server for fatal startup errors (e.g., port in use).
	// Stderr output is deferred to after prog.Run() returns so it doesn't
	// corrupt the TUI alternate screen.
	go func() {
		if err := <-srv.Err(); err != nil {
			slog.Error("API server failed", "error", err)
			exitFailed.Store(true)
			prog.Kill()
		}
	}()

	// Forward SIGINT/SIGTERM to TUI for graceful shutdown.
	// After the first signal, stop intercepting so a second signal
	// terminates immediately via the OS default handler.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("received shutdown signal")
		prog.Quit()
		signal.Stop(sigCh)
	}()

	if _, err := prog.Run(); err != nil {
		// ErrInterrupted is normal (user Ctrl+C); ErrProgramKilled is handled
		// by the API monitor goroutine which already sets exitFailed.
		// ErrProgramPanic wraps ErrProgramKilled, so check it first.
		if errors.Is(err, tea.ErrProgramPanic) {
			slog.Error("TUI crashed (panic)", "error", err)
			exitFailed.Store(true)
		} else if !errors.Is(err, tea.ErrInterrupted) && !errors.Is(err, tea.ErrProgramKilled) {
			slog.Error("TUI exited with error", "error", err)
			exitFailed.Store(true)
		}
	}

	// Drain any API error that the monitor goroutine may not have processed
	// yet (e.g., signal-induced exit raced with API failure).
	select {
	case err := <-srv.Err():
		if err != nil {
			slog.Error("API server failed", "error", err)
			exitFailed.Store(true)
		}
	default:
	}

	// Now that the TUI has restored the terminal, report fatal errors.
	if exitFailed.Load() {
		fmt.Fprintln(os.Stderr, "fatal: exiting due to startup error (see cm-debug.log)")
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

	if exitFailed.Load() {
		os.Exit(1)
	}
}
