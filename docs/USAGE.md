# Usage Guide

## 1. CLI Options

```text
Usage: cm [OPTIONS]

Options:
  --config PATH      Path to config file (default: /etc/cm/config.yaml)
  --headless         Run without TUI (API server only, for systemd)
  --connect URL      Connect TUI to running CM service (skip local server)
  --version          Show version and exit
  -help, --help      Show help message
```

## 2. Configuration

CM Core reads configuration from a YAML file. If not found, defaults are used.

Default path: `/etc/cm/config.yaml`

```yaml
listen_host: localhost
listen_port: 7788
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
# Run with defaults (standalone — starts own API server + TUI)
./cm

# Run with custom config
./cm --config /path/to/config.yaml

# Run headless (API server only, for systemd service)
./cm --headless

# Connect TUI to a running headless service
./cm --connect http://localhost:7788

# Check version
./cm --version
```

### Service + Standalone Mode

CM auto-detects whether a headless service is already running:

- **Standalone**: No service detected → TUI starts its own embedded API server
  (default behavior).
- **Connected**: Service detected at configured port → TUI connects to it, no
  duplicate server started. The footer shows a green `● connected` badge.

Use `--connect URL` to override auto-detection and force client mode with an
explicit service URL. Both modes can run side by side without crashing either.

## 5. Deployment

### Quick install

```bash
curl -fsSL https://raw.githubusercontent.com/msutara/config-manager-core/main/scripts/install.sh | sudo bash
```

The script auto-detects your architecture, downloads the latest `.deb`, and
installs it. Override with `bash -s --`:

```bash
curl -fsSL https://raw.githubusercontent.com/msutara/config-manager-core/main/scripts/install.sh | sudo bash -s -- --version 0.2.0 --arch armhf
```

### Via .deb package

```bash
# Install
sudo dpkg -i cm_<version>_armhf.deb

# Manage service
sudo systemctl start cm
sudo systemctl status cm
sudo systemctl stop cm

# View logs
journalctl -u cm -f              # systemd journal
cat /var/log/cm/cm.log            # application log

# Upgrade
sudo dpkg -i cm_<version>_armhf.deb

# Remove (keeps config in /etc/cm/)
sudo dpkg -r cm

# Remove everything including config
sudo dpkg --purge cm
```

### Full uninstall

To completely remove CM including all data, logs, user, and systemd overrides:

```bash
sudo systemctl stop cm.service
sudo dpkg --purge cm
sudo rm -rf /etc/cm /var/log/cm /var/lib/cm /etc/systemd/system/cm.service.d
sudo userdel cm 2>/dev/null
sudo groupdel cm 2>/dev/null
sudo systemctl daemon-reload
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
# Copy binary and service file
scp cm packaging/cm.service user@node:/tmp/

# On the node: install binary, service, and start
ssh user@node 'sudo mv /tmp/cm /usr/bin/cm && sudo chmod 755 /usr/bin/cm && \
  sudo mv /tmp/cm.service /lib/systemd/system/cm.service && \
  sudo systemctl daemon-reload && sudo systemctl enable --now cm'
```
