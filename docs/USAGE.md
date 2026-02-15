# Usage Guide

## CLI Options

```text
Usage: cm [OPTIONS]

Options:
  --config PATH   Path to config file (default: /etc/cm/config.yaml)
  --version       Show version and exit
  -h, --help      Show help message
```

## Configuration

CM Core reads configuration from a YAML file. If not found, defaults are used.

Default path: `/etc/cm/config.yaml`

```yaml
listen_host: localhost
listen_port: 8080
log_level: info
enabled_plugins: []  # empty = all enabled
```

## Building

```bash
# Native build
go build -o cm ./cmd/cm

# Cross-compile for Raspbian (ARM64)
GOOS=linux GOARCH=arm64 go build -o cm ./cmd/cm

# Cross-compile for Debian Bullseye (ARMv7)
GOOS=linux GOARCH=arm GOARM=7 go build -o cm ./cmd/cm
```

## Running

```bash
# Run with defaults
./cm

# Run with custom config
./cm --config /path/to/config.yaml

# Check version
./cm --version
```

## Deployment

Copy the binary to your node and run as a systemd service:

```bash
scp cm user@node:/usr/local/bin/cm
ssh user@node 'sudo systemctl enable --now cm'
```

> systemd service file will be provided in a future release.
