# Development Guide

This guide covers local development, integration testing, and release
procedures for the Config Manager multi-repo project.

## Repository Layout

Config Manager is split across five repositories that compile into a
single binary:

| Repository | Module Path | Role |
| --- | --- | --- |
| [config-manager-core](https://github.com/msutara/config-manager-core) | `github.com/msutara/config-manager-core` | Core framework, plugin system, API server, scheduler, CLI |
| [config-manager-tui](https://github.com/msutara/config-manager-tui) | `github.com/msutara/config-manager-tui` | Terminal UI (Bubble Tea, raspi-config style) |
| [config-manager-web](https://github.com/msutara/config-manager-web) | `github.com/msutara/config-manager-web` | Browser dashboard (htmx, Go templates) |
| [cm-plugin-update](https://github.com/msutara/cm-plugin-update) | `github.com/msutara/cm-plugin-update` | System update management (apt) |
| [cm-plugin-network](https://github.com/msutara/cm-plugin-network) | `github.com/msutara/cm-plugin-network` | Network configuration (static IP, DNS, rollback) |

**Dependency graph:**

```txt
config-manager-core
├── config-manager-tui    (UI layer)
├── config-manager-web    (UI layer)
├── cm-plugin-update      (plugin)
└── cm-plugin-network     (plugin)
```

Core imports all four as Go modules. Plugins and UI layers depend only
on core's `plugin` package for the interface contract.

## Prerequisites

- **Go** 1.24+ (`go version`)
- **golangci-lint** v2.1+ (`golangci-lint version`)
- **nfpm** (for `.deb` packaging only — `go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest`)
- **Git** with access to all five repositories

## Setting Up Local Development

### Step 1: Clone All Repositories

Clone all five repos into a common parent directory:

```bash
mkdir -p ~/repo && cd ~/repo
git clone https://github.com/msutara/config-manager-core.git
git clone https://github.com/msutara/config-manager-tui.git
git clone https://github.com/msutara/config-manager-web.git
git clone https://github.com/msutara/cm-plugin-update.git
git clone https://github.com/msutara/cm-plugin-network.git
```

### Step 2: Create a Go Workspace

Create a `go.work` file in the parent directory to link all modules
for local development:

```bash
cd ~/repo
go work init ./config-manager-core ./config-manager-tui \
  ./config-manager-web ./cm-plugin-update ./cm-plugin-network
```

This produces a `go.work` file:

```go
go 1.24.0

use (
    ./config-manager-core
    ./config-manager-tui
    ./config-manager-web
    ./cm-plugin-update
    ./cm-plugin-network
)
```

**Important:** `go.work` is gitignored in all repos. It is a local
development tool — never commit it.

### Step 3: Verify the Workspace

```bash
cd ~/repo
go build ./config-manager-core/cmd/cm
```

If this succeeds, the workspace is correctly configured. Changes to
any module are immediately reflected when building from the workspace
root.

## Development Workflow

### Working on a Single Repository

If your change is isolated to one repo (e.g., fixing a bug in
`cm-plugin-network`):

```bash
cd ~/repo/cm-plugin-network
# make changes
go test ./...
golangci-lint run
# commit and push via PR
```

The repo's own CI validates lint + tests independently.

### Working Across Repositories

If your change spans multiple repos (e.g., adding a new plugin
interface in core and implementing it in a plugin):

```bash
cd ~/repo

# Edit core's plugin interface
vim config-manager-core/plugin/plugin.go

# Implement in a plugin
vim cm-plugin-network/service.go

# Build and test from workspace root — uses local code, no push needed
go build ./config-manager-core/cmd/cm
go test ./config-manager-core/... ./cm-plugin-network/...
```

The `go.work` file ensures Go resolves all `github.com/msutara/*`
imports to your local directories instead of fetching from the module
proxy.

### Testing Without the Workspace

Before releasing, verify that each module builds independently
(without `go.work` overrides):

```bash
cd ~/repo/config-manager-core
GOWORK=off go build ./cmd/cm
GOWORK=off go test ./...
```

`GOWORK=off` disables workspace resolution and forces Go to use
`go.mod` versions. This simulates what CI and end users see.

## Adding a New Plugin

### Step 1: Create the Repository

```bash
# On GitHub: create msutara/cm-plugin-<name>

cd ~/repo
git clone https://github.com/msutara/cm-plugin-<name>.git
cd cm-plugin-<name>
go mod init github.com/msutara/cm-plugin-<name>
```

### Step 2: Implement the Plugin Interface

Your plugin must implement `plugin.Plugin` from core:

```go
package myplugin

import (
    "net/http"
    "github.com/msutara/config-manager-core/plugin"
)

type MyPlugin struct{}

func NewMyPlugin() *MyPlugin { return &MyPlugin{} }

func (p *MyPlugin) Name() string        { return "myplugin" }
func (p *MyPlugin) Version() string     { return "0.1.0" }
func (p *MyPlugin) Description() string { return "My new plugin" }

func (p *MyPlugin) Routes() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /status", p.handleStatus)
    return mux
}

func (p *MyPlugin) ScheduledJobs() []plugin.JobDefinition {
    return nil // or define cron jobs
}

func (p *MyPlugin) Endpoints() []plugin.Endpoint {
    return []plugin.Endpoint{
        {Method: "GET", Path: "/status", Description: "Plugin status"},
    }
}
```

Optional interfaces:

- `plugin.Configurable` — receive runtime config from
  `/api/v1/plugins/{name}/settings`

### Step 3: Register in Core

Add the import and registration call in `cmd/cm/main.go`:

```go
import myplugin "github.com/msutara/cm-plugin-<name>"

// In main():
plugin.Register(myplugin.NewMyPlugin())
```

### Step 4: Add to Workspace

```bash
cd ~/repo
go work use ./cm-plugin-<name>
```

### Step 5: Add UI Support

Both TUI and Web auto-discover plugins via the `/api/v1/plugins`
endpoint. For basic read-only display, no UI changes are needed.

For custom UI (write operations, special forms):

- **TUI:** Add menu entries in `config-manager-tui/menu.go`
- **Web:** Add route handlers in `config-manager-web/routes.go` and
  templates in `templates/`

## Integration Testing

### Smoke Test Script

The `scripts/integration_test.sh` script performs end-to-end
validation:

```bash
cd ~/repo/config-manager-core
./scripts/integration_test.sh
```

What it does:

1. Builds the binary from the current source (respects `go.work`)
2. Creates a temporary config with a test auth token
3. Starts the server in headless mode on a random port
4. Verifies all core endpoints respond (`/health`, `/node`,
   `/plugins`, `/jobs`)
5. Verifies each plugin is registered and its routes respond
6. Verifies the web UI serves the dashboard
7. Tears down the server and cleans up

### Running Without Workspace (CI Mode)

```bash
GOWORK=off ./scripts/integration_test.sh
```

This builds using `go.mod` versions only — catches cases where a
local change works in the workspace but the published module is
outdated.

### Manual Testing on Device

For testing on a real ARM device:

```bash
# Build for your target
cd ~/repo/config-manager-core
GOOS=linux GOARCH=arm GOARM=7 go build -o cm-armv7 ./cmd/cm

# Copy to device
scp cm-armv7 user@device:/tmp/cm

# On device: stop existing service, test new binary
sudo systemctl stop cm
sudo /tmp/cm --config /etc/cm/config.yaml --headless &
# verify, then Ctrl-C and restart service
sudo systemctl start cm
```

Or build a `.deb` for clean upgrade testing:

```bash
make deb GOARCH=arm GOARM=7
scp build/cm_*_armhf.deb user@device:/tmp/
# On device:
sudo dpkg -i /tmp/cm_*_armhf.deb
```

## Release Process

### Step 1: Validate All Repos

```bash
cd ~/repo
for d in cm-plugin-update cm-plugin-network config-manager-tui config-manager-web config-manager-core; do
    echo "=== $d ==="
    cd ~/repo/$d
    golangci-lint run && go test ./... && echo "PASS" || echo "FAIL"
done
```

### Step 2: Tag Plugin Repos First

Plugins have no cross-dependencies — tag them first:

```bash
for repo in cm-plugin-update cm-plugin-network config-manager-tui config-manager-web; do
    cd ~/repo/$repo
    git tag -a v0.X.Y -m "v0.X.Y — description"
    git push origin v0.X.Y
done
```

### Step 3: Update Core Dependencies

```bash
cd ~/repo/config-manager-core

# Update go.mod to new plugin tags
go get github.com/msutara/cm-plugin-update@v0.X.Y
go get github.com/msutara/cm-plugin-network@v0.X.Y
go get github.com/msutara/config-manager-tui@v0.X.Y
go get github.com/msutara/config-manager-web@v0.X.Y
go mod tidy
```

### Step 4: Integration Test (Without Workspace)

```bash
GOWORK=off ./scripts/integration_test.sh
```

### Step 5: Tag and Release Core

```bash
cd ~/repo/config-manager-core
git add go.mod go.sum
git commit -m "release: bump dependencies to v0.X.Y"
git tag -a v0.X.Y -m "v0.X.Y — description"
git push origin main v0.X.Y
```

### Step 6: Build and Upload Artifacts

```bash
make build-all
make deb-all
gh release create v0.X.Y build/cm-* build/cm_*.deb \
    --title "v0.X.Y" --notes-file RELEASE_NOTES.md
```

### Step 7: Create GitHub Releases for Plugins

```bash
for repo in cm-plugin-update cm-plugin-network config-manager-tui config-manager-web; do
    gh release create v0.X.Y --repo msutara/$repo \
        --title "v0.X.Y" --notes "Release notes here"
done
```

## CI Configuration

Each repository runs its own CI on push to `main` and on PRs:

| Check | Tool | Purpose |
| --- | --- | --- |
| Lint | golangci-lint v2.1.6 (action v9) | Code quality, errcheck, staticcheck |
| Test | `go test ./...` | Unit tests |
| Markdown | markdownlint-cli2 | Documentation quality |

**CI validates each repo independently.** Cross-repo integration is
validated locally via `go.work` and the integration test script.

## Code Style

- **Linter:** golangci-lint v2 with errcheck, govet, ineffassign,
  staticcheck, unused, gofumpt
- **Formatting:** gofumpt (stricter than gofmt)
- **Commits:** Conventional Commits (`feat:`, `fix:`, `docs:`,
  `chore:`)
- **PRs:** Squash-merge to `main`, branch protection requires CI pass
  and Copilot auto-review
- **Line endings:** LF only (no CRLF)

## Troubleshooting

### `go.work` Not Found

If Go ignores your workspace, check that you're running commands from
the workspace root (the directory containing `go.work`), or set
`GOWORK` explicitly:

```bash
export GOWORK=~/repo/go.work
```

### Module Proxy Cache

After making repos public, the Go module proxy may serve stale
checksums. Fix by regenerating `go.sum`:

```bash
rm go.sum
go mod tidy
```

### golangci-lint v1 vs v2

The `.golangci.yml` files use v2 format (`version: "2"`). If you have
golangci-lint v1 installed, it will reject the config. Install v2:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6
```
