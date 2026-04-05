# perf-guard

macOS memory pressure monitor that automatically manages runaway processes. Single binary, no dependencies.

Built for an M4 Pro with 48 GB RAM that was drowning in memory compression thrash (4.7 billion compressions), 235 browser helper processes, and swap-driven I/O spikes. No existing tool combines memory pressure monitoring with automated process suspension on macOS -- earlyoom and nohang do this on Linux but can't run here.

## Install

```bash
go build -o perf-guard ./cmd/perf-guard/
cp perf-guard /usr/local/bin/

# Optional: run as a LaunchAgent (starts on login, auto-restarts)
perf-guard install
```

## Usage

### Check current pressure

```
$ perf-guard status
Pressure Level:    CRITICAL

Compression rate:  8096 pages/s
Decompress rate:   11391 pages/s
Swap in rate:      16 pages/s
Swap out rate:     0 pages/s

Pages in compressor: 7090319 (108.2 GB)
Total compressions:  7859211927
Total swapins:       100352968
Total swapouts:      106660057

Browser helpers:     372

Thresholds: warning=500  high=1000  critical=2000 pages/s  max_helpers=150
```

### One-click focus mode

```bash
perf-guard focus          # SIGSTOP all browser helpers, VS Code helpers; stop Lima VM
perf-guard restore        # resume everything
```

### Targeted actions

```bash
perf-guard suspend vm         # stop Lima VM only
perf-guard suspend browsers   # SIGSTOP browser helpers only
perf-guard restore            # resume all
```

### Background daemon

```bash
perf-guard daemon   # runs in foreground, logs to /tmp/perf-guard.log

# Or install as LaunchAgent:
perf-guard install
```

The daemon monitors continuously with adaptive polling (30s idle, 5s warning, 1s critical) and takes graduated actions:

| Level | Trigger | Action |
|-------|---------|--------|
| Warning | >500 pages/s compressed | macOS notification |
| High | >1000 pages/s compressed | Stop Lima VM |
| Critical | >2000 pages/s OR >150 helpers | SIGSTOP browser + editor helpers |

Hysteresis prevents flapping: 3 consecutive readings below threshold before downgrading.

## How it works

- **Monitoring:** Parses `/usr/bin/vm_stat` to compute compression rate (pages/sec) between consecutive snapshots. Counts browser helper processes via `ps`.
- **Suspension:** Uses `SIGSTOP`/`SIGCONT`, not `SIGKILL`. Processes freeze in place and resume instantly. No data loss, no restarts needed.
- **State tracking:** Records every PID we suspend in `~/.config/perf-guard/state.json`. On restore, verifies PID + process name still match (guards against PID reuse).
- **Protection list:** Never touches Finder, loginwindow, WindowServer, sshd, or anything in the `protect` config list.
- **VM management:** Stops/starts Lima VMs via `limactl` (graceful shutdown, not SIGSTOP).

## Configuration

Optional. Works with sensible defaults out of the box. To customize:

```bash
mkdir -p ~/.config/perf-guard
cp config/default.toml ~/.config/perf-guard/config.toml
```

```toml
[polling]
normal = 30      # seconds between checks when idle
warning = 5      # when pressure rising
critical = 1     # when taking action

[thresholds]
warning = 500    # pages/s compressed -> notify
high = 1000      # pages/s compressed -> stop VM
critical = 2000  # pages/s compressed -> SIGSTOP helpers
max_helpers = 150

# Processes to NEVER touch (substring match, not regex)
protect = [
    "Finder", "loginwindow", "WindowServer",
    "sshd", "Spotlight", "iTerm2", "Terminal",
]

# Process groups to manage. Lower priority = acted on first.
[groups.lima]
match = "lima"
action = "vm-stop"
priority = 0

[groups.brave]
match = "Brave Browser Helper"
action = "suspend"
priority = 1

[groups.firefox]
match = "firefox"
action = "suspend"
priority = 2

[groups.vscode]
match = "Code Helper"
action = "suspend"
priority = 3
```

Process matching is deliberately simple: substring match, not regex. This avoids the footgun where nohang users accidentally killed their desktop environment with a misconfigured regex.

## Machine-readable output

Every command supports `--output json` for agent and script consumption. Output auto-detects: JSON when piped, text in a terminal. Set `OUTPUT_FORMAT=json` to always get JSON.

```bash
# Structured status
perf-guard status --output json --fields level,compression_rate,browser_helpers

# Preview before acting
perf-guard focus --dry-run --output json

# Schema introspection
perf-guard describe
```

See [AGENTS.md](AGENTS.md) for agent integration details.

## Design decisions

Key decisions informed by studying earlyoom (3.9K stars) and nohang (1.2K stars) issue trackers:

- **SIGSTOP, not kill.** nohang's kill-based approach caused 10-30 minute system freezes. SIGSTOP is reversible.
- **Adaptive polling.** Fixed 10Hz polling was earlyoom's most discussed issue. We poll every 30s when idle, 1s under pressure.
- **Warn before acting.** earlyoom's #1 open feature request. The daemon notifies at Warning level before taking action at High/Critical.
- **Substring match, not regex.** nohang's regex config caused users to accidentally match and kill their desktop environment.
- **Per-PID signals, not process groups.** earlyoom's process group signaling caused a signal storm bug.
- **Hysteresis.** 3 consecutive readings below threshold to downgrade. Prevents notification spam and suspend/resume oscillation.

See [DIAGNOSTIC-REPORT.md](DIAGNOSTIC-REPORT.md) for the full system analysis that motivated this tool.
