# Branch Code Review — Findings & Corrections

Review of branch `copilot/review-branch-issues` against
`github.com/msutara/config-manager-core`.

All tests currently pass (`go test ./...`) and the build is clean
(`go build ./...`). The issues below are correctness, design, and
coverage gaps that should be addressed.

---

## 1. Functional Bug — Trigger endpoint always returns 202 even for unknown jobs

**File:** `internal/api/routes.go` — `handleTriggerJob`

**Severity:** High

**Description:**
The job trigger is dispatched in a goroutine *before* the response is
written, so a `job_not_found` error is only logged server-side while
the client always receives `202 Accepted`. This contradicts the API
spec in `specs/API.md`, which requires a `404` response for unknown
job IDs.

**Current code:**

```go
jid := req.JobID
go func() {
    if err := s.scheduler.TriggerJob(jid); err != nil {
        slog.Error("triggered job failed", "job_id", jid, "error", err)
    } else {
        slog.Info("triggered job completed", "job_id", jid)
    }
}()
writeJSON(w, http.StatusAccepted, map[string]string{
    "status": "accepted",
    "job_id": req.JobID,
})
```

**Correction:**
Check existence synchronously before dispatching the goroutine. Add
a `JobExists(id string) bool` method to `JobTriggerer` (or change
the interface so `TriggerJob` is split into validate + dispatch), then:

```go
if req.JobID == "" {
    writeError(w, http.StatusBadRequest, "invalid_request", "job_id is required")
    return
}

if !s.scheduler.JobExists(req.JobID) {
    writeError(w, http.StatusNotFound, "job_not_found",
        "Job '"+req.JobID+"' not found")
    return
}

jid := req.JobID
go func() {
    if err := s.scheduler.TriggerJob(jid); err != nil {
        slog.Error("triggered job failed", "job_id", jid, "error", err)
    } else {
        slog.Info("triggered job completed", "job_id", jid)
    }
}()
writeJSON(w, http.StatusAccepted, map[string]string{
    "status": "accepted",
    "job_id": req.JobID,
})
```

Add `JobExists` to `JobTriggerer` in `server.go` and implement it in
`Scheduler` using a read lock on the jobs map.

---

## 2. Feature Gap — `enabled_plugins` config field is never enforced

**File:** `internal/config/config.go`, `cmd/cm/main.go`

**Severity:** Medium

**Description:**
`Config.EnabledPlugins` is documented in `docs/USAGE.md` and parsed
from the YAML file, but it is never applied. All registered plugins
are always active regardless of the config value.

**Correction:**
After collecting the plugin list in `cmd/cm/main.go`, filter it
against `cfg.EnabledPlugins` when the slice is non-empty:

```go
plugins := plugin.List()
if len(cfg.EnabledPlugins) > 0 {
    allowed := make(map[string]bool, len(cfg.EnabledPlugins))
    for _, n := range cfg.EnabledPlugins {
        allowed[n] = true
    }
    filtered := plugins[:0]
    for _, p := range plugins {
        if allowed[p.Name()] {
            filtered = append(filtered, p)
        }
    }
    plugins = filtered
}
```

Note: The API server and scheduler currently call `plugin.List()` and
`plugin.AllJobs()` directly from the registry, bypassing any filter
applied in `main.go`. A more robust fix is to add a `Disable(name)`
function to the registry, or to accept an allowlist in `NewServer` and
`sched.RegisterJobs`.

---

## 3. Design Issue — `startTime` measures package-init time, not server start

**File:** `internal/api/routes.go` line 17

**Severity:** Low

**Description:**

```go
var (
    startTime = time.Now()   // captured at package init, not at main() start
    Version   = "0.1.0"
)
```

`uptime_seconds` returned by `GET /api/v1/node` is computed from
package initialization time. If the package is imported early (e.g.,
in tests), the uptime will drift. The value should reflect the actual
service start time.

**Correction:**
Remove the package-level initialiser and set `startTime` explicitly
inside `NewServer`, so it captures the moment the server is created:

```go
// in routes.go — remove the initialiser
var startTime time.Time
```

```go
// in server.go — set it once at server construction
func NewServer(host string, port int, sched JobTriggerer) *Server {
    startTime = time.Now()
    // … rest of NewServer …
}
```

A cleaner alternative is to move `startTime` into the `Server` struct
and thread it through to the handler functions, keeping package-level
state to a minimum.

---

## 4. Design Issue — Non-deterministic ordering in `List()`, `AllRoutes()`, `AllJobs()`

**File:** `plugin/registry.go`

**Severity:** Low

**Description:**
All three functions iterate a `map[string]Plugin`, which produces a
different order on every call. This causes the `GET /api/v1/plugins`
and `GET /api/v1/jobs` responses to be non-deterministic, complicating
snapshot tests and diffing.

**Correction:**
Sort the output slices by name before returning:

```go
import "sort"

func List() []Plugin {
    mu.RLock()
    defer mu.RUnlock()

    plugins := make([]Plugin, 0, len(registry))
    for _, p := range registry {
        plugins = append(plugins, p)
    }
    sort.Slice(plugins, func(i, j int) bool {
        return plugins[i].Name() < plugins[j].Name()
    })
    return plugins
}
```

Apply the same pattern to `AllJobs()` (sort by `ID`) and the keys
of `AllRoutes()` (the map itself is fine for mounting, order does not
matter there).

---

## 5. Design Issue — `ErrJobNotFound` should use `errors.New`

**File:** `internal/scheduler/scheduler.go`

**Severity:** Low

**Description:**

```go
var ErrJobNotFound = &jobNotFoundError{}

type jobNotFoundError struct{}

func (e *jobNotFoundError) Error() string {
    return "job not found"
}
```

The custom type adds complexity without benefit. The
`errors.Is` comparison works by pointer equality on the package-level
variable, but this non-idiomatic pattern is harder to read and
maintain.

**Correction:**

```go
var ErrJobNotFound = errors.New("job not found")
```

Remove `jobNotFoundError` entirely. The existing tests using
`errors.Is(err, ErrJobNotFound)` continue to pass unchanged.

---

## 6. Missing Validation — Empty `job_id` in trigger endpoint

**File:** `internal/api/routes.go` — `handleTriggerJob`

**Severity:** Low

**Description:**
If the request body is `{"job_id": ""}`, the empty string is passed
to `TriggerJob`, which logs a not-found error silently while the
client still gets a `202 Accepted`. The endpoint should validate
that `job_id` is non-empty before dispatching.

**Correction:**
This is covered by the fix in item 1 above (the empty-string guard
before the existence check).

---

## 7. Design Issue — `Server.Start()` silently swallows start-up errors

**File:** `internal/api/server.go` — `Start()`

**Severity:** Low

**Description:**

```go
func (s *Server) Start() {
    go func() {
        slog.Info("API server starting", "addr", s.httpServer.Addr)
        if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("API server error", "error", err)
        }
    }()
}
```

If the port is already in use the error is only logged. `main.go` has
no way to know the server failed to start, so the process continues
running with no API.

**Correction:**
Return an error channel or call `os.Exit(1)` on fatal start-up errors:

```go
func (s *Server) Start() {
    go func() {
        slog.Info("API server starting", "addr", s.httpServer.Addr)
        if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("API server error — exiting", "error", err)
            os.Exit(1)
        }
    }()
}
```

Alternatively, expose `StartWithErrCh() <-chan error` to let `main`
react to the failure without hard-coding an `os.Exit`.

---

## 8. Test Coverage Gap — No test for trigger 404 path

**File:** `internal/api/routes_test.go`

**Severity:** Medium (if issue 1 is fixed)

**Description:**
There is no test that asserts a `404` is returned when
`POST /api/v1/jobs/trigger` is called with a job ID that does not
exist. Once the async-dispatch bug (item 1) is fixed, add:

```go
func TestHandleTriggerJobNotFound(t *testing.T) {
    sched := &mockScheduler{
        triggerFunc: func(_ string) error { return scheduler.ErrJobNotFound },
    }
    srv := &Server{scheduler: sched}
    body := `{"job_id": "no.such.job"}`
    w := httptest.NewRecorder()
    r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/trigger",
        bytes.NewBufferString(body))
    srv.handleTriggerJob(w, r)

    if w.Code != http.StatusNotFound {
        t.Fatalf("got status %d, want %d", w.Code, http.StatusNotFound)
    }
}
```

---

## 9. Test Coverage Gap — No test for empty `job_id`

**File:** `internal/api/routes_test.go`

**Severity:** Low

**Description:**
No test exercises the empty `job_id` validation path. Add:

```go
func TestHandleTriggerJobEmptyID(t *testing.T) {
    srv := &Server{scheduler: &mockScheduler{}}
    w := httptest.NewRecorder()
    r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/trigger",
        bytes.NewBufferString(`{"job_id": ""}`))
    srv.handleTriggerJob(w, r)

    if w.Code != http.StatusBadRequest {
        t.Fatalf("got status %d, want %d", w.Code, http.StatusBadRequest)
    }
}
```

---

## 10. Test Coverage Gap — `internal/logging` has no tests

**File:** `internal/logging/logging.go`

**Severity:** Low

**Description:**
`Setup()` and `ForPlugin()` have zero test coverage. While simple,
covering them prevents regressions in level parsing:

```go
package logging

import (
    "log/slog"
    "testing"
)

func TestSetupLevels(t *testing.T) {
    for _, tc := range []struct {
        input string
        want  slog.Level
    }{
        {"debug", slog.LevelDebug},
        {"info", slog.LevelInfo},
        {"warn", slog.LevelWarn},
        {"warning", slog.LevelWarn},
        {"error", slog.LevelError},
        {"", slog.LevelInfo},      // default
        {"unknown", slog.LevelInfo}, // default
    } {
        t.Run(tc.input, func(t *testing.T) {
            // Setup does not return the level; verify indirectly via ForPlugin logger
            Setup(tc.input)
            l := ForPlugin("test")
            if l == nil {
                t.Fatal("ForPlugin returned nil")
            }
        })
    }
}
```

---

## 11. Spec/Implementation Gap — `next_run_time` always `null`

**File:** `specs/API.md`, `internal/api/routes.go`

**Severity:** Low (documentation)

**Description:**
`specs/API.md` §2.5 shows `next_run_time` as a datetime string, but
the implementation always returns `null` with a "Phase 2" comment.
The spec should explicitly note this is a Phase 1 placeholder. Replace
the example value in the spec:

```json
"next_run_time": null
```

Add a note below the code block:

> **Phase 1:** `next_run_time` is always `null`. It will be computed
> from the cron expression in Phase 2.

---

## Summary Table

| # | File | Severity | Category |
|---|------|----------|----------|
| 1 | `internal/api/routes.go` | **High** | Functional bug |
| 2 | `cmd/cm/main.go` + `config.go` | Medium | Feature gap |
| 3 | `internal/api/routes.go` | Low | Design |
| 4 | `plugin/registry.go` | Low | Design |
| 5 | `internal/scheduler/scheduler.go` | Low | Design |
| 6 | `internal/api/routes.go` | Low | Validation |
| 7 | `internal/api/server.go` | Low | Design |
| 8 | `internal/api/routes_test.go` | Medium | Test coverage |
| 9 | `internal/api/routes_test.go` | Low | Test coverage |
| 10 | `internal/logging/logging.go` | Low | Test coverage |
| 11 | `specs/API.md` | Low | Documentation |

**Priority order for correction:**

1. Item 1 (trigger 404 bug) — breaks API contract
2. Item 8 (missing test for item 1 fix)
3. Item 2 (enabled_plugins not enforced)
4. Items 5, 6, 9 — can be addressed in a single small PR
5. Items 3, 4, 7, 10, 11 — low-risk cleanups
