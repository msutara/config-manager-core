# Config Manager Core

Core service for managing headless Debian-based nodes.
Provides a plugin system, TUI interface (raspi-config style), REST API,
and job scheduler — all in a single binary.

## Features

- **Plugin architecture** — modular, separate repos compiled into one binary
- **TUI interface** — interactive menus via Bubble Tea (raspi-config style)
- **Headless mode** — `--headless` for systemd/daemon use (API server only)
- **REST API** — embedded HTTP server for remote access and future web UI
- **Job scheduler** — cron-based recurring tasks defined by plugins
- **Single binary** — cross-compile for ARM, no runtime dependencies
- **.deb packaging** — install with `dpkg`, systemd service included

## Installation

### Quick install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/msutara/config-manager-core/main/scripts/install.sh | sudo bash
```

Or with a specific version/architecture:

```bash
curl -fsSL https://raw.githubusercontent.com/msutara/config-manager-core/main/scripts/install.sh | sudo bash -s -- --version 0.2.0 --arch armhf
```

### From .deb package

Download the latest release for your architecture from
[GitHub Releases](https://github.com/msutara/config-manager-core/releases):

```bash
# Raspberry Pi 2 / UCK Gen1 (ARMv7)
sudo dpkg -i cm_<version>_armhf.deb

# Raspberry Pi 4+ (ARM64)
sudo dpkg -i cm_<version>_arm64.deb

# x86_64
sudo dpkg -i cm_<version>_amd64.deb
```

The package installs a systemd service and sets up directories. Start with:

```bash
sudo systemctl start cm
```

### From source

```bash
# Native build
make build

# Cross-compile all targets (amd64, arm64, armhf)
make build-all

# Build .deb packages for all architectures
make deb-all
```

## Quick Start

```bash
./cm                              # run with defaults
./cm --config /path/to/config.yaml  # custom config
./cm --version                    # show version
```

## Documentation

- [Usage Guide](docs/USAGE.md) — CLI options, configuration, and deployment
- [How It Works](docs/HOW-IT-WORKS.md) — architecture and plugin model

## Specifications

- [SPEC.md](specs/SPEC.md) — core specification
- [ARCHITECTURE.md](specs/ARCHITECTURE.md) — internal structure
- [PLUGIN-INTERFACE.md](specs/PLUGIN-INTERFACE.md) — plugin contract
- [API.md](specs/API.md) — REST API specification

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Security

See [SECURITY.md](SECURITY.md) for vulnerability reporting.

## License

See [LICENSE](LICENSE) for details.
