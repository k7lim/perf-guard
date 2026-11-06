# perf-guard

For the first session-review implementation milestone, start with the
[M1 orchestration guide](docs/MILESTONE-1.md) and its
[machine-readable task graph](docs/milestone-1.tasks.json). They define the
contract-first dispatch order, work boundaries, verification, and SITREP handback.

macOS memory pressure monitor that automatically manages runaway processes. Single binary, no dependencies.

Built for an M4 Pro with 48 GB RAM that was drowning in memory compression thrash (4.7 billion compressions), 235 browser helper processes, and swap-driven I/O spikes. No existing tool combines memory pressure monitoring with automated process suspension on macOS -- earlyoom and nohang do this on Linux but can't run here.

## Install

```bash
go build -o perf-guard ./cmd/perf-guard/
cp perf-guard /usr/local/bin/

# Recommended first run: monitor, alert and capture diagnostics only
perf-guard install --monitor-only --dry-run
perf-guard install --monitor-only
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
perf-guard daemon --monitor-only   # foreground monitoring and alerts, no process actions

# Or install as LaunchAgent:
perf-guard install --monitor-only
```

With `--monitor-only`, the daemon logs pressure changes, posts macOS notifications, writes a heartbeat, and captures forensic snapshots. It never suspends/resumes processes or stops/starts VMs. The mode persists in the LaunchAgent arguments.

Without monitor-only, the daemon monitors continuously with adaptive polling (30s idle, 5s warning, 1s critical) and takes graduated actions:

| Level | Trigger | Action |
|-------|---------|--------|
| Warning | >500 pages/s compressed | macOS notification |
| High | >1000 pages/s compressed | Stop Lima VM |
| Critical | ≥2000 pages/s; helper count ≥150 raises the compression-based level by one | SIGSTOP browser + editor helpers |

Hysteresis requires 3 consecutive readings below threshold before downgrading.

## How it works

- **Monitoring:** Parses `/usr/bin/vm_stat` to compute compression rate (pages/sec) between consecutive snapshots. Counts browser helper processes via `ps`.
- **Suspension:** Uses `SIGSTOP`/`SIGCONT`, not `SIGKILL`. Processes freeze in place and resume instantly. No data loss, no restarts needed.
- **State tracking:** Records every PID we suspend in `~/.config/perf-guard/state.json`. On restore, verifies PID + process name still match (guards against PID reuse).
- **Protection list:** Never touches Finder, loginwindow, WindowServer, sshd, or anything in the `protect` config list.
- **VM management:** Stops/starts Lima VMs via `limactl` (graceful shutdown, not SIGSTOP).

## Configuration

Pressure notifications are sent on escalation only. Repeated severities are
limited to one alert every five minutes; the first escalation to a higher
severity can alert immediately during that cooldown. Downgrades stay quiet.
Monitoring, diagnostics and menu-bar status continue at their normal intervals.

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

CLI sensors and actuators support `--output json`; daemon runtime events go to its log. Every interactive command supports `--output json` for agent and script consumption. Output auto-detects: JSON when piped, text in a terminal. Set `OUTPUT_FORMAT=json` to always get JSON.

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
- **Pressure notifications.** The daemon notifies on upward pressure changes, including jumps straight to Critical. Monitor-only mode keeps notifications without automatic actions.
- **Substring match, not regex.** nohang's regex config caused users to accidentally match and kill their desktop environment.
- **Per-PID signals, not process groups.** earlyoom's process group signaling caused a signal storm bug.
- **Hysteresis.** 3 consecutive readings below threshold to downgrade. A separate notification cooldown limits alert bursts when pressure keeps oscillating.

See [DIAGNOSTIC-REPORT.md](DIAGNOSTIC-REPORT.md) for the full system analysis that motivated this tool.

## Agent process inventory

```bash
perf-guard sessions --output json --limit 20 --offset 0
perf-guard status --output json --fields level,compression_rate,compressor_gb,browser_helpers,daemon
```

`sessions` separates terminal sessions, nested terminal helpers, desktop helpers,
app servers, and other background agent processes. It reports PID, parent, TTY,
age, and resident memory without reading command arguments or terminating anything.
Age is not inactivity; resident memory excludes compressed and swapped pages.
There is no automatic session reaper.

For scoped, identity-safe read-only review and fresh dry-run plans:

```sh
perf-guard review --output json --herdr-endpoint /absolute/herdr.sock --herdr-session explicit-session
perf-guard review plan --output json --session REVIEW_ID --herdr-endpoint /absolute/herdr.sock --herdr-session explicit-session
```

Both commands are sensors. Plans always report `execution_supported=false` and
preserve the shell; no session is exited. Missing Herdr context is an explicit
partial result. See the [review service guide](internal/review/README.md) for
selection, freshness, pagination, inherited target context and verification.

`compressor_gb` is the physical GiB occupied by the compressor, not the size of
original pages stored inside it. `daemon.stale` flags a heartbeat older than
90 seconds; `daemon: null` means no readable heartbeat. A heartbeat does not
prove notification delivery. Process-discovery failures produce an error rather
than a misleading zero helper count. If an agent sandbox blocks `ps`, run the
read-only sensor with host process visibility.

## Installed service

- **Name:** Performance Guard - Monitor; user LaunchAgent `com.user.perf-guard`.
- **Owner/installer:** this repository, `perf-guard install --monitor-only`.
- **Executable:** `~/Library/Application Support/perf-guard/bin/perf-guard`.
- **Launcher:** sibling executable `Performance Guard - Monitor`.
- **CLI:** `~/.local/bin/perf-guard` forwards arguments to the installed binary.
  Ensure `~/.local/bin` precedes any older installation on PATH.
- **Plist:** `~/Library/LaunchAgents/com.user.perf-guard.plist`.
- **Schedule:** starts at login; launchd restarts it if it exits. Polling is 30s
  at normal pressure, 5s warning/high, and 1s critical.
- **Data:** `~/.config/perf-guard/live-status.json`, `state.json`, and `forensics/`.
  Log: `/tmp/perf-guard.log`. No root privileges are required.
- **Disable impact:** continuous monitoring, pressure notifications, heartbeat,
  and automatic forensic capture stop; manual CLI sensors remain available.
- **Check:** `launchctl print gui/$(id -u)/com.user.perf-guard`, then check fresh
  `status` telemetry and the log. `launchctl print-disabled gui/$(id -u)` shows
  the explicit disabled override independently of loaded/running state.
- **Disable:** `launchctl bootout gui/$(id -u)/com.user.perf-guard`, then
  `launchctl disable gui/$(id -u)/com.user.perf-guard`.
- **Restore:** preview and run `perf-guard install --monitor-only` again.
  Upgrades preserve existing plist settings, including log destinations.
- **Rollback:** each install prints a timestamped backup under
  `~/Library/Application Support/perf-guard/backups/`. It saves the original
  plist and replaced payload/launchers. Stop the service before restoring those
  files. The installer explicitly enables the service, including a previously
  disabled one. Its returned `loaded` field indicates successful bootstrap;
  verify the next heartbeat to confirm useful operation.

Browser tab capture is best-effort and requires a compatible local CDP endpoint.
Other forensic data still works if CDP is unavailable. Runtime snapshots can
contain private process paths, URLs, and tab titles; keep them out of Git.

## Menu bar companion

The daemon runs independently of the menu bar. To build and install the companion:

```bash
bash menubar/install.sh --dry-run
bash menubar/install.sh --apply
```

This installs `~/Applications/Performance Guard.app`, launches it, and uses
Apple's `SMAppService.mainApp` registration to request opening at login. macOS
may require approval in Login Items. It leaves the daemon's mode unchanged.
The menu shows **PG** with a colored pressure dot, live status, and the current
monitor-only mode. **Quit Menu** closes only the companion; monitoring continues.

The installed app prefers the current user-owned CLI, with a bundled CLI as a
fallback. Reinstall through `menubar/install.sh` after changing the Swift sources.
Existing installed app bundles are copied into the perf-guard backups directory
before replacement. Build products and runtime session-review reports are not
committed. To disable menu startup, remove Performance Guard from macOS Login
Items; disabling the menu does not disable the `com.user.perf-guard` monitor.
