# Ticket 01: Live Status Package

**Depends on**: Ticket 00 (contracts). The full schema is defined inline below — you do NOT need `contracts/live-status.json` to exist to implement this ticket.

## Goal

Create `internal/livestatus/livestatus.go` — a small package that defines the `LiveStatus` struct matching the contract and provides atomic read/write functions. This is the shared-state bridge between the daemon and the menu bar app.

## Context

The Go daemon will write `~/.config/perf-guard/live-status.json` every poll cycle. A future SwiftUI menu bar app reads it. The concurrency model is: single writer, single reader, last-write-wins via atomic rename. See `contracts/README.md` for full rules.

The existing state package (`internal/state/state.go`) handles `state.json` with a similar pattern (JSON marshal + write) but does NOT use atomic writes. This package must use temp file + `os.Rename`.

## Deliverables

### New file: `internal/livestatus/livestatus.go`

```go
package livestatus

// LiveStatus mirrors contracts/live-status.json.
type LiveStatus struct {
    SchemaVersion  int              `json:"schema_version"`
    Level          string           `json:"level"`
    CompressionRate float64         `json:"compression_rate"`
    HelperCount    int              `json:"helper_count"`
    TopProcesses   []TopProcess     `json:"top_processes"`
    LastSpikeTime  string           `json:"last_spike_time,omitempty"`
    ForensicCount  int              `json:"forensic_count"`
    Suspended      []SuspendedBrief `json:"suspended"`
    VMStopped      bool             `json:"vm_stopped"`
    UpdatedAt      string           `json:"updated_at"`
}

type TopProcess struct {
    PID     int     `json:"pid"`
    Name    string  `json:"name"`
    RSSBytes uint64 `json:"rss_bytes"`
    CPUPct  float64 `json:"cpu_pct"`
}

type SuspendedBrief struct {
    PID  int    `json:"pid"`
    Name string `json:"name"`
}
```

**Functions**:

1. `Path() string` — returns `~/.config/perf-guard/live-status.json`
2. `Write(status *LiveStatus) error` — marshal to JSON (indented), write to temp file in same directory, `os.Rename` to final path. Always sets `SchemaVersion = 1` and `UpdatedAt` to current time.
3. `Read() (*LiveStatus, error)` — read + unmarshal. Return error if file missing (caller decides what to do).

### New file: `internal/livestatus/livestatus_test.go`

Test cases:
- Write then Read round-trip — all fields preserved
- `SchemaVersion` is always 1 after Write regardless of input
- `UpdatedAt` is set automatically
- Read on missing file returns clear error
- JSON round-trip: unknown fields in file are ignored by Read (forward compat)

## Onboarding

**New package directory**: `internal/livestatus/` does not exist yet. Create it:
```bash
mkdir -p internal/livestatus
```

**Module path**: `github.com/k7lim/perf-guard` (see `go.mod`). Your import path will be `github.com/k7lim/perf-guard/internal/livestatus`.

**Go version**: 1.26.1 (see `go.mod`).

**Reference pattern**: Read `internal/state/state.go` to see the existing JSON load/save pattern. Your package is similar but adds atomic writes (temp + rename) and auto-sets `SchemaVersion` and `UpdatedAt`.

**Quick grep** to see how `ConfigPath` builds the `~/.config/perf-guard/` base path:
```bash
rg "config/perf-guard" internal/config/config.go
```

## Implementation notes

- Use `os.CreateTemp` in the same directory as the target file (same filesystem = atomic rename guaranteed on APFS).
- `os.MkdirAll` the config directory on Write if it doesn't exist.
- The `SuspendedBrief` type is intentionally simpler than `state.SuspendedProcess` — the live status only needs PID + name for display.

## Existing code to reference

- `internal/state/state.go` — similar load/save pattern but without atomic writes. Module path: `github.com/k7lim/perf-guard/internal/state`.
- `internal/config/config.go:71-75` — `ConfigPath()` pattern for `~/.config/perf-guard/`.

## Files touched

| File | Action |
|------|--------|
| `internal/livestatus/livestatus.go` | Create |
| `internal/livestatus/livestatus_test.go` | Create |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.4 --append-notes "..." --json`

Downstream tickets that import this package: 04a (daemon writes status), 10 (adds spike time + forensic count), 12 (Swift app reads the JSON this produces).

## Acceptance criteria

- [ ] `go build ./internal/livestatus/...` compiles
- [ ] `go test ./internal/livestatus/...` passes
- [ ] Write produces valid JSON matching the `LiveStatus` struct shape defined above
- [ ] Atomic write: uses temp file + `os.Rename`, not direct write
- [ ] Tests use `t.TempDir()` to avoid writing to real `~/.config/perf-guard/`
