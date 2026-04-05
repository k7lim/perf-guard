# Ticket 10: Wire Forensic Capture into Daemon

**Depends on**: Ticket 04 (daemon live-status), Ticket 05 (spike detector), Ticket 09 (snapshot assembly), Ticket 03 (forensic config)

## Goal

Connect the spike detector and forensic capture system to the daemon's main loop. When the spike detector fires, launch a goroutine to capture and save a forensic snapshot.

## Context

After tickets 04 and 05-09, the daemon writes live-status.json and all forensic packages exist. This ticket wires them together: the daemon feeds compression rates to the spike detector, and on spike, captures a snapshot.

## Deliverables

### Modify `internal/daemon/daemon.go`

1. **Add fields to Daemon struct**:

```go
type Daemon struct {
    // ... existing fields ...
    spikeDetector *forensic.SpikeDetector
    lastSpikeMu   sync.Mutex
    lastSpikeTime time.Time
}
```

2. **Initialize in `New()`** or at start of `Run()`:

```go
d.spikeDetector = forensic.NewSpikeDetector(
    d.cfg.Forensic.WindowSize,
    time.Duration(d.cfg.Forensic.Cooldown) * time.Second,
)
```

3. **Add spike check in main loop** — insert after line 97 (`ComputeRates`) and before line 100 (`helperSubstrings` — the helper substring retrieval, not the count itself). Confirm with:
```bash
rg -n "ComputeRates\|helperSubstrings\|CountHelpers" internal/daemon/daemon.go
```
Expected output: ComputeRates at ~97, helperSubstrings at ~100, CountHelpers at ~101. Insert the spike check between ComputeRates and helperSubstrings.

```go
if d.cfg.Forensic.Enabled && d.spikeDetector.Record(reading.CompressionRate) {
    go d.captureForensic(reading)
}
```

4. **Add `captureForensic` method**:

```go
func (d *Daemon) captureForensic(reading monitor.PressureReading) {
    snap, err := forensic.CaptureSnapshot(
        d.currentLevel.String(),
        reading,
        d.cfg.Forensic.CDPPort,
        d.cfg.Forensic.FSUsage,
    )
    if err != nil {
        d.logger.Printf("forensic capture failed: %v", err)
        return
    }
    if err := forensic.SaveSnapshot(snap); err != nil {
        d.logger.Printf("forensic save failed: %v", err)
        return
    }
    if err := forensic.RotateSnapshots(d.cfg.Forensic.MaxKeep); err != nil {
        d.logger.Printf("forensic rotation failed: %v", err)
    }
    
    d.lastSpikeMu.Lock()
    d.lastSpikeTime = snap.Timestamp
    d.lastSpikeMu.Unlock()
    
    d.logger.Printf("forensic snapshot captured: %s", snap.Timestamp.Format(time.RFC3339))
}
```

5. **Update `writeLiveStatus`** (added by ticket 04) to include two new fields. Find the `writeLiveStatus` method and add this code BEFORE the `livestatus.Write(&status)` call:

```go
// Add spike time (mutex-protected, written by captureForensic goroutine)
d.lastSpikeMu.Lock()
spikeTime := d.lastSpikeTime
d.lastSpikeMu.Unlock()
if !spikeTime.IsZero() {
    status.LastSpikeTime = spikeTime.Format(time.RFC3339)
}

// Count forensic snapshots on disk for the ForensicCount field
forensicsDir := filepath.Join(os.UserHomeDir() /* handle err */, ".config", "perf-guard", "forensics")
if entries, err := os.ReadDir(forensicsDir); err == nil {
    status.ForensicCount = len(entries)
}
```

The `LiveStatus` struct (from ticket 01) already has `LastSpikeTime string` and `ForensicCount int` fields — you're just populating them.

### Imports to add

```go
import (
    "sync"
    "github.com/k7lim/perf-guard/internal/forensic"
)
```

## Concurrency model

- **Max one concurrent capture**: Enforced by the spike detector's cooldown (30s default). Even if somehow two spikes fire close together, each goroutine writes its own independent file — no contention.
- **`d.currentLevel`**: Read by goroutine, written by main loop. This is safe because: (a) the goroutine reads it once at capture start, (b) `pressure.Level` is an `int` (atomic read on 64-bit), (c) a stale level value in a snapshot is acceptable.
- **`d.lastSpikeTime`**: Written by goroutine, read by main loop's `writeLiveStatus` — protected by `lastSpikeMu`.
- **No mutex on `spikeDetector`**: Only called from main loop (single goroutine).

## Existing daemon loop structure (daemon.go)

The main loop (around line 81-114):
1. Sleep for poll interval
2. Take vm_stat snapshot
3. `monitor.ComputeRates(old, new)` → `reading`
4. Count browser helpers
5. `d.thresholds.Evaluate(rate, helperCount)` → `newLevel`
6. Hysteresis check
7. `d.act(oldLevel, newLevel)` on level change

The spike check goes between step 3 and step 4 (or after step 3, before step 5 — order doesn't matter much since it's a goroutine).

## Files touched

| File | Action |
|------|--------|
| `internal/daemon/daemon.go` | Edit — add spike detector field, init, main loop check, `captureForensic` method, update `writeLiveStatus` |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.12 --append-notes "..." --json`

No downstream code dependencies — but ticket 11 (CLI) and 12 (menu bar) consume the snapshots and live-status this produces.

## Acceptance criteria

- [ ] `go build ./...` compiles
- [ ] Daemon creates spike detector from config on startup
- [ ] Spike detection runs every poll cycle (when forensic.enabled=true)
- [ ] Goroutine captures snapshot on spike
- [ ] `lastSpikeTime` properly mutex-protected
- [ ] `live-status.json` includes `last_spike_time` and `forensic_count`
- [ ] Forensic disabled (`enabled=false`) → no spike detection or capture
- [ ] Rotation runs after each capture
