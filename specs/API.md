# Config Manager Core — API Specification

## 1. Overview

All responses are JSON. Errors use a common error format.

---

## 2. Base URL

Base path: `/api/v1`

---

## 3. Error Format

On error, endpoints return:

```json
{
  "error": {
    "code": "string_identifier",
    "message": "Human-readable message",
    "details": {}
  }
}
```

---

## 4. Core Endpoints

### 4.1. `GET /api/v1/health`

Health check for the core service.

**Response 200:**

```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

---

### 4.2. `GET /api/v1/node`

Basic node/system information.

**Response 200:**

```json
{
  "hostname": "node-01",
  "os": "Debian GNU/Linux 12 (bookworm)",
  "kernel": "6.1.0-xyz",
  "uptime_seconds": 123456,
  "arch": "arm64"
}
```

---

### 4.3. `GET /api/v1/plugins`

List loaded plugins.

**Response 200:**

```json
[
  {
    "name": "update",
    "version": "0.1.0",
    "description": "System updates management",
    "route_prefix": "/api/v1/plugins/update",
    "endpoints": [
      {"method": "GET", "path": "/status", "description": "Pending updates and system info"},
      {"method": "GET", "path": "/logs", "description": "Last update run output"},
      {"method": "GET", "path": "/config", "description": "Update plugin configuration"},
      {"method": "POST", "path": "/run", "description": "Trigger update run"}
    ]
  },
  {
    "name": "network",
    "version": "0.1.0",
    "description": "Network configuration",
    "route_prefix": "/api/v1/plugins/network",
    "endpoints": [
      {"method": "GET", "path": "/interfaces", "description": "Network interface details"},
      {"method": "GET", "path": "/status", "description": "Connectivity and reachability status"},
      {"method": "GET", "path": "/dns", "description": "DNS configuration"}
    ]
  }
]
```

---

### 4.4. `GET /api/v1/plugins/{name}`

Get metadata for a specific plugin.

**Path params:**

- `name`: plugin name.

**Response 200:**

```json
{
  "name": "update",
  "version": "0.1.0",
  "description": "System updates management",
  "route_prefix": "/api/v1/plugins/update",
  "endpoints": [
    {"method": "GET", "path": "/status", "description": "Pending updates and system info"},
    {"method": "GET", "path": "/logs", "description": "Last update run output"},
    {"method": "GET", "path": "/config", "description": "Update plugin configuration"},
    {"method": "POST", "path": "/run", "description": "Trigger update run"}
  ]
}
```

**Response 404:**

```json
{
  "error": {
    "code": "plugin_not_found",
    "message": "Plugin 'foo' not found",
    "details": {}
  }
}
```

---

### 4.5. `GET /api/v1/plugins/{name}/settings`

Get a plugin's configurable settings. Only plugins implementing the
`Configurable` interface support this endpoint.

**Note:** The `/settings` endpoint is a core-managed route that is mounted
alongside any plugin-provided routes such as `/config`. The core wraps each
plugin's handler so `GET /api/v1/plugins/{name}/settings` remains reachable
even though the plugin itself is mounted at `/api/v1/plugins/{name}` and may
define its own sub-routes. Configuration exposed via `/settings` is distinct
from any plugin-specific APIs.

**Path params:**

- `name`: plugin name.

**Response 200:**

```json
{
  "config": {
    "schedule": "0 3 * * *",
    "auto_security": true,
    "security_source": "detected"
  }
}
```

**Response 404** (plugin not found):

```json
{
  "error": {
    "code": "plugin_not_found",
    "message": "Plugin 'foo' not found",
    "details": {}
  }
}
```

**Response 501** (plugin not configurable):

```json
{
  "error": {
    "code": "not_configurable",
    "message": "Plugin 'network' does not support configuration",
    "details": {}
  }
}
```

---

### 4.6. `PUT /api/v1/plugins/{name}/settings`

Update a single setting for a plugin. When a ConfigProvider is configured,
the change is persisted via the provider (for example, writing to disk) and
hot-reloaded in memory. Without a ConfigProvider, the change only affects
in-memory state for the current process. If the key is `schedule`, the
scheduler job `{name}.security` is rescheduled using the new value.

**Path params:**

- `name`: plugin name.

**Request body:**

```json
{"key": "schedule", "value": "0 4 * * *"}
```

**Response 200:**

```json
{
  "config": {"schedule": "0 4 * * *", "auto_security": true, "security_source": "detected"}
}
```

The `warning` field is included only when non-empty (e.g. scheduler update failed).

**Response 400** (invalid JSON body):

```json
{
  "error": {
    "code": "invalid_request",
    "message": "Invalid JSON body",
    "details": {}
  }
}
```

**Response 400** (trailing data):

```json
{
  "error": {
    "code": "invalid_request",
    "message": "Request body must contain exactly one JSON object",
    "details": {}
  }
}
```

**Response 400** (missing key):

```json
{
  "error": {
    "code": "invalid_request",
    "message": "key is required",
    "details": {}
  }
}
```

**Response 400** (invalid key / value):

```json
{
  "error": {
    "code": "invalid_config",
    "message": "Invalid configuration value; see server logs for details",
    "details": {}
  }
}
```

**Response 413** (request body too large):

```json
{
  "error": {
    "code": "request_too_large",
    "message": "Request body too large",
    "details": {}
  }
}
```

**Response 500** (persistence failure):

```json
{
  "error": {
    "code": "save_failed",
    "message": "Config applied but failed to persist; see server logs for details",
    "details": {}
  }
}
```

---

### 4.7. `GET /api/v1/jobs`

List scheduled jobs from all plugins.

**Response 200:**

```json
[
  {
    "id": "update.security",
    "plugin": "update",
    "description": "Run security updates",
    "schedule": "0 3 * * *",
    "next_run_time": null
  },
  {
    "id": "update.full",
    "plugin": "update",
    "description": "Run full upgrade",
    "schedule": null,
    "next_run_time": null
  }
]
```

> **Note:** `next_run_time` is always `null` in the current implementation.
> It will be computed from the cron expression in a future phase.

---

### 4.8. `POST /api/v1/jobs/trigger`

Trigger a job by ID.

**Request body:**

```json
{
  "job_id": "update.security"
}
```

**Response 202:**

```json
{
  "status": "accepted",
  "job_id": "update.security"
}
```

**Response 400** (invalid JSON):

```json
{
  "error": {
    "code": "invalid_request",
    "message": "Invalid JSON body",
    "details": {}
  }
}
```

**Response 400** (trailing data):

```json
{
  "error": {
    "code": "invalid_request",
    "message": "Request body must contain exactly one JSON object",
    "details": {}
  }
}
```

**Response 400** (missing job_id):

```json
{
  "error": {
    "code": "invalid_request",
    "message": "job_id is required",
    "details": {}
  }
}
```

**Response 404:**

```json
{
  "error": {
    "code": "job_not_found",
    "message": "Job 'update.security' not found",
    "details": {}
  }
}
```

**Response 413** (request body too large):

```json
{
  "error": {
    "code": "request_too_large",
    "message": "Request body too large",
    "details": {}
  }
}
```

**Response 500:**

```json
{
  "error": {
    "code": "trigger_failed",
    "message": "Failed to trigger job; see server logs",
    "details": {}
  }
}
```

> Also returns `500` with `"scheduler_unavailable"` if the scheduler is not
> configured.

### 4.9. `GET /api/v1/jobs/{id}/runs/latest`

Returns the most recent execution record for a job. Useful for polling job
progress after triggering via `POST /api/v1/jobs/trigger`.

The `{id}` parameter uses dot-notation (e.g., `update.full`, `update.security`).

**Response 200 (running):**

```json
{
  "job_id": "update.full",
  "status": "running",
  "started_at": "2026-03-02T00:10:00Z"
}
```

**Response 200 (completed):**

```json
{
  "job_id": "update.full",
  "status": "completed",
  "started_at": "2026-03-02T00:10:00Z",
  "ended_at": "2026-03-02T00:10:45Z",
  "duration": "45s"
}
```

**Response 200 (failed):**

```json
{
  "job_id": "update.full",
  "status": "failed",
  "started_at": "2026-03-02T00:10:00Z",
  "ended_at": "2026-03-02T00:10:03Z",
  "error": "job failed; see server logs",
  "duration": "3s"
}
```

**Response 404 (no runs):**

```json
{
  "error": {
    "code": "no_runs",
    "message": "No runs recorded for job 'update.full'",
    "details": {}
  }
}
```

**Response 404 (job not found):**

```json
{
  "error": {
    "code": "job_not_found",
    "message": "Job 'unknown' not found",
    "details": {}
  }
}
```

**Response 500:**

```json
{
  "error": {
    "code": "scheduler_unavailable",
    "message": "Scheduler not configured",
    "details": {}
  }
}
```

---

### 4.10. `GET /api/v1/jobs/{id}/runs`

Returns paginated execution history for a job, newest-first. History is
persisted to disk and survives service restarts.

The `{id}` parameter uses dot-notation (e.g., `update.full`, `update.security`).

**Note:** The `error` field in run records is sanitized before being returned
to clients. Internal error details are replaced with `"job failed; see server
logs"` to avoid leaking implementation details. This matches the sanitization
applied by the `/runs/latest` endpoint.

**Query params:**

| Param    | Type | Default | Description                      |
| -------- | ---- | ------- | -------------------------------- |
| `limit`  | int  | 20      | Max results per page (1–100)     |
| `offset` | int  | 0       | Number of records to skip        |

**Response 200:**

```json
[
  {
    "job_id": "update.security",
    "status": "completed",
    "started_at": "2026-03-02T03:00:00Z",
    "ended_at": "2026-03-02T03:00:45Z",
    "duration": "45s"
  },
  {
    "job_id": "update.security",
    "status": "failed",
    "started_at": "2026-03-01T03:00:00Z",
    "ended_at": "2026-03-01T03:00:03Z",
    "error": "job failed; see server logs",
    "duration": "3s"
  }
]
```

Returns an empty JSON array (`[]`) when no runs have been recorded.

**Response 400 (invalid limit):**

```json
{
  "error": {
    "code": "invalid_parameter",
    "message": "limit must be a positive integer",
    "details": {}
  }
}
```

**Response 400 (invalid offset):**

```json
{
  "error": {
    "code": "invalid_parameter",
    "message": "offset must be a non-negative integer",
    "details": {}
  }
}
```

**Response 404 (job not found):**

```json
{
  "error": {
    "code": "job_not_found",
    "message": "Job 'unknown' not found",
    "details": {}
  }
}
```

**Response 500:**

```json
{
  "error": {
    "code": "storage_error",
    "message": "Failed to retrieve job history",
    "details": {}
  }
}
```

> Also returns `500` with `"scheduler_unavailable"` if the scheduler is not
> configured.

---

## 5. Plugin Endpoints

### 5.1. Mounting rules

Each plugin is mounted under:

```text
/api/v1/plugins/{plugin_name}
```

Plugin handlers define paths relative to this base.

Example (update plugin):

- `/api/v1/plugins/update/status`
- `/api/v1/plugins/update/run`
- `/api/v1/plugins/update/config`

The exact schemas for each plugin live in the plugin's own `specs/SPEC.md`.

---

## 6. Authentication (Future)

Initial version runs without auth on localhost only.

Later, optional auth modes:

- Shared token in header:

  ```text
  Authorization: Bearer <token>
  ```

- Basic auth for simple setups.

Details will be added as the security model evolves.
