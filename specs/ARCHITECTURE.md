# Config Manager Core — Architecture

## 1. Directory layout

```txt
config-manager-core/
  cmd/
    cm/
      main.go              Entry point
  internal/
    plugin/
      plugin.go            Plugin interface definition
      registry.go          Plugin registry
    config/
      config.go            Configuration loading
    api/
      server.go            HTTP server (Chi)
      routes.go            Core API routes
    scheduler/
      scheduler.go         Job scheduler
    logging/
      logging.go           Structured logging setup
  specs/                   Agent-readable specifications
  docs/                    User-facing documentation
  .github/
    copilot-instructions.md
    workflows/
      ci.yml
  go.mod
  go.sum
  README.md
  LICENSE
```

---

## 2. Core components

### 2.1. Entry point (`cmd/cm/main.go`)

- Parses CLI flags (if any).
- Loads configuration.
- Initializes logging.
- Initializes plugin registry (plugins self-register via `init()`).
- Initializes scheduler and registers plugin jobs.
- Starts HTTP API server in a background goroutine.
- Starts TUI (Bubble Tea) as the main loop.

### 2.2. Configuration (`internal/config/config.go`)

- Go struct with YAML tags.
- Sources:
  - Config file (YAML, default `/etc/cm/config.yaml`).
  - Environment variables (override).
- Key settings:
  - `listen_host`, `listen_port`
  - `auth_mode`, `auth_token` (future)
  - `enabled_plugins` (optional, all enabled by default)
  - `log_level`

---

## 3. API layer

### 3.1. Core routes (`internal/api/routes.go`)

Implements:

- `GET /api/v1/health`
- `GET /api/v1/node`
- `GET /api/v1/plugins`
- `GET /api/v1/plugins/{name}`
- `GET /api/v1/jobs`
- `POST /api/v1/jobs/trigger`

### 3.2. Server (`internal/api/server.go`)

- Creates a Chi router.
- Mounts core routes under `/api/v1`.
- Mounts plugin routes under `/api/v1/plugins/{plugin_name}`.
- Runs in a goroutine alongside the TUI.

---

## 4. Plugin system

### 4.1. Interface (`internal/plugin/plugin.go`)

Defines the `Plugin` interface:

```go
type Plugin interface {
    Name() string
    Version() string
    Description() string
    Routes() http.Handler
    ScheduledJobs() []JobDefinition
}
```

### 4.2. Registry (`internal/plugin/registry.go`)

- Global registry populated by plugin `init()` functions.
- Provides:
  - `Register(p Plugin)`
  - `List() []Plugin`
  - `Get(name string) (Plugin, bool)`
  - `AllRoutes() map[string]http.Handler`
  - `AllJobs() []JobDefinition`

---

## 5. Scheduler

### 5.1. Scheduler (`internal/scheduler/scheduler.go`)

- Simple cron-based scheduler (internal implementation or third-party).
- Registers jobs from plugins.
- Exposes:
  - `ListJobs() []Job`
  - `TriggerJob(id string) error`
- Job IDs are globally unique: `{plugin_name}.{job_name}`.

---

## 6. Logging

### 6.1. Logging setup (`internal/logging/logging.go`)

- Structured logging via `log/slog` (Go 1.21+ standard library).
- Output to:
  - stdout (for systemd journal).
  - Optional file.
- Provides a standard logger for plugins: `slog.With("plugin", name)`.

---

## 7. Startup sequence

1. Parse CLI flags.
2. Load configuration from YAML + env overrides.
3. Initialize structured logging.
4. Collect registered plugins from the global registry.
5. Initialize scheduler and register plugin jobs.
6. Build HTTP server:
   - Mount core routes.
   - Mount plugin routes.
7. Start HTTP server in a goroutine.
8. Start TUI as the main blocking loop.
9. On TUI exit, gracefully shut down HTTP server and scheduler.

---

## 8. Error handling

- **Plugin registration:**
  - Log error, skip faulty plugin.
- **API errors:**
  - Return structured JSON error responses (see API.md).
- **Scheduler errors:**
  - Log job failures with plugin and job ID.
- **TUI errors:**
  - Display error in TUI, allow retry or back navigation.
