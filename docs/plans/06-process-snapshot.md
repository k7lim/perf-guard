# Ticket 06: Forensic Process Snapshot

**Depends on**: Nothing (independent)

## Goal

Create `internal/forensic/procs.go` — captures an extended process snapshot (top 30 by RSS) with more fields than the existing `process.Info` struct. This is one of the data sources assembled into a forensic snapshot.

## Context

The existing `process.Info` struct has `PID, Name, RSS, CPU`. The forensic snapshot needs more: VSZ and MEM% for memory analysis. Rather than modifying the existing struct (which is used throughout the codebase), create a forensic-specific `ProcessSnapshot` struct and a capture function.

## Deliverables

### New file: `internal/forensic/procs.go`

```go
package forensic

type ProcessSnapshot struct {
    PID      int     `json:"pid"`
    Name     string  `json:"name"`
    RSSBytes uint64  `json:"rss_bytes"`
    VSZBytes uint64  `json:"vsz_bytes"`
    CPUPct   float64 `json:"cpu_pct"`
    MEMPct   float64 `json:"mem_pct"`
}

// CaptureProcesses runs `ps -eo pid,rss,vsz,%cpu,%mem,comm` and returns
// the top N processes sorted by RSS descending.
func CaptureProcesses(n int) ([]ProcessSnapshot, error)
```

**Implementation**:
- Run `ps -eo pid,rss,vsz,%cpu,%mem,comm`
- Parse each line using `strings.Fields()` (space-delimited, NOT column-position-based):
  - `fields[0]` = PID (int)
  - `fields[1]` = RSS in KB (uint64, multiply by 1024 for bytes)
  - `fields[2]` = VSZ in KB (uint64, multiply by 1024 for bytes)
  - `fields[3]` = %CPU (float64)
  - `fields[4]` = %MEM (float64)
  - `fields[5:]` = COMM — **use `strings.Join(fields[5:], " ")` to handle process names with spaces** (e.g., "Brave Browser Helper (Renderer)")
- Sort by RSSBytes descending
- Return top `n` (default 30 in caller, but function takes param)
- Skip header line (first line of ps output)
- Skip PID 0 (kernel_task — not useful)
- Required imports: `os/exec`, `sort`, `strconv`, `strings`, `fmt`

**Directory note**: `internal/forensic/` may not exist yet. Create it if needed: `mkdir -p internal/forensic`. If ticket 05 was done first, the directory already exists.

### New file: `internal/forensic/procs_test.go`

Test the parsing logic by passing known `ps` output. Don't test against live system in unit tests.

Test cases:
- Parse sample ps output with 5 lines → correct struct fields
- RSS/VSZ converted from KB to bytes
- Sorted by RSS descending
- `n` limits output
- Handles lines with spaces in process name (e.g., "Brave Browser Helper (Renderer)")
- Empty input returns empty slice, no error

**Sample test input** (first line is the header — skip it during parsing):
```
  PID   RSS   VSZ  %CPU %MEM COMM
 1234 1257812 4394000  34.2  2.7 /Applications/Brave Browser.app/Contents/Frameworks/Brave Browser Framework.framework/Versions/133.1.75.178/Helpers/Brave Browser Helper (Renderer)
  567 524288 2000000  12.1  1.1 /usr/local/bin/node
    1  15000  400000   0.0  0.0 /sbin/launchd
```

## Design notes

- This function is independent of `process.Manager` — it doesn't need a protect list because forensic snapshots should capture everything (the point is diagnosis, not action).
- The `comm` field from `ps` gives the full path. Keep the full path in `Name` for forensic value. The existing `process.parsePS` also keeps the full comm output.

## Files touched

| File | Action |
|------|--------|
| `internal/forensic/procs.go` | Create |
| `internal/forensic/procs_test.go` | Create |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.6 --append-notes "..." --json`

Downstream: ticket 09 (snapshot assembly) calls `CaptureProcesses(30)` and uses `ProcessSnapshot` type.

## Acceptance criteria

- [ ] `go test ./internal/forensic/...` passes
- [ ] Parses real macOS `ps -eo pid,rss,vsz,%cpu,%mem,comm` output format
- [ ] RSS and VSZ in bytes (not KB)
- [ ] Sorted by RSS descending, top N returned
- [ ] Handles process names with spaces and parentheses
