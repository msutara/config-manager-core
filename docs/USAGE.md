# Usage Guide

## 1. CLI Options

```text
Usage: cm [OPTIONS]

Options:
  --config PATH   Path to config file (default: /etc/cm/config.yaml)
  --version       Show version and exit
  -help, --help   Show help message
```

## 2. Configuration

CM Core reads configuration from a YAML file. If not found, defaults are used.

Default path: `/etc/cm/config.yaml`

```yaml
listen_host: localhost
listen_port: 8080
log_level: info
enabled_plugins: []  # empty = all enabled
```

### Environment Variable Overrides

Environment variables override YAML values. Set any combination:

| Variable | Description | Example |
|----------|-------------|---------|
| `CM_LISTEN_HOST` | Bind address | `0.0.0.0` |
| `CM_LISTEN_PORT` | Listen port | `9090` |
| `CM_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `debug` |
| `CM_ENABLED_PLUGINS` | Comma-separated plugin allowlist | `update,network` |

```bash
# Override port and log level
CM_LISTEN_PORT=9090 CM_LOG_LEVEL=debug ./cm

# Restrict to specific plugins
CM_ENABLED_PLUGINS=update ./cm
```

Invalid `CM_LISTEN_PORT` values are ignored with a warning; the YAML or
default value is kept.

## 3. Building

```bash
# Native build
go build -o cm ./cmd/cm

# Cross-compile for Raspbian (ARM64)
GOOS=linux GOARCH=arm64 go build -o cm ./cmd/cm

# Cross-compile for Debian Bullseye (ARMv7)
GOOS=linux GOARCH=arm GOARM=7 go build -o cm ./cmd/cm
```

## 4. Running

```bash
# Run with defaults
./cm

# Run with custom config
./cm --config /path/to/config.yaml

# Check version
./cm --version
```

## 5. Deployment

Copy the binary to your node and run as a systemd service:

```bash
scp cm user@node:/usr/local/bin/cm
ssh user@node 'sudo systemctl enable --now cm'
```

> systemd service file will be provided in a future release.
