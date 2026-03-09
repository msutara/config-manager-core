#!/usr/bin/env bash
#
# integration_test.sh — Smoke-test the Config Manager binary.
#
# Builds from local workspace (or tagged modules), starts the server
# in headless mode, verifies all plugins register and respond, then
# tears down.  Designed for pre-release validation.
#
# Usage:
#   ./scripts/integration_test.sh          # uses go.work if present
#   GOWORK=off ./scripts/integration_test.sh  # uses go.mod only (CI)
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$REPO_ROOT/build"
BINARY="$BUILD_DIR/cm-integration-test"
DATA_DIR=$(mktemp -d)
CONFIG="$DATA_DIR/config.yaml"
TOKEN_FILE="$DATA_DIR/auth.token"
LOG_FILE="$DATA_DIR/cm.log"
PORT=17788
BASE_URL="http://localhost:$PORT"
PID=""

cleanup() {
    if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
        kill "$PID" 2>/dev/null || true
        wait "$PID" 2>/dev/null || true
    fi
    rm -rf "$DATA_DIR" "$BINARY"
}
trap cleanup EXIT

echo "=== Config Manager Integration Test ==="
echo ""

# --- Step 1: Build ---
echo "[1/6] Building binary..."
mkdir -p "$BUILD_DIR"
cd "$REPO_ROOT"
go build -o "$BINARY" ./cmd/cm
echo "      Built: $BINARY"

# --- Step 2: Configure ---
echo "[2/6] Creating test configuration..."
echo "test-token-for-integration" > "$TOKEN_FILE"

cat > "$CONFIG" <<EOF
listen_host: localhost
listen_port: $PORT
log_level: info
data_dir: $DATA_DIR
storage_backend: json
auth_token_file: $TOKEN_FILE
EOF
echo "      Config: $CONFIG"

# --- Step 3: Start headless ---
echo "[3/6] Starting server (headless)..."
"$BINARY" --config "$CONFIG" --headless > "$LOG_FILE" 2>&1 &
PID=$!

# Wait for server to be ready (up to 10 seconds)
for i in $(seq 1 20); do
    if curl -sf "$BASE_URL/api/v1/health" > /dev/null 2>&1; then
        break
    fi
    if ! kill -0 "$PID" 2>/dev/null; then
        echo "FAIL: Server exited prematurely"
        cat "$LOG_FILE"
        exit 1
    fi
    sleep 0.5
done

if ! curl -sf "$BASE_URL/api/v1/health" > /dev/null 2>&1; then
    echo "FAIL: Server did not become ready within 10 seconds"
    cat "$LOG_FILE"
    exit 1
fi
echo "      Server running (PID $PID, port $PORT)"

# --- Helper ---
AUTH="Authorization: Bearer test-token-for-integration"

api_get() {
    local path="$1"
    local expect_status="${2:-200}"
    local status
    status=$(curl -sf -o /dev/null -w '%{http_code}' -H "$AUTH" "$BASE_URL$path" 2>/dev/null || true)
    if [ "$status" = "$expect_status" ]; then
        echo "      PASS $path -> $status"
        return 0
    else
        echo "      FAIL $path -> $status (expected $expect_status)"
        return 1
    fi
}

FAILURES=0

# --- Step 4: Core endpoints ---
echo "[4/6] Testing core endpoints..."
api_get "/api/v1/health" 200 || ((FAILURES++))
api_get "/api/v1/node" 200 || ((FAILURES++))
api_get "/api/v1/plugins" 200 || ((FAILURES++))
api_get "/api/v1/jobs" 200 || ((FAILURES++))

# --- Step 5: Plugin registration ---
echo "[5/6] Verifying plugin registration..."

# Get plugin list and verify expected plugins are present
PLUGINS=$(curl -sf -H "$AUTH" "$BASE_URL/api/v1/plugins" 2>/dev/null)

for name in update network; do
    if echo "$PLUGINS" | grep -q "\"$name\""; then
        echo "      PASS plugin '$name' registered"
    else
        echo "      FAIL plugin '$name' NOT registered"
        ((FAILURES++))
    fi
done

# --- Step 6: Plugin endpoints ---
echo "[6/6] Testing plugin endpoints..."

# Update plugin
api_get "/api/v1/plugins/update" 200 || ((FAILURES++))
api_get "/api/v1/plugins/update/status" 200 || ((FAILURES++))

# Network plugin
api_get "/api/v1/plugins/network" 200 || ((FAILURES++))
api_get "/api/v1/plugins/network/interfaces" 200 || ((FAILURES++))
api_get "/api/v1/plugins/network/dns" 200 || ((FAILURES++))
api_get "/api/v1/plugins/network/status" 200 || ((FAILURES++))

# Web UI (if running)
api_get "/" 200 || ((FAILURES++))

echo ""
echo "=== Results ==="
if [ "$FAILURES" -eq 0 ]; then
    echo "ALL TESTS PASSED"
    exit 0
else
    echo "$FAILURES TEST(S) FAILED"
    echo ""
    echo "Server log:"
    tail -20 "$LOG_FILE"
    exit 1
fi
