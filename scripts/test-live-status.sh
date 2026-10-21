#!/bin/bash
# Test that the daemon writes live-status.json
set -euo pipefail
cd "$(dirname "$0")/.."

LIVE_STATUS="$HOME/.config/perf-guard/live-status.json"

go build -o perf-guard ./cmd/perf-guard

# Remove stale file if present
rm -f "$LIVE_STATUS"

echo "=== Starting daemon ==="
./perf-guard daemon &
DAEMON_PID=$!

# Poll for up to 45 seconds (daemon takes ~2s for initial vm_stat + up to 30s poll interval)
echo "=== Waiting for live-status.json (up to 45s) ==="
for i in $(seq 1 45); do
    if [ -f "$LIVE_STATUS" ]; then
        echo "  found after ${i}s"
        break
    fi
    sleep 1
done

kill $DAEMON_PID 2>/dev/null || true
wait $DAEMON_PID 2>/dev/null || true

echo "=== Checking live-status.json ==="
if [ ! -f "$LIVE_STATUS" ]; then
    echo "FAIL: $LIVE_STATUS not written after 45s"
    rm -f perf-guard
    exit 1
fi

jq -e '.schema_version == 1' "$LIVE_STATUS"
jq -e '.level' "$LIVE_STATUS"
jq -e '.updated_at' "$LIVE_STATUS"

rm -f perf-guard
echo "Live status tests passed"
