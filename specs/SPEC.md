# Config Manager Core — Specification

## 1. Purpose

The Config Manager Core (CM Core) is a lightweight management service for
Debian-based, headless nodes. It provides:

- A plugin system for modular functionality (updates, networking, etc.).
- A TUI interface (raspi-config style) for interactive management.
- A REST API for future web UI / remote access.
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
- Provide a web frontend (future scope).

Those concerns are handled by plugins and external tools.

---

## 4. Architecture

- **Core binary:**
  - Go 1.24
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
- Adding a new plugin = one import line + rebuild.

---

## 6. Frontend

- **TUI frontend:**
  - Built with Bubble Tea (Charmbracelet).
  - Runs as the primary interface when `cm` is executed.
  - Discovers plugins via the internal registry.
  - Renders menus dynamically based on plugin metadata.

- **REST API:**
  - Runs embedded (separate goroutine) alongside the TUI.
  - Listens on configurable port (default: `localhost:8080`).
  - Same plugin routes available to future web UI.

---

## 7. Deployment

- Built via `go build ./cmd/cm` → single binary `cm`.
- Cross-compile: `GOOS=linux GOARCH=arm64 go build ./cmd/cm` for Raspbian.
- Delivered as:
  - Direct binary download (wget/curl).
  - Future: `.deb` package.
- Runs as a systemd service: `cm.service`.
- Config file: `/etc/cm/config.yaml` (default path, configurable).
- Logs: systemd journal and optional file logging.
