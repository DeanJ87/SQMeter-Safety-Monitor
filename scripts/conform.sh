#!/usr/bin/env bash
# Run an ASCOM ConformU conformance test against the service backed by a mock SQMeter.
#
# On Linux the conformu binary runs headlessly (CLI mode). On macOS the app is
# GUI-only, so this script starts the services and prints the device URL to paste
# into the ConformU browser UI, then waits until you press Ctrl-C.
#
# Usage:
#   bash scripts/conform.sh
#   CONFORM_BIN=/path/to/conformu bash scripts/conform.sh   # override binary
#
# Environment variables (all optional):
#   CONFORM_BIN   path to the conformu binary (auto-detected)
#   SQM_PORT      port for the mock SQMeter server  (default: 18080)
#   ALPACA_PORT   port for the Alpaca service        (default: 11111)
#   SERVICE_BIN   path to the built service binary   (default: bin/sqmeter-alpaca-safetymonitor)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SQM_PORT="${SQM_PORT:-18080}"
ALPACA_PORT="${ALPACA_PORT:-11111}"
SERVICE_BIN="${SERVICE_BIN:-${REPO_ROOT}/bin/sqmeter-alpaca-safetymonitor}"
IS_MACOS=false
[[ "$(uname)" == "Darwin" ]] && IS_MACOS=true

if [ ! -x "$SERVICE_BIN" ]; then
    echo "error: service binary not found at $SERVICE_BIN — run 'make build' first" >&2
    exit 1
fi

# Locate conformu binary (Linux/CI only — macOS uses the GUI app)
if ! $IS_MACOS; then
    if [ -z "${CONFORM_BIN:-}" ]; then
        if command -v conformu &>/dev/null; then
            CONFORM_BIN="conformu"
        elif [ -x "${REPO_ROOT}/.conform/conformu" ]; then
            CONFORM_BIN="${REPO_ROOT}/.conform/conformu"
        else
            echo "error: conformu not found — set CONFORM_BIN or run 'make conform-download'" >&2
            exit 1
        fi
    fi
fi

SQM_PID=""
SVC_PID=""

cleanup() {
    [ -n "$SQM_PID" ] && kill "$SQM_PID" 2>/dev/null || true
    [ -n "$SVC_PID" ] && kill "$SVC_PID" 2>/dev/null || true
}
trap cleanup EXIT

# Start mock SQMeter
python3 "$REPO_ROOT/scripts/mock-sqm.py" "$SQM_PORT" &
SQM_PID=$!

# Start Alpaca service pointing at the mock with fast polling.
# FAIL_CLOSED=false so the service reports safe even before the first poll lands.
SQMETER_BASE_URL="http://127.0.0.1:${SQM_PORT}" \
ALPACA_HTTP_BIND="127.0.0.1" \
ALPACA_HTTP_PORT="$ALPACA_PORT" \
ALPACA_DISCOVERY_PORT=32227 \
POLL_INTERVAL_SECONDS=1 \
STALE_AFTER_SECONDS=10 \
CONNECTED_ON_STARTUP=true \
FAIL_CLOSED=false \
  "$SERVICE_BIN" &
SVC_PID=$!

# Wait for the Alpaca management endpoint to respond
echo "waiting for Alpaca service on port ${ALPACA_PORT}..."
READY=0
for i in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:${ALPACA_PORT}/management/apiversions" >/dev/null 2>&1; then
        READY=1
        break
    fi
    sleep 1
done
if [ "$READY" -eq 0 ]; then
    echo "error: service did not become ready within 30s" >&2
    exit 1
fi

# Trigger an immediate SQMeter poll so IsSafe is populated before ConformU queries it.
curl -sf -X PUT \
    -d "Action=refresh&ClientID=0&ClientTransactionID=1" \
    "http://127.0.0.1:${ALPACA_PORT}/api/v1/safetymonitor/0/action" >/dev/null

DEVICE_URL="http://127.0.0.1:${ALPACA_PORT}/api/v1/safetymonitor/0"

if $IS_MACOS; then
    # ConformU on macOS is a GUI-only app. UDP discovery doesn't work on loopback,
    # so enter the device URL directly in the ConformU interface.
    echo ""
    echo "Services are ready."
    echo ""
    echo "  Mock SQMeter : http://127.0.0.1:${SQM_PORT}"
    echo "  Alpaca device: ${DEVICE_URL}"
    echo ""
    echo "In ConformU:"
    echo "  1. Select 'Alpaca Device' and choose device type 'SafetyMonitor'"
    echo "  2. Enter the Alpaca URL manually: ${DEVICE_URL}"
    echo "  3. Click 'Start Conform'"
    echo ""
    echo "Press Ctrl-C to stop the services when done."
    wait
else
    echo "running ConformU conformance test..."
    echo "device: ${DEVICE_URL}"
    echo ""
    "$CONFORM_BIN" conformance "$DEVICE_URL"
fi
