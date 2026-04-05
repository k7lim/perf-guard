# Ticket 04a: Daemon Writes Live Status

**Depends on**: Ticket 01 (livestatus package), Ticket 02 (TopByRSS)

## Goal

Add `writeLiveStatus()` to the daemon's main loop so it writes `live-status.json` every poll cycle. This is the first half of replacing notifications — the new output channel. Notification removal is a separate commit (ticket 04b).

**Per P18 (Single-Goal Edits)**: This ticket does ONE thing: add the live-status writer. The notify removal is independently verifiable and reversible in ticket 04b.

## Context

The daemon (`internal/daemon/daemon.go`, 242 lines) polls vm_stat and manages processes. After this ticket, it also writes a JSON status file every cycle for the menu bar app to read.

The live-status package (ticket 01) provides `livestatus.Write()` with atomic file operations. The `TopByRSS` method (ticket 02) provides the top processes for the `top_processes` field.

## Deliverables

### Modify `internal/daemon/daemon.go`

1. **Add import**: `"github.com/k7lim/perf-guard/internal/livestatus"`

2. **Add `writeLiveStatus()` method** to `Daemon`:

```go
func (d *Daemon) writeLiveStatus(reading monitor.PressureReading, helperCount int) {
    topProcs, _ := d.mgr.TopByRSS(5)

    // Convert process.Info → livestatus.TopProcess (field mapping):
    //   process.Info.PID (int)    → TopProcess.PID (int)
    //   process.Info.Name (string) → TopProcess.Name (string)
    //   process.Info.RSS (uint64)  → TopProcess.RSSBytes (uint64)  -- direct assignment, both bytes
    //   process.Info.CPU (float64) → TopProcess.CPUPct (float64)
    var top []livestatus.TopProcess
    for _, p := range topProcs {
        top = append(top, livestatus.TopProcess{PID: p.PID, Name: p.Name, RSSBytes: p.RSS, CPUPct: p.CPU})
    }

    // Convert state.SuspendedProcess → livestatus.SuspendedBrief (drop Group, SuspendedAt)
    var susp []livestatus.SuspendedBrief
    for _, s := range d.state.Suspended {
        susp = append(susp, livestatus.SuspendedBrief{PID: s.PID, Name: s.Name})
    }

    status := &livestatus.LiveStatus{
        Level:           d.currentLevel.String(), // pressure.Level has a String() method
        CompressionRate: reading.CompressionRate,
        HelperCount:     helperCount,
        TopProcesses:    top,
        Suspended:       susp,
        VMStopped:       d.state.VMStopped,
        // LastSpikeTime and ForensicCount left at zero — ticket 10 adds these
    }
    if err := livestatus.Write(status); err != nil {
        d.logger.Printf("write live status: %v", err)
    }
}
```

3. **Call `writeLiveStatus()` in the main loop** — insert the call **after line 106** (after hysteresis check) but **outside** the `if effectiveLevel != d.currentLevel` block (lines 108-113). This ensures it runs every poll cycle, not just on level changes. Confirm placement:
```bash
rg -n "ComputeRates\|Evaluate\|hysteresis\|effectiveLevel != d.currentLevel" internal/daemon/daemon.go
```

## Existing code structure

**Daemon struct** (daemon.go lines 25-34):
```go
type Daemon struct {
    cfg          config.Config
    mgr          *process.Manager
    thresholds   pressure.Thresholds
    currentLevel pressure.Level
    belowCount   int
    lastSnapshot *monitor.VmStatSnapshot
    state        state.State
    logger       *log.Logger
}
```

**Main loop** (daemon.go lines 81-114): polls vm_stat, computes rates, counts helpers, evaluates level, applies hysteresis, calls `act()` on level changes.

**process.Info struct** (process.go lines 14-19):
```go
type Info struct {
    PID  int
    Name string
    RSS  uint64  // bytes
    CPU  float64 // percent
}
```

**state.SuspendedProcess** (state.go lines 11-16):
```go
type SuspendedProcess struct {
    PID         int       `json:"pid"`
    Name        string    `json:"name"`
    Group       string    `json:"group"`
    SuspendedAt time.Time `json:"suspended_at"`
}
```

## Files touched

| File | Action |
|------|--------|
| `internal/daemon/daemon.go` | Edit — add `writeLiveStatus()`, call it in loop, add livestatus import |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.9 --append-notes "..." --json`

Downstream: ticket 04b removes notify (depends on this being in place first). Ticket 10 modifies `writeLiveStatus` to add spike time + forensic count.

## Acceptance criteria

- [ ] `go build ./...` compiles
- [ ] Daemon writes `~/.config/perf-guard/live-status.json` every poll cycle
- [ ] Live status JSON matches the `LiveStatus` struct shape from ticket 01
- [ ] `top_processes` field populated with top 5 by RSS
- [ ] `suspended` field reflects current `state.Suspended`
- [ ] Existing daemon behavior (VM stop, SIGSTOP, hysteresis, **and notifications**) unchanged — notify calls still present
