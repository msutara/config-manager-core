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
make build

# Cross-compile all targets (amd64, arm64, armhf)
make build-all

# Build .deb packages
make deb-all
```

Or without Make:

```bash
# Cross-compile for Raspberry Pi 2 / UCK Gen1 (ARMv7)
GOOS=linux GOARCH=arm GOARM=7 go build -o cm ./cmd/cm

# Cross-compile for Raspberry Pi 4+ (ARM64)
GOOS=linux GOARCH=arm64 go build -o cm ./cmd/cm
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

### Via .deb package (recommended)

```bash
# Install
sudo dpkg -i cm_0.1.0_armhf.deb

# Manage service
sudo systemctl start cm
sudo systemctl status cm
sudo systemctl stop cm

# View logs
journalctl -u cm -f              # systemd journal
cat /var/log/cm/cm.log            # application log

# Upgrade
sudo dpkg -i cm_0.2.0_armhf.deb
sudo systemctl restart cm

# Remove (keeps config in /etc/cm/)
sudo dpkg -r cm

# Remove everything including config
sudo dpkg --purge cm
```

### Installed filesystem layout

```text
/usr/bin/cm                     # binary
/etc/cm/config.yaml             # config (preserved on upgrade)
/lib/systemd/system/cm.service  # systemd service unit
/var/log/cm/cm.log              # application log
/var/lib/cm/                    # working directory
```

### Manual deployment

```bash
scp cm user@node:/usr/local/bin/cm
ssh user@node 'sudo systemctl enable --now cm'
```
