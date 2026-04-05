# Ticket 09: Forensic Snapshot Assembly + Storage

**Depends on**: Tickets 05 (spike detector types), 06 (process snapshot), 07 (CDP facade), 08 (IO stats facade)

## Goal

Create `internal/forensic/snapshot.go` — assembles all forensic data sources into a single snapshot, writes it atomically, and handles rotation. This is the "orchestrator" that calls the capture functions from tickets 06-08.

## Context

When the spike detector fires, the daemon calls this module to capture a full forensic snapshot. It collects:
- Process list (top 30 by RSS) — from `CaptureProcesses()` (ticket 06)
- Browser tabs via CDP — from `CaptureTabs()` (ticket 07, optional)
- IO statistics — from `CaptureIOStats()` (ticket 08)
- The current vm_stat reading (passed in by caller)

Each data source can fail independently. Tabs and IO are optional — if they fail, the snapshot is still written with those fields null/empty.

## Deliverables

### New file: `internal/forensic/snapshot.go`

```go
package forensic

import (
    "time"
    "github.com/k7lim/perf-guard/internal/monitor"
)

type Snapshot struct {
    SchemaVersion int                    `json:"schema_version"`
    Timestamp     time.Time              `json:"timestamp"`
    Level         string                 `json:"level"`
    CompRate      float64                `json:"compression_rate"`
    DecompRate    float64                `json:"decompression_rate"`
    SwapInRate    float64                `json:"swap_in_rate"`
    SwapOutRate   float64                `json:"swap_out_rate"`
    Processes     []ProcessSnapshot      `json:"processes"`
    Tabs          []TabInfo              `json:"tabs,omitempty"`
    IO            IOStats                `json:"io"`
    VmStat        monitor.VmStatSnapshot `json:"vmstat"`
}
```

**Functions**:

1. **`CaptureSnapshot(level string, reading monitor.PressureReading, cdpPort int, fsUsage bool) (*Snapshot, error)`**
   - Sets `SchemaVersion = 1`, `Timestamp = time.Now()`
   - Field mapping from `reading` (type `monitor.PressureReading`):
     - `reading.CompressionRate` → `snap.CompRate`
     - `reading.DecompressionRate` → `snap.DecompRate`
     - `reading.SwapInRate` → `snap.SwapInRate`
     - `reading.SwapOutRate` → `snap.SwapOutRate`
     - `reading.Snapshot` → `snap.VmStat` (direct assignment, same type `monitor.VmStatSnapshot`)
   - Calls `CaptureProcesses(30)` — required, error fails the snapshot
   - Calls `CaptureTabs(cdpPort)` — optional, log warning via `log.Printf` on failure, set `Tabs` to nil
   - Calls `CaptureIOStats(fsUsage)` — optional, log warning via `log.Printf` on failure, set `IO` to zero struct
   - Copies `reading.Snapshot` into `VmStat`

2. **`SaveSnapshot(snap *Snapshot) error`**
   - Directory: `~/.config/perf-guard/forensics/`
   - Filename: timestamp formatted as `2006-01-02T150405.json` (Go reference time)
   - Atomic write: `os.CreateTemp` in same dir + `os.Rename`
   - `os.MkdirAll` the directory if needed

3. **`RotateSnapshots(maxKeep int) error`**
   - List all `.json` files in the forensics directory
   - Sort by name (lexicographic = chronological since filenames are timestamps)
   - Delete oldest files until count <= `maxKeep`

4. **`ListSnapshots(limit, offset int) ([]SnapshotMeta, error)`**
   - Returns metadata (timestamp, level, compression_rate, process count, tab count) without loading full snapshots
   - Reads each file's first few fields or loads and extracts
   - Sorted newest-first
   - Pagination via limit/offset

5. **`LoadSnapshot(timestampPrefix string) (*Snapshot, error)`**
   - Finds file matching prefix (e.g., "2026-04-04T2347" matches "2026-04-04T234712.json")
   - If zero files match: return `nil, fmt.Errorf("no snapshot matching prefix %q", prefix)`
   - If multiple files match: return `nil, fmt.Errorf("ambiguous prefix %q matches %d snapshots", prefix, count)`
   - Exactly one match: return the full deserialized snapshot

**Helper types**:

```go
type SnapshotMeta struct {
    Timestamp      time.Time `json:"timestamp"`
    Level          string    `json:"level"`
    CompRate       float64   `json:"compression_rate"`
    ProcessCount   int       `json:"process_count"`
    TabCount       int       `json:"tab_count"`
}
```

### New file: `internal/forensic/snapshot_test.go`

**All tests must use `t.TempDir()`** to avoid writing to real `~/.config/perf-guard/`. Pass a custom directory to save/load/rotate functions (add an optional `dir` parameter or use a package-level var for the base path that tests can override).

Test cases:
- `SaveSnapshot` + `LoadSnapshot` round-trip — all fields preserved
- `SchemaVersion` always 1
- Filename format: `2006-01-02T150405.json` (Go reference time format)
- `RotateSnapshots(3)` with 5 files → 2 oldest deleted
- `ListSnapshots` pagination: limit=2, offset=0 returns first 2 newest; offset=2 returns next 2
- `LoadSnapshot` with unique prefix → success
- `LoadSnapshot` with ambiguous prefix → error
- `LoadSnapshot` with no match → error

## Existing code to reference

- `internal/monitor/vmstat.go` — `VmStatSnapshot` struct (lines 12-36) and `PressureReading` struct (lines 39-46). Module: `github.com/k7lim/perf-guard/internal/monitor`
- `contracts/forensic-snapshot.json` (from ticket 00) — the target JSON shape

## Files touched

| File | Action |
|------|--------|
| `internal/forensic/snapshot.go` | Create |
| `internal/forensic/snapshot_test.go` | Create |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.10 --append-notes "..." --json`

Downstream: ticket 10 (daemon wiring) calls `CaptureSnapshot`, `SaveSnapshot`, `RotateSnapshots`. Ticket 11 (CLI) calls `ListSnapshots`, `LoadSnapshot`.

## Acceptance criteria

- [ ] `go build ./internal/forensic/...` compiles
- [ ] `go test ./internal/forensic/...` passes
- [ ] Snapshot JSON matches `contracts/forensic-snapshot.json` shape
- [ ] Atomic write (temp + rename)
- [ ] Rotation deletes oldest when count exceeds `maxKeep`
- [ ] Tab/IO failures produce partial snapshot (not total failure)
- [ ] Prefix-match load works
- [ ] Pagination works
