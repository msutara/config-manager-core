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
- Core injects `GET/PUT /settings` handlers into each plugin's router so the
  settings endpoint is reachable despite the mount prefix shadowing the
  parameterized route.
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
