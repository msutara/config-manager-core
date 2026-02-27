# Config Manager Core — Plugin Interface

## 1. Overview

Plugins extend CM Core with domain-specific functionality (e.g., updates,
networking). They are separate Go modules that:

- Implement a common `Plugin` interface.
- Are imported at build time into the core binary.
- Export a constructor (e.g., `NewUpdatePlugin()`) for explicit registration.
- Provide HTTP handlers for API routes.
- Optionally define scheduled jobs.

In the plugin repo (e.g., `cm-plugin-update/plugin.go`), export a constructor:

```go
func NewUpdatePlugin() *UpdatePlugin {
    return &UpdatePlugin{}
}
```

In the core binary's `cmd/cm/main.go`, import the plugin and register it
explicitly:

```go
import update "github.com/msutara/cm-plugin-update"

plugin.Register(update.NewUpdatePlugin())
```

---

## 2. Plugin Interface

Defined in `plugin/plugin.go`:

```go
type Plugin interface {
    // Name returns the unique plugin identifier (e.g., "update", "network").
    Name() string

    // Version returns the plugin version string (semver recommended).
    Version() string

    // Description returns a human-readable description.
    Description() string

    // Routes returns an http.Handler to be mounted under
    // /api/v1/plugins/{Name()}.
    Routes() http.Handler

    // ScheduledJobs returns a list of job definitions for the scheduler.
    // Return nil or empty slice if no scheduled jobs.
    ScheduledJobs() []JobDefinition

    // Endpoints returns the HTTP endpoints this plugin exposes.
    // UIs use this to build generic pages/menus for plugins without
    // a custom template. Return nil or empty if not applicable.
    Endpoints() []Endpoint
}
```

---

## 3. Job Definitions

Plugins can define scheduled jobs via `ScheduledJobs()`:

```go
type JobDefinition struct {
    // ID is globally unique: "{plugin_name}.{job_name}"
    ID          string
    Description string
    Cron        string // cron expression, e.g. "0 3 * * *"
    Func        func() error
}
```

Example:

```go
func (p *UpdatePlugin) ScheduledJobs() []JobDefinition {
    return []JobDefinition{
        {
            ID:          "update.security",
            Description: "Run security updates",
            Cron:        "0 3 * * *",
            Func:        p.RunSecurityUpdates,
        },
    }
}
```

The core scheduler will:

- Register these jobs.
- Expose them via `GET /api/v1/jobs`.
- Allow triggering via `POST /api/v1/jobs/trigger`.

---

## 3a. Endpoint Declarations

Plugins declare their HTTP endpoints via `Endpoints()`:

```go
type Endpoint struct {
    Method      string // "GET" or "POST"
    Path        string // relative to plugin mount, e.g. "/status"
    Description string // human-readable label for UI display
}
```

Example:

```go
func (p *UpdatePlugin) Endpoints() []Endpoint {
    return []Endpoint{
        {Method: "GET", Path: "/status", Description: "Pending updates"},
        {Method: "GET", Path: "/config", Description: "Update configuration"},
        {Method: "POST", Path: "/run", Description: "Trigger update"},
    }
}
```

The web UI and TUI use endpoint declarations to build generic plugin pages
and menus. Plugins with a custom UI template get a richer experience;
plugins without one get a functional page automatically.

---

## 4. Route Mounting

CM Core will:

1. Iterate registered plugins.
2. Call `Routes()` on each.
3. Mount the returned handler under:

```text
/api/v1/plugins/{plugin_name}/<handler_paths>
```

Example:

- Plugin name: `update`
- Handler path: `/status`
- Final URL: `/api/v1/plugins/update/status`

---

## 5. Configuration

Plugin-specific configuration is managed by the plugin itself.
Recommended pattern:

- Plugin reads its config from a plugin-specific YAML section in the
  shared config file, or its own file under
  `/etc/cm/plugins/{plugin_name}.yaml`.
- The core does not enforce a specific plugin config mechanism but
  may provide helpers later.

---

## 6. Creating a Plugin

1. Create a new Go module repo (e.g., `cm-plugin-foo`).
2. Implement the `Plugin` interface (including `Endpoints()`).
3. Export a constructor function (e.g., `NewFooPlugin()`).
4. Add import and `plugin.Register()` call to `cmd/cm/main.go`.
5. Run `go mod tidy` and rebuild.
6. The web UI sidebar and TUI menu discover the plugin automatically.
7. Optionally add a custom web template or TUI handler for richer UX.
