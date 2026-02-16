# Config Manager Core — API Specification

Base path: `/api/v1`

All responses are JSON. Errors use a common error format.

---

## 1. Error format

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

## 2. Core endpoints

### 2.1. `GET /api/v1/health`

Health check for the core service.

**Response 200:**

```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

---

### 2.2. `GET /api/v1/node`

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

### 2.3. `GET /api/v1/plugins`

List loaded plugins.

**Response 200:**

```json
[
  {
    "name": "update",
    "version": "0.1.0",
    "description": "System updates management"
  },
  {
    "name": "network",
    "version": "0.1.0",
    "description": "Network configuration"
  }
]
```

---

### 2.4. `GET /api/v1/plugins/{name}`

Get metadata for a specific plugin.

**Path params:**

- `name`: plugin name.

**Response 200:**

```json
{
  "name": "update",
  "version": "0.1.0",
  "description": "System updates management"
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

### 2.5. `GET /api/v1/jobs`

List scheduled jobs from all plugins.

**Response 200:**

```json
[
  {
    "id": "update.run_security",
    "plugin": "update",
    "description": "Run security updates",
    "schedule": "0 3 * * *",
    "next_run_time": "2026-02-16T03:00:00Z"
  },
  {
    "id": "update.run_full",
    "plugin": "update",
    "description": "Run full upgrade",
    "schedule": null,
    "next_run_time": null
  }
]
```

---

### 2.6. `POST /api/v1/jobs/trigger`

Trigger a job by ID.

**Request body:**

```json
{
  "job_id": "update.run_full"
}
```

**Response 202:**

```json
{
  "status": "accepted",
  "job_id": "update.run_full"
}
```

**Response 404:**

```json
{
  "error": {
    "code": "job_not_found",
    "message": "Job 'update.run_full' not found",
    "details": {}
  }
}
```

---

## 3. Plugin endpoints

### 3.1. Mounting rules

Each plugin is mounted under:

```text
/api/v1/plugins/{plugin_name}
```

Plugin handlers define paths relative to this base.

Example (update plugin):

- `/api/v1/plugins/update/status`
- `/api/v1/plugins/update/run`
- `/api/v1/plugins/update/config`

The exact schemas for each plugin live in the plugin's own `specs/API.md`.

---

## 4. Authentication (future)

Initial version runs without auth on localhost only.

Later, optional auth modes:

- Shared token in header:
  ```text
  Authorization: Bearer <token>
  ```
- Basic auth for simple setups.

Details will be added as the security model evolves.
