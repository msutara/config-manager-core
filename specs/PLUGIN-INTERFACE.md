# Config Manager Core — Plugin Interface

## 1. Overview

Plugins extend CM Core with domain-specific functionality (e.g., updates,
networking). They are separate Go modules that:

- Implement a common `Plugin` interface.
- Are imported at build time into the core binary.
- Self-register via Go `init()` functions.
- Provide HTTP handlers for API routes.
- Optionally define scheduled jobs.

---

## 2. Build-time registration

In a plugin's Go module, a file (conventionally `register.go` or within
`plugin.go`) calls the core registry during `init()`:

```go
package update

import (
    "github.com/msutara/config-manager-core/plugin"
)

func init() {
    plugin.Register(&UpdatePlugin{})
}
```

In the core binary's `cmd/cm/main.go`, plugins are imported with a blank
identifier:

```go
import (
    _ "github.com/msutara/cm-plugin-update"
    _ "github.com/msutara/cm-plugin-network"
)
```

This triggers each plugin's `init()` function, registering it with the core.

---

## 3. Plugin interface

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
}
```

---

## 4. Job definitions

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
            ID:          "update.run_security",
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

## 5. Route mounting

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

## 6. Plugin metadata

The `Name()`, `Version()`, and `Description()` methods provide metadata
exposed via:

- `GET /api/v1/plugins` — list all plugins.
- `GET /api/v1/plugins/{name}` — get one plugin's metadata.
- TUI menu generation — each plugin appears as a menu item.

---

## 7. Configuration

Plugin-specific configuration is managed by the plugin itself.
Recommended pattern:

- Plugin reads its config from a plugin-specific YAML section in the
  shared config file, or its own file under
  `/etc/cm/plugins/{plugin_name}.yaml`.
- The core does not enforce a specific plugin config mechanism but
  may provide helpers later.

---

## 8. TUI integration

Plugins may optionally implement a `TUIModel` interface (future) to provide
custom Bubble Tea views. For Phase 1, the TUI will show plugin metadata
and provide action triggers based on routes.

---

## 9. Versioning

Plugins should:

- Expose a version string via `Version()`.
- Follow semantic versioning where possible.
- Be able to report their version via metadata and their own endpoints.

---

## 10. Creating a new plugin

1. Create a new Go module repo (e.g., `cm-plugin-foo`).
2. Implement the `Plugin` interface.
3. Call `plugin.Register()` in an `init()` function.
4. In `config-manager-core/cmd/cm/main.go`, add:
   `import _ "github.com/msutara/cm-plugin-foo"`
5. Run `go mod tidy` and rebuild.
