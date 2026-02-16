# Config Manager Core

Core service for managing headless Debian-based nodes.
Provides a plugin system, TUI interface (raspi-config style), REST API,
and job scheduler — all in a single binary.

## Features

- **Plugin architecture** — modular, separate repos compiled into one binary
- **TUI interface** (planned) — interactive menus via Bubble Tea (raspi-config style)
- **REST API** — embedded HTTP server for remote access and future web UI
- **Job scheduler** — cron-based recurring tasks defined by plugins
- **Single binary** — cross-compile for ARM, no runtime dependencies

## Quick Start

```bash
# Build
go build -o cm ./cmd/cm

# Cross-compile for Raspbian (ARM64)
GOOS=linux GOARCH=arm64 go build -o cm ./cmd/cm

# Run
./cm
./cm --config /path/to/config.yaml
./cm --version
```

## Documentation

- [Usage Guide](docs/USAGE.md) — CLI options and configuration
- [How It Works](docs/HOW-IT-WORKS.md) — architecture and plugin model

## Specifications

- [SPEC.md](specs/SPEC.md) — core specification
- [ARCHITECTURE.md](specs/ARCHITECTURE.md) — internal structure
- [PLUGIN-INTERFACE.md](specs/PLUGIN-INTERFACE.md) — plugin contract
- [API.md](specs/API.md) — REST API specification

## License

See [LICENSE](LICENSE) for details.
