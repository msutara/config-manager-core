# Usage Guide

## 1. CLI Options

```text
Usage: cm [OPTIONS]

Options:
  --config PATH      Path to config file (default: /etc/cm/config.yaml)
  --headless         Run without TUI (API server only, for systemd)
  --connect URL      Connect TUI to running CM service (skip local server)
  --rotate-token     Generate a new auth token and exit
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
theme: ""            # built-in name or absolute path to YAML theme file
plugins:
  update:
    schedule: "0 2 * * 1"
    auto_security: true
```

The `plugins` section holds per-plugin configuration. Each key matches a plugin name.

### Environment Variable Overrides

Environment variables override YAML values. Set any combination:

| Variable | Description | Example |
|----------|-------------|---------|
| `CM_LISTEN_HOST` | Bind address | `0.0.0.0` |
| `CM_LISTEN_PORT` | Listen port | `9090` |
| `CM_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `debug` |
| `CM_ENABLED_PLUGINS` | Comma-separated plugin allowlist | `update,network` |
| `CM_THEME` | TUI theme name or path | `nord` |

```bash
# Override port and log level
CM_LISTEN_PORT=9090 CM_LOG_LEVEL=debug ./cm

# Restrict to specific plugins
CM_ENABLED_PLUGINS=update ./cm
```

Invalid `CM_LISTEN_PORT` values are ignored with a warning; the YAML or
default value is kept.

## 3. Authentication

CM uses Bearer token authentication for API access. A random token is
generated during package installation at `/etc/cm/auth.token` (mode 0600).

All API endpoints require a valid token **except** `/api/v1/health`, which
remains public for auto-detection probes.

### How it works

- The service reads the token file on startup. If the file exists but is
  unreadable or empty, the service refuses to start (fail-closed).
- The TUI reads the same file and attaches the token to every request
  automatically — no user interaction needed. Run with `sudo cm` since
  the token file is root-readable only.
- External clients must include the header:

```bash
curl -H "Authorization: Bearer $(cat /etc/cm/auth.token)" \
     http://localhost:7788/api/v1/node
```

### Rotating the token

```bash
# Generate a new token and restart the service
sudo cm --rotate-token
sudo systemctl restart cm
```

### Disabling auth

If no token file exists, authentication is disabled and all endpoints are
open. Remove the file and restart:

```bash
sudo rm /etc/cm/auth.token
sudo systemctl restart cm
```

## 4. Building

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

## 5. Running

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

## 6. API Endpoints

Core exposes these REST endpoints (when auth is enabled via `/etc/cm/auth.token`, all require a Bearer token except health):

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/v1/health` | Health check (public, no auth) |
| `GET` | `/api/v1/node` | Node information (hostname, uptime, OS) |
| `GET` | `/api/v1/plugins` | List registered plugins |
| `GET` | `/api/v1/plugins/{name}` | Plugin metadata |
| `GET` | `/api/v1/plugins/{name}/settings` | Plugin configuration |
| `PUT` | `/api/v1/plugins/{name}/settings` | Update plugin configuration |
| `GET` | `/api/v1/jobs` | List all scheduled jobs |
| `POST` | `/api/v1/jobs/trigger` | Trigger a job by ID |
| `GET` | `/api/v1/jobs/{id}/runs/latest` | Latest run status for a job |

Plugin routes are mounted under `/api/v1/plugins/{name}/...`.

## 7. Deployment

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

To completely remove CM including all data, logs, and systemd overrides:

```bash
sudo systemctl stop cm.service
sudo dpkg --purge cm
sudo rm -rf /etc/cm /var/log/cm /var/lib/cm /etc/systemd/system/cm.service.d
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
