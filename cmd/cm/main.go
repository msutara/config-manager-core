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
	"net/http"
	"net/url"
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
	network "github.com/msutara/cm-plugin-network"
	update "github.com/msutara/cm-plugin-update"
	tui "github.com/msutara/config-manager-tui"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	configPath := flag.String("config", "", "path to config file (default: /etc/cm/config.yaml)")
	headless := flag.Bool("headless", false, "run without TUI (API server only, for systemd)")
	connectURL := flag.String("connect", "", "connect TUI to running CM service at URL (skip local server)")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("cm", version)
		os.Exit(0)
	}

	if *headless && *connectURL != "" {
		fmt.Fprintln(os.Stderr, "error: --headless and --connect cannot be used together")
		os.Exit(2)
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
	plugin.Register(update.NewUpdatePlugin())
	plugin.Register(network.NewNetworkPlugin())

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

	// Create API server (not started yet — TUI mode probes first).
	srv := api.NewServer(cfg.ListenHost, cfg.ListenPort, sched)

	// Track whether a fatal error occurred.
	var exitFailed atomic.Bool

	// Track whether server/scheduler were started (not started in client mode).
	serverStarted := false

	if *headless {
		// Headless mode: start server, no TUI, block on signal.
		sched.Start()
		srv.Start()
		serverStarted = true
		slog.Info("running in headless mode",
			"api", fmt.Sprintf("http://%s:%d", cfg.ListenHost, cfg.ListenPort),
		)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		// Block until shutdown signal or API server failure.
		select {
		case sig := <-sigCh:
			slog.Info("received shutdown signal", "signal", sig)
		case err := <-srv.Err():
			if err != nil {
				exitFailed.Store(true)
				slog.Error("API server failed", "error", err)
			}
		}

		// Restore OS default so a second signal during shutdown force-kills.
		signal.Stop(sigCh)

		// Drain API error that may have raced with the signal.
		select {
		case err := <-srv.Err():
			if err != nil {
				exitFailed.Store(true)
				slog.Error("API server failed", "error", err)
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

		// Use localhost for the TUI client URL when binding to all interfaces.
		tuiHost := cfg.ListenHost
		if tuiHost == "0.0.0.0" || tuiHost == "::" || tuiHost == "" {
			tuiHost = "localhost"
		}
		apiURL := fmt.Sprintf("http://%s:%d", tuiHost, cfg.ListenPort)

		// Determine connection mode: client (service running) vs standalone.
		clientMode := false
		if *connectURL != "" {
			// Explicit --connect flag: validate and use that URL, force client mode.
			u, uerr := url.Parse(*connectURL)
			if uerr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				fmt.Fprintf(os.Stderr, "error: --connect requires a valid http(s) URL, got %q\n", *connectURL)
				os.Exit(2)
			}
			apiURL = u.Scheme + "://" + u.Host
			clientMode = true
			slog.Info("using explicit service URL", "url", apiURL)
		} else if probeHealth(apiURL) {
			// Auto-detect: service already running at configured port.
			clientMode = true
			slog.Info("detected running service, connecting as client", "url", apiURL)
		}

		if !clientMode {
			// Standalone mode: start our own server and scheduler.
			sched.Start()
			srv.Start()
			serverStarted = true
		}

		connMode := tui.ModeStandalone
		if clientMode {
			connMode = tui.ModeConnected
		}

		slog.Info("starting TUI",
			"api", apiURL,
			"plugins", len(tuiPlugins),
			"mode", connMode,
		)
		model := tui.NewWithAPI(tuiPlugins, apiURL)
		model.SetConnectionMode(connMode)
		prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithoutSignalHandler())

		if !clientMode {
			// Monitor API server for fatal errors (standalone only).
			go func() {
				if err := <-srv.Err(); err != nil {
					exitFailed.Store(true)
					slog.Error("API server failed", "error", err)
					prog.Kill()
				}
			}()
		}

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

		if !clientMode {
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

	// Graceful shutdown (only if server/scheduler were started).
	slog.Info("shutting down")
	if serverStarted {
		sched.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}
	slog.Info("goodbye")

	if exitFailed.Load() {
		os.Exit(1)
	}
}

// probeHealth checks whether a CM service is already running at the given URL.
// Returns true if the health endpoint responds 200 within 1 second.
func probeHealth(baseURL string) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(baseURL + "/api/v1/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}
