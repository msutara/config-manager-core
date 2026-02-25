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

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	configPath := flag.String("config", "", "path to config file (default: /etc/cm/config.yaml)")
	headless := flag.Bool("headless", false, "run without TUI (API server only, for systemd)")
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

	// Initialize logging.
	// In headless mode, log directly to file (no TUI to corrupt).
	// In TUI mode, use tea.LogToFile to redirect the global log package.
	logPath := "/var/log/cm/cm.log"
	var logFileOk bool
	if *headless {
		f, ferr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if ferr != nil {
			logPath = "cm-debug.log"
			f, ferr = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		}
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not open log file: %v\n", ferr)
			logging.Setup(cfg.LogLevel, io.Discard)
		} else {
			defer f.Close()
			logging.Setup(cfg.LogLevel, f)
			logFileOk = true
		}
	} else {
		logFile, lerr := tea.LogToFile(logPath, "cm")
		if lerr != nil {
			logPath = "cm-debug.log"
			logFile, lerr = tea.LogToFile(logPath, "cm")
		}
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not open log file: %v\n", lerr)
			logging.Setup(cfg.LogLevel, io.Discard)
		} else {
			defer logFile.Close()
			logging.Setup(cfg.LogLevel, logFile)
			logFileOk = true
		}
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

	// Track whether a fatal error occurred.
	var exitFailed atomic.Bool

	if *headless {
		// Headless mode: no TUI, block on signal.
		slog.Info("running in headless mode",
			"api", fmt.Sprintf("http://%s:%d", cfg.ListenHost, cfg.ListenPort),
		)

		// Register signal handler before monitor goroutine to avoid race
		// where a fast API failure sends SIGTERM before Notify is active.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		// Monitor API server for fatal errors.
		go func() {
			if err := <-srv.Err(); err != nil {
				exitFailed.Store(true)
				slog.Error("API server failed", "error", err)
				if p, perr := os.FindProcess(os.Getpid()); perr == nil {
					_ = p.Signal(syscall.SIGTERM) //nolint:errcheck // best-effort self-signal
				}
			}
		}()

		// Block until SIGINT or SIGTERM.
		sig := <-sigCh
		signal.Stop(sigCh) // restore OS default so second signal force-kills
		slog.Info("received shutdown signal", "signal", sig)

		// Drain any API error that the monitor goroutine may not have processed.
		select {
		case err := <-srv.Err():
			if err != nil {
				slog.Error("API server failed", "error", err)
				exitFailed.Store(true)
			}
		default:
		}
	} else {
		// TUI mode: build plugin info and run interactive UI.
		var tuiPlugins []tui.PluginInfo
		for _, p := range plugins {
			tuiPlugins = append(tuiPlugins, tui.PluginInfo{
				Name:        p.Name(),
				Description: p.Description(),
			})
		}

		slog.Info("starting TUI",
			"api", fmt.Sprintf("http://%s:%d", cfg.ListenHost, cfg.ListenPort),
			"plugins", len(tuiPlugins),
		)
		model := tui.New(tuiPlugins)
		prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithoutSignalHandler())

		// Monitor API server for fatal errors.
		go func() {
			if err := <-srv.Err(); err != nil {
				exitFailed.Store(true)
				slog.Error("API server failed", "error", err)
				prog.Kill()
			}
		}()

		// Forward SIGINT/SIGTERM to TUI for graceful shutdown.
		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			slog.Info("received shutdown signal")
			signal.Stop(sigCh)
			prog.Quit()
		}()

		if _, err := prog.Run(); err != nil {
			if errors.Is(err, tea.ErrProgramPanic) {
				slog.Error("TUI crashed (panic)", "error", err)
				exitFailed.Store(true)
			} else if !errors.Is(err, tea.ErrInterrupted) && !errors.Is(err, tea.ErrProgramKilled) {
				slog.Error("TUI exited with error", "error", err)
				exitFailed.Store(true)
			}
		}

		// Drain any API error that the monitor goroutine may not have processed.
		select {
		case err := <-srv.Err():
			if err != nil {
				slog.Error("API server failed", "error", err)
				exitFailed.Store(true)
			}
		default:
		}
	}

	// Report fatal errors to stderr (visible in journald for headless,
	// on terminal for TUI after alt-screen restore).
	if exitFailed.Load() {
		if logFileOk {
			fmt.Fprintf(os.Stderr, "fatal: exiting due to error (see %s)\n", logPath)
		} else {
			fmt.Fprintln(os.Stderr, "fatal: exiting due to error (logs unavailable)")
		}
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
