#!/bin/bash
# diagnose.sh — figure out what's sending perf-guard notifications
set -u

echo "=== perf-guard diagnostic ==="
echo "Date: $(date)"
echo

# 1. Running perf-guard processes
echo "--- Running perf-guard processes ---"
ps aux | grep -i perf-guard | grep -v grep || echo "(none)"
echo

# 2. LaunchAgent plist
echo "--- LaunchAgent plist ---"
PLIST="$HOME/Library/LaunchAgents/com.user.perf-guard.plist"
if [ -f "$PLIST" ]; then
    echo "Found: $PLIST"
    cat "$PLIST"
    echo
    echo "LaunchAgent status:"
    launchctl list | grep perf-guard || echo "(not loaded)"
else
    echo "No plist at $PLIST"
fi
echo

# 3. What binary is actually running (if any)
echo "--- Binary details ---"
PIDS=$(pgrep -f "perf-guard" 2>/dev/null)
if [ -n "$PIDS" ]; then
    for pid in $PIDS; do
        echo "PID $pid:"
        ps -p "$pid" -o pid,ppid,lstart,command=
        # On macOS, get the actual binary path
        lsof -p "$pid" -Fn 2>/dev/null | head -3
        echo
    done
else
    echo "(no perf-guard processes found)"
fi
echo

# 4. Check if the running binary has notify/osascript compiled in
echo "--- Binary contents check (notify/osascript) ---"
for candidate in \
    "$HOME/go/bin/perf-guard" \
    /opt/homebrew/bin/perf-guard \
    /usr/local/bin/perf-guard; do
    if [ -x "$candidate" ]; then
        echo "$candidate:"
        echo "  modified: $(stat -f '%Sm' "$candidate" 2>/dev/null || stat -c '%y' "$candidate" 2>/dev/null)"
        if strings "$candidate" | grep -q "osascript"; then
            echo "  *** HAS osascript — this is an OLD binary with notifications ***"
        else
            echo "  clean (no osascript)"
        fi
        if strings "$candidate" | grep -q "live-status"; then
            echo "  has live-status writer"
        else
            echo "  *** MISSING live-status writer ***"
        fi
        echo
    fi
done
echo

# 5. Live-status file
echo "--- live-status.json ---"
STATUS="$HOME/.config/perf-guard/live-status.json"
if [ -f "$STATUS" ]; then
    echo "Found: $STATUS"
    echo "Modified: $(stat -f '%Sm' "$STATUS" 2>/dev/null || stat -c '%y' "$STATUS" 2>/dev/null)"
    cat "$STATUS"
else
    echo "Not found — daemon is not writing status (or never ran with live-status support)"
fi
echo

# 6. Daemon log
echo "--- Daemon log (last 30 lines) ---"
LOG=/tmp/perf-guard.log
if [ -f "$LOG" ]; then
    tail -30 "$LOG"
else
    echo "No log at $LOG"
fi
echo

# 7. Currently running osascript processes (the notification mechanism)
echo "--- Active osascript processes ---"
ps aux | grep osascript | grep -v grep || echo "(none)"
echo

# 8. Config file
echo "--- Config ---"
CFG="$HOME/.config/perf-guard/config.toml"
if [ -f "$CFG" ]; then
    cat "$CFG"
else
    echo "No config file (using compiled defaults)"
fi
echo

echo "=== end diagnostic ==="
