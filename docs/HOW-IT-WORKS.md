# How It Works

## 1. System Design

Config Manager Core is a single Go binary that embeds:

- A **plugin registry** for modular functionality.
- A **TUI** (Bubble Tea) as the primary interface.
- A **REST API** (Chi) running in a background goroutine.
- A **job scheduler** for recurring plugin tasks.

```text
┌─────────────────────────────────────┐
│              cm binary              │
│                                     │
│  ┌─────────┐  ┌──────────────────┐  │
│  │   TUI   │  │    REST API      │  │
│  │ (main)  │  │  (goroutine)     │  │
│  └────┬────┘  └────────┬─────────┘  │
│       │                │            │
│  ┌────┴────────────────┴─────────┐  │
│  │      Plugin Registry          │  │
│  │  ┌────────┐  ┌─────────────┐  │  │
│  │  │ update │  │   network   │  │  │
│  │  └────────┘  └─────────────┘  │  │
│  └───────────────────────────────┘  │
│                                     │
│  ┌───────────────────────────────┐  │
│  │         Scheduler             │  │
│  └───────────────────────────────┘  │
└─────────────────────────────────────┘
```

## 2. Plugin Model

Plugins are separate Go modules compiled into the core binary at build time:

1. Each plugin implements the `Plugin` interface (see [PLUGIN-INTERFACE.md](../specs/PLUGIN-INTERFACE.md)).
2. Plugins export constructors and are registered explicitly in `cmd/cm/main.go`.
3. Adding a plugin = one import + `plugin.Register()` call in `cmd/cm/main.go` + rebuild.

## 3. Startup Sequence

1. Parse CLI flags.
2. Load config from YAML (default: `/etc/cm/config.yaml`).
3. Initialize structured logging.
4. Collect registered plugins from the global registry.
5. Initialize scheduler and register plugin jobs.
6. Start REST API server in a background goroutine.
7. Start TUI as the main blocking loop.
8. Block until TUI exit (SIGINT/SIGTERM or quit action).
9. On exit: gracefully shut down API server and scheduler.

## 4. Configuration

YAML-based configuration loaded from a YAML file.
See [USAGE.md](USAGE.md) for details.
