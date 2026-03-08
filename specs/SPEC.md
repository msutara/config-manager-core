# Config Manager Core — Specification

## 1. Purpose

The Config Manager Core (CM Core) is a lightweight management service for
Debian-based, headless nodes. It provides:

- A plugin system for modular functionality (updates, networking, etc.).
- A TUI interface (raspi-config style) for interactive management.
- A REST API for the web UI dashboard and remote access.
- A scheduling layer for recurring tasks.

CM Core does not implement domain-specific logic itself; that logic lives in
plugins.

---

## 2. Responsibilities

- **Plugin lifecycle:**
  - Register plugins at build time via Go interface.
  - Expose plugin routes under a unified API namespace.
  - Provide plugin metadata to the TUI for dynamic menu generation.

- **Core API:**
  - Health, node info, plugin listing, job listing/triggering.

- **Scheduling:**
  - Provide a scheduler for plugin-defined jobs.

- **Configuration:**
  - Load global config (port, auth, enabled plugins) from YAML.

- **TUI:**
  - Serve as the primary interface (Bubble Tea).
  - Discover plugins and render menus dynamically.

- **Security:**
  - Provide basic auth/token mechanisms (configurable, future).

- **Observability:**
  - Central structured logging and basic audit trail for plugin actions.

---

## 3. Non-responsibilities

CM Core does **not**:

- Implement OS-specific logic (updates, networking, etc.).
- Handle multi-node orchestration (it is per-node).

Those concerns are handled by plugins and external tools.

---

## 4. Architecture

- **Core binary:**
  - Go 1.22+
  - Single binary: `cm`
  - Embeds TUI (Bubble Tea), REST API (Chi), scheduler, plugin registry.

- **Plugin system:**
  - Plugins are separate Go modules (separate repos).
  - Each plugin implements the `plugin.Plugin` interface.
  - Plugins are imported at build time and registered explicitly in `main.go`.
  - Core mounts plugin routes under `/api/v1/plugins/{name}`.

- **Scheduler:**
  - Internal scheduler for plugin-defined recurring jobs.
  - Cron expressions parsed once and cached (not re-parsed every tick).
  - Star-equivalent expressions (e.g., `*/1`, `1-31`) detected and handled
    with correct AND/OR semantics per standard cron.
  - Overlap protection: if a job is still running when the next tick fires,
    the tick is skipped for that job.
  - Job execution history persisted to a JSON file in `data_dir`. On
    startup, the latest run for each job is restored from disk so
    `LatestRun()` works immediately after restart.
  - Automatic pruning keeps the most recent N records per job
    (`job_history_max_runs`, default 50). Atomic writes (temp + rename)
    prevent corruption on crash.

- **Config:**
  - Struct-based settings loaded from YAML file and environment.

---

## 5. Plugin Model

- Plugins are separate Go modules (often separate repos).
- Each plugin:
  - Implements the `plugin.Plugin` interface defined in this repo.
  - Provides HTTP handler functions for its routes.
  - Optionally defines scheduled jobs.
  - Is imported into `cmd/cm/main.go` at build time.
- Core mounts plugin routes under `/api/v1/plugins/{plugin_name}`.
- Core wraps each plugin handler in a Chi router with `GET/PUT /settings`
  so the settings endpoint is reachable regardless of the plugin's handler
  type (the mount prefix would otherwise shadow the parameterized route).
- Adding a new plugin = one import line + rebuild.

---

## 6. Frontend

- **TUI frontend:**
  - Built with Bubble Tea (Charmbracelet).
  - Runs as the primary interface when `cm` is executed.
  - Discovers plugins via the internal registry.
  - Renders menus dynamically based on plugin metadata.
  - Footer shows connection mode badge (`● connected` or `● standalone`).

- **REST API:**
  - Runs embedded (separate goroutine) alongside the TUI.
  - Listens on configurable port (default: `localhost:7788`).
  - Same plugin routes available to the embedded web UI dashboard.

- **Web UI:**
  - Optional browser-based dashboard (config-manager-web).
  - Connects to the same REST API as the TUI.
  - Served by the core binary when a web handler is provided (e.g., when built with config-manager-web).

---

## 7. Operating Modes

- **Standalone** (default): TUI starts its own embedded API server and
  scheduler. This is the behavior when no service is already running.
- **Connected**: TUI detects a running headless service (via health probe) and
  connects to its API. No duplicate server or scheduler is started.
- **Headless**: `--headless` flag runs the API server only (no TUI), intended
  for systemd service deployment.

Auto-detection: on startup, the TUI probes `GET /api/v1/health` at the
configured URL with a 1-second timeout. If it responds 200, client mode is
used. Override with `--connect URL` to force a specific service endpoint.

---

## 8. Deployment

- Built via `go build ./cmd/cm` → single binary `cm`.
- Cross-compile: `GOOS=linux GOARCH=arm64 go build ./cmd/cm` for Raspbian.
- Delivered as:
  - Direct binary download (wget/curl).
  - Future: `.deb` package.
- Runs as a systemd service: `cm.service`.
- Config file: `/etc/cm/config.yaml` (default path, configurable).
- Logs: systemd journal and optional file logging.

---

## 9. Configuration Reference

CM Core configuration can be set via YAML file (`/etc/cm/config.yaml`) or
environment variables. Environment variables take precedence over YAML values.

| Key                    | Env Var                  | Default        | Description                                                   |
| ---------------------- | ------------------------ | -------------- | ------------------------------------------------------------- |
| `listen_host`          | `CM_LISTEN_HOST`         | `localhost`    | Address to bind the API server                                |
| `listen_port`          | `CM_LISTEN_PORT`         | `7788`         | Port for the API server                                       |
| `log_level`            | `CM_LOG_LEVEL`           | `info`         | Log level (`debug`, `info`, `warn`, `error`)                  |
| `enabled_plugins`      | `CM_ENABLED_PLUGINS`     | (all)          | Comma-separated list of plugins to enable                     |
| `theme`                | `CM_THEME`               | (default)      | TUI theme name or absolute file path                          |
| `data_dir`             | `CM_DATA_DIR`            | `/var/lib/cm`  | Directory for persistent data (job history, etc.)              |
| `storage_backend`      | `CM_STORAGE_BACKEND`     | `json`         | Job history storage backend (`json`; `sqlite` with build tag) |
| `job_history_max_runs` | `CM_JOB_HISTORY_MAX_RUNS`| `50`           | Max run records kept per job (0 = unlimited)                  |

### Storage Backend Registry

Storage backends self-register via Go `init()` functions. The `json` backend
is always available. Additional backends (e.g., `sqlite`) can be compiled in
using build tags. At startup, `storage.New(name, dataDir, maxRuns)` looks up
the named backend in the registry and returns a `JobStore` implementation.
If the requested backend is not registered, startup fails with a clear error
listing the available backends.
