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

### 4.5. `GET /api/v1/jobs`

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

### 4.6. `POST /api/v1/jobs/trigger`

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

**Response 400:**

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
