# Config Manager Core — Architecture

## 1. Directory Structure

```text
config-manager-core/
  cmd/
    cm/
      main.go              Entry point
  plugin/
    plugin.go              Plugin interface definition (public)
    registry.go            Plugin registry (public)
  internal/
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

## 2. Components

### Entry point (`cmd/cm/main.go`)

- Parses CLI flags (if any).
- Loads configuration.
- Initializes logging.
- Initializes plugin registry (core registers plugins explicitly in `main.go`).
- Initializes scheduler and registers plugin jobs.
- Starts HTTP API server in a background goroutine.
- Starts TUI (Bubble Tea) as the main loop with dynamic plugin menus.
- On TUI exit, gracefully shuts down HTTP server and scheduler.

### Core routes (`internal/api/routes.go`)

Implements:

- `GET /api/v1/health`
- `GET /api/v1/node`
- `GET /api/v1/plugins`
- `GET /api/v1/plugins/{name}`
- `GET /api/v1/jobs`
- `POST /api/v1/jobs/trigger`

### Server (`internal/api/server.go`)

- Creates a Chi router.
- Mounts core routes under `/api/v1`.
- Mounts plugin routes under `/api/v1/plugins/{plugin_name}`.
- Runs in a goroutine alongside the TUI.

### Plugin interface (`plugin/plugin.go`)

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

### Plugin registry (`plugin/registry.go`)

- Global registry populated by explicit `plugin.Register()` calls in `main.go`.
- Provides:
  - `Register(p Plugin)`
  - `List() []Plugin`
  - `Get(name string) (Plugin, bool)`
  - `AllRoutes() map[string]http.Handler`
  - `AllJobs() []JobDefinition`

### Scheduler (`internal/scheduler/scheduler.go`)

- Simple cron-based scheduler (internal implementation or third-party).
- Registers jobs from plugins.
- Exposes:
  - `ListJobs() []Job`
  - `TriggerJob(id string) error`
- Job IDs are globally unique: `{plugin_name}.{job_name}`.

### Logging (`internal/logging/logging.go`)

- Structured logging via `log/slog` (Go 1.21+ standard library).
- Output to:
  - stdout (for systemd journal).
  - Optional file.
- Provides a standard logger for plugins: `slog.With("plugin", name)`.

### Error handling

- **Plugin registration:**
  - Log error, skip faulty plugin.
- **API errors:**
  - Return structured JSON error responses (see API.md).
- **Scheduler errors:**
  - Log job failures with plugin and job ID.
- **TUI errors:**
  - Display error in TUI, allow retry or back navigation.

---

## 3. Startup Sequence

1. Parse CLI flags.
2. Load configuration from YAML file.
3. Initialize structured logging.
4. Collect registered plugins from the global registry.
5. Initialize scheduler and register plugin jobs.
6. Build HTTP server:
   - Mount core routes.
   - Mount plugin routes.
7. Start HTTP server in a goroutine.
8. Start TUI as the main blocking loop (with plugin info from registry).
9. On TUI exit, gracefully shut down HTTP server and scheduler.

---

## 4. Configuration

Configuration is handled in `internal/config/config.go`:

- Go struct with YAML tags.
- Sources (Phase 1):
  - Config file (YAML, default `/etc/cm/config.yaml`).
  - Environment variables (`CM_LISTEN_HOST`, `CM_LISTEN_PORT`, `CM_LOG_LEVEL`,
    `CM_ENABLED_PLUGINS`) override YAML values.
- Key settings (Phase 1 implementation):
  - `listen_host`, `listen_port`
  - `enabled_plugins` (optional, all enabled by default)
  - `log_level`
- Planned future settings:
  - `auth_mode`, `auth_token`
