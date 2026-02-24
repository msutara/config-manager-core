# Copilot Instructions

## Project Overview

Config Manager Core is the central service for managing headless Debian-based nodes. It provides a plugin system, TUI interface (raspi-config style via Bubble Tea), embedded REST API (Chi), and a job scheduler — all compiled into a single Go binary.

Target platforms: Raspbian Bookworm (ARM64), Debian Bullseye slim.

## Architecture

- **Entry point**: `cmd/cm/main.go` — parses flags, loads config, initializes plugins/scheduler/API, starts TUI
- **Plugin interface**: `plugin/plugin.go` — defines the `Plugin` interface that all plugins implement
- **Plugin registry**: `plugin/registry.go` — global registry; plugins registered explicitly in `main.go`
- **Configuration**: `internal/config/config.go` — YAML config with struct tags
- **API server**: `internal/api/server.go` — Chi router, runs in a goroutine alongside TUI
- **API routes**: `internal/api/routes.go` — core endpoints: health, node info, plugins, jobs
- **Scheduler**: `internal/scheduler/scheduler.go` — registers and triggers plugin-defined jobs
- **Logging**: `internal/logging/logging.go` — structured logging via `log/slog`

## Plugin Model

Plugins are separate Go modules in their own repos:

- `github.com/msutara/cm-plugin-update` — OS/package update management
- `github.com/msutara/cm-plugin-network` — network configuration

Each plugin:

1. Implements `plugin.Plugin` interface
2. Exports a constructor (e.g., `NewUpdatePlugin()`)
3. Is imported and registered explicitly in `cmd/cm/main.go`:
   `plugin.Register(update.NewUpdatePlugin())`

## Conventions

- Use `internal/` for all non-exported packages
- Plugin routes are mounted under `/api/v1/plugins/{name}`
- Job IDs follow the pattern `{plugin_name}.{job_name}`
- Use `log/slog` for all logging (structured, with plugin name)
- Config file: YAML at `/etc/cm/config.yaml` (default)
- Error responses use the standard `{"error": {"code": ..., "message": ..., "details": ...}}` format
- Use `gopkg.in/yaml.v3` for YAML parsing
- Use `github.com/go-chi/chi/v5` for HTTP routing

## Specifications

Agent-readable specifications live in `specs/`:

- `specs/SPEC.md` — what the core does and doesn't do
- `specs/ARCHITECTURE.md` — directory layout, components, startup sequence
- `specs/PLUGIN-INTERFACE.md` — the Plugin interface contract
- `specs/API.md` — REST API endpoints and JSON schemas

User-facing documentation lives in `docs/`.

## Validation

- All Go code must pass `golangci-lint run`
- All tests must pass: `go test ./...`
- CI runs markdownlint + lint + test via `.github/workflows/ci.yml`
- Never push directly to main — always use feature branches and PRs
