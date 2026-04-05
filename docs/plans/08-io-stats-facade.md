# Ticket 08: IO Stats Facade

**Depends on**: Ticket 03 (forensic config — for `FSUsage` flag)

## Goal

Create `internal/forensic/iostats.go` — a facade over `ioreg` output that captures disk IO statistics. Translates raw `ioreg` text into our `IOStats` struct.

## Context

macOS doesn't have `/proc/diskstats`. The standard approach is `ioreg -c IOMedia -r -d 1` which outputs a property list of disk devices with read/write byte counts.

There's also `fs_usage` which gives per-process file I/O, but it requires `sudo` / root privileges. It's off by default (config flag `forensic.fs_usage = false`). This ticket only implements the `ioreg` path. `fs_usage` support is a placeholder that returns an error if enabled without privileges.

## Deliverables

### New file: `internal/forensic/iostats.go`

```go
package forensic

type IOStats struct {
    ReadBytes  int64  `json:"read_bytes"`
    WriteBytes int64  `json:"write_bytes"`
    Source     string `json:"source"` // "ioreg" or "fs_usage"
}

// CaptureIOStats runs `ioreg -c IOMedia -r -d 1` and sums read/write bytes
// across all disk devices. Returns IOStats with Source="ioreg".
//
// If fsUsage is true, returns an error explaining that fs_usage requires
// elevated privileges (placeholder for future implementation).
func CaptureIOStats(fsUsage bool) (IOStats, error)
```

**Implementation**:

1. If `fsUsage` is true, return `IOStats{}, fmt.Errorf("fs_usage requires elevated privileges; not implemented yet")`
2. Run `/usr/sbin/ioreg -c IOMedia -r` (no `-d` flag — depth limiting may miss nested byte-count properties)
3. Parse output looking for `"Bytes (Read)"` and `"Bytes (Write)"` properties. The ioreg output format is:
   ```
   +-o disk0s1  <class IOMedia, ...>
     | {
     |   ...
     |   "Bytes (Read)" = 12345678
     |   "Bytes (Write)" = 87654321
     |   ...
     | }
   ```
4. Sum across all devices (there may be multiple IOMedia entries).
5. Return `IOStats{ReadBytes: sum_read, WriteBytes: sum_write, Source: "ioreg"}`
6. On parse failure or ioreg error, return `IOStats{ReadBytes: 0, WriteBytes: 0, Source: ""}, fmt.Errorf("ioreg: %w", err)`. Caller (snapshot assembly) treats non-nil error as "IO stats unavailable" and uses the zero struct. The empty `Source` signals the data is invalid.

**Directory note**: `internal/forensic/` may not exist yet — create if needed.

**Verify ioreg output format**: Before writing the parser, run on the target machine and confirm:
```bash
/usr/sbin/ioreg -c IOMedia -r | grep "Bytes (Read)\|Bytes (Write)"
```
The output should show unquoted integers like `"Bytes (Read)" = 45821034496`.

### New file: `internal/forensic/iostats_test.go`

Test the parsing logic with sample `ioreg` output (don't call actual ioreg in tests).

Test cases:
- Single device with known read/write bytes → correct sums
- Multiple devices → bytes summed
- Missing bytes fields → zero for that device
- Empty output → zero stats, no error (or error, your call — document it)
- `fsUsage=true` → error returned

**Sample test input:**
```
+-o disk0s1  <class IOMedia, id 0x10000028f, registered, matched, active, busy 0 (0 ms), retain 7>
  | {
  |   "Content Hint" = "Apple_APFS"
  |   "Bytes (Read)" = 45821034496
  |   "Bytes (Write)" = 23847102464
  | }
+-o disk0s2  <class IOMedia, id 0x1000002a1, registered, matched, active, busy 0 (0 ms), retain 5>
  | {
  |   "Bytes (Read)" = 1024000
  |   "Bytes (Write)" = 512000
  | }
+-o disk0s3  <class IOMedia, id 0x1000002b3, registered, matched, active, busy 0 (0 ms), retain 3>
  | {
  |   "Content Hint" = "Apple_Boot"
  | }
```

The third device has NO `"Bytes (Read)"` or `"Bytes (Write)"` — parser must treat missing fields as 0 for that device. Expected totals: Read = 45821034496 + 1024000 + 0, Write = 23847102464 + 512000 + 0.

## Design notes

- **Facade pattern**: Raw ioreg output stays in this file. Callers only see `IOStats`.
- **`fs_usage` is intentionally stubbed**: It requires `sudo` and Kevin's AGENTS.md says the tool runs as the current user. Adding it later is straightforward — parse `fs_usage -f diskio` output — but not needed now.
- **Point-in-time**: These are cumulative counters from boot, not rates. The forensic snapshot stores the absolute values. Comparing two snapshots gives a rate if needed later.

## Files touched

| File | Action |
|------|--------|
| `internal/forensic/iostats.go` | Create |
| `internal/forensic/iostats_test.go` | Create |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.8 --append-notes "..." --json`

Downstream: ticket 09 calls `CaptureIOStats(fsUsage)` and uses `IOStats` type.

## Acceptance criteria

- [ ] `go test ./internal/forensic/...` passes
- [ ] Parses real macOS `ioreg -c IOMedia -r` output format
- [ ] Sums bytes across multiple devices
- [ ] `fsUsage=true` returns error (not implemented)
- [ ] `Source` field is `"ioreg"`
