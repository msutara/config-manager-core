// CM is the Config Manager for headless Debian-based nodes.
// It provides a TUI (raspi-config style) and REST API with a plugin
// architecture for modular system management.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/msutara/config-manager-core/internal/api"
	"github.com/msutara/config-manager-core/internal/config"
	"github.com/msutara/config-manager-core/internal/logging"
	"github.com/msutara/config-manager-core/internal/scheduler"
	"github.com/msutara/config-manager-core/internal/storage"
	"github.com/msutara/config-manager-core/plugin"

	tea "github.com/charmbracelet/bubbletea"
	network "github.com/msutara/cm-plugin-network"
	update "github.com/msutara/cm-plugin-update"
	tui "github.com/msutara/config-manager-tui"
	web "github.com/msutara/config-manager-web"
)

// version is set at build time via -ldflags.
var version = "dev"

const defaultTokenPath = "/etc/cm/auth.token"

// loadToken reads the auth token from the given file. Returns empty string
// only when the file does not exist (auth disabled). Permission errors and
// empty/whitespace-only files cause a fatal exit to prevent silently
// disabling auth.
func loadToken(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		fmt.Fprintf(os.Stderr, "fatal: cannot read auth token %s: %v\n", path, err)
		os.Exit(1)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		fmt.Fprintf(os.Stderr, "fatal: auth token file %s is empty\n", path)
		os.Exit(1)
	}
	return token
}

// generateToken writes a new random 32-byte hex token to the given file.
func generateToken(path string) error {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("generate random bytes: %w", err)
	}
	token := hex.EncodeToString(b)
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	return nil
}

func main() {
	configPath := flag.String("config", "", "path to config file (default: /etc/cm/config.yaml)")
	headless := flag.Bool("headless", false, "run without TUI (API server only, for systemd)")
	connectURL := flag.String("connect", "", "connect TUI to running CM service at URL (skip local server)")
	rotateToken := flag.Bool("rotate-token", false, "generate a new auth token and exit")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("cm", version)
		os.Exit(0)
	}

	if *rotateToken {
		if err := generateToken(defaultTokenPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Token written to", defaultTokenPath)
		fmt.Println("Restart the service to apply: sudo systemctl restart cm")
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

	// Pass persisted config to plugins that support it.
	for _, p := range plugin.List() {
		if c, ok := p.(plugin.Configurable); ok {
			raw := cfg.PluginConfig(p.Name())
			if raw != nil {
				safe := make(map[string]any, len(raw))
				for k, v := range raw {
					safe[k] = v
				}
				raw = safe
			}
			c.Configure(raw)
		}
	}

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

	// Load auth token from file (empty = auth disabled).
	authToken := loadToken(defaultTokenPath)
	if authToken != "" {
		slog.Info("bearer auth enabled", "token_file", defaultTokenPath)
	} else {
		slog.Info("bearer auth disabled (no token file)")
	}

	// Initialize scheduler with persistent job history store.
	if strings.TrimSpace(cfg.DataDir) == "" {
		slog.Error("invalid configuration: data_dir must not be empty")
		os.Exit(1)
	}
	store, err := storage.New(cfg.StorageBackend, cfg.DataDir, cfg.JobHistoryMaxRuns)
	if err != nil {
		slog.Error("failed to initialize storage backend", "backend", cfg.StorageBackend, "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := store.Close(); err != nil {
			slog.Error("failed to close storage backend", "backend", cfg.StorageBackend, "error", err)
		}
	}()
	sched := scheduler.New(store)
	sched.RegisterJobs(plugin.AllJobs())
	sched.LoadHistory()

	// Build the API base URL for the web UI client (loopback).
	webHost := cfg.ListenHost
	if webHost == "0.0.0.0" || webHost == "::" || webHost == "" {
		webHost = "localhost"
	}
	apiBaseURL := fmt.Sprintf("http://%s:%d", webHost, cfg.ListenPort)

	// Create web UI handler (browser-based dashboard).
	webHandler := web.NewHandler(apiBaseURL, authToken)

	// Create API server (not started yet — TUI mode probes first).
	srv := api.NewServer(cfg.ListenHost, cfg.ListenPort, sched, cfg, authToken, webHandler)

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

		// Use the pre-computed API base URL for TUI client.
		apiURL := apiBaseURL

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
		model := tui.NewWithAuth(tuiPlugins, apiURL, authToken)
		model.SetConnectionMode(connMode)

		if theme := resolveTheme(cfg.Theme); theme != nil {
			model.SetTheme(*theme)
		}

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
	_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // best-effort drain for connection reuse
	return resp.StatusCode == http.StatusOK
}

// maxThemeFileSize is the maximum allowed theme file size (1 MB).
const maxThemeFileSize = 1 << 20

// resolveTheme resolves a theme name or file path to a tui.Theme.
// Returns nil when no theme is configured or resolution fails (falls back
// to the TUI default). File paths must be absolute and cannot contain "..".
func resolveTheme(name string) *tui.Theme {
	if name == "" {
		return nil
	}

	// Try built-in theme first.
	if th, ok := tui.BuiltinTheme(name); ok {
		slog.Info("using built-in theme", "name", name)
		return &th
	}

	// Validate path: must be absolute, no traversal components.
	// Check raw input for ".." BEFORE Clean resolves them.
	if !filepath.IsAbs(name) || strings.Contains(name, "..") {
		slog.Warn("invalid theme path (must be absolute, no ..)", "theme", name)
		return nil
	}
	cleaned := filepath.Clean(name)

	// Read file with size limit to prevent OOM from large files or devices.
	f, err := os.Open(cleaned)
	if err != nil {
		slog.Warn("theme not found as built-in or file, using default", "theme", name, "error", err)
		return nil
	}
	defer f.Close()

	// Reject non-regular files (FIFOs, device nodes) that could block reads.
	info, err := f.Stat()
	if err != nil {
		slog.Warn("failed to stat theme file, using default", "path", cleaned, "error", err)
		return nil
	}
	if !info.Mode().IsRegular() {
		slog.Warn("theme path is not a regular file, using default", "path", cleaned, "mode", info.Mode())
		return nil
	}
	if info.Size() > int64(maxThemeFileSize) {
		slog.Warn("theme file too large, using default", "path", cleaned, "size", info.Size(), "limit", maxThemeFileSize)
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(f, int64(maxThemeFileSize)+1))
	if err != nil {
		slog.Warn("failed to read theme file, using default", "path", cleaned, "error", err)
		return nil
	}
	if len(data) > maxThemeFileSize {
		slog.Warn("theme file too large (post-read check), using default", "path", cleaned, "limit", maxThemeFileSize)
		return nil
	}

	th, err := tui.ThemeFromYAML(data)
	if err != nil {
		slog.Warn("invalid theme file, using default", "path", cleaned, "error", err)
		return nil
	}

	slog.Info("loaded theme from file", "path", cleaned)
	return &th
}
