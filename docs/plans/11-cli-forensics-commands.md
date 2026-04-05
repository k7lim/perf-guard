# Ticket 11: CLI Forensics Commands

**Depends on**: Ticket 09 (snapshot assembly — `ListSnapshots`, `LoadSnapshot` functions)

## Goal

Add `perf-guard forensics list|show|report` subcommands to the CLI. All return the standard JSON envelope. These are read-only sensor commands — no mutations.

## Context

The CLI uses `spf13/cobra` for command registration. All commands are in `cmd/perf-guard/main.go` (622 lines) except `describe` which is in `cmd/perf-guard/describe.go`. Commands use the shared `printer` (type `output.Printer`) for output and the `output.Response` envelope.

The forensic snapshot storage and query functions (`ListSnapshots`, `LoadSnapshot`) are in `internal/forensic/snapshot.go` (ticket 09).

**Import path**: `github.com/k7lim/perf-guard/internal/forensic`

## Deliverables

### New file: `cmd/perf-guard/forensics.go`

Create a new file (following the pattern of `describe.go` being separate from `main.go`).

**1. `forensics` parent command**:
```go
func forensicsCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "forensics",
        Short: "Query forensic snapshots captured during memory pressure spikes",
    }
    cmd.AddCommand(forensicsListCmd())
    cmd.AddCommand(forensicsShowCmd())
    cmd.AddCommand(forensicsReportCmd())
    return cmd
}
```

**2. `forensics list [--limit N] [--offset N]`**:
- Default limit: 20, offset: 0
- Calls `forensic.ListSnapshots(limit, offset)`
- Returns envelope: `{"command": "forensics list", "success": true, "data": [...]}`
- Data is array of `SnapshotMeta` (timestamp, level, compression_rate, process_count, tab_count)
- **Per P5 (Paginate Always)**: Response `meta` must include `total`, `offset`, `limit` so agents can iterate. Use the existing envelope's `data` for the array and add pagination info. Check how existing commands handle this — if the envelope doesn't have a `meta` field, wrap the data:
  ```json
  {"command": "forensics list", "success": true, "data": {"items": [...], "total": 42, "offset": 0, "limit": 20}}
  ```

**3. `forensics show <timestamp>`**:
- Takes one positional arg: timestamp prefix (e.g., "2026-04-04T2347")
- Calls `forensic.LoadSnapshot(prefix)`
- Returns envelope with full snapshot in `data`
- Error if no match or ambiguous match

**4. `forensics report [--days N]`**:
- Default: 7 days. Loads snapshots where `snapshot.Timestamp` is within the last N*24 hours from `time.Now()` (local timezone).
- Computes aggregate analysis:
  - `most_frequent_processes`: For each snapshot, take the first 5 entries from `Processes` (already sorted by RSS desc). Count how many snapshots each process name appears in across those top-5 lists. Return sorted by count desc, limit 10.
  - `most_frequent_urls`: Tab URLs appearing most often (if CDP data present, with count)
  - `spikes_by_hour`: Array of 24 entries, count of spikes per hour-of-day
  - `avg_compression_at_spike`: Mean compression rate across all snapshots
  - `total_spikes`: Count of snapshots in range
- Returns envelope with this analysis object in `data`

### Modify `cmd/perf-guard/main.go`

Add `root.AddCommand(forensicsCmd())` in the command registration block. Confirm exact location:
```bash
rg -n "AddCommand" cmd/perf-guard/main.go
```
As of this writing, commands are registered at lines 45-51. Insert before `describeCmd()` (line 51).

### Modify `cmd/perf-guard/describe.go`

Add the three forensics commands to the schema output. Follow the existing pattern — each command gets a `commandSchema` entry with description, flags, output_schema, and notes. Mark all three as `Mutates: false`.

Add them as subcommands of a `"forensics"` command entry:

```go
"forensics": {
    Description: "Query forensic snapshots",
    Subcommands: map[string]commandSchema{
        "list":   { ... },
        "show":   { ... },
        "report": { ... },
    },
},
```

## Existing CLI patterns to follow

**Command registration** (main.go ~line 31-56):
```go
var root = &cobra.Command{ ... }
root.AddCommand(statusCmd())
root.AddCommand(focusCmd())
// ... etc
root.AddCommand(describeCmd())
```

**Output via printer** (used in every command). The `Print` method takes THREE args — the third is a `textFn` closure for text-mode rendering:
```go
printer.Print("command-name", data, func() {
    // text-mode output here (fmt.Printf, etc.)
    // only called when output format is Text, skipped for JSON
})
printer.PrintError("command-name", err) // failure (two args)
```

Verify the actual signature: `rg "func.*Printer.*Print\b" internal/output/output.go`

Where `printer` is a package-level `*output.Printer` initialized in `PersistentPreRunE`.

**Response envelope** (`internal/output/output.go` lines 20-26):
```go
type Response struct {
    Command   string `json:"command"`
    Success   bool   `json:"success"`
    Error     string `json:"error,omitempty"`
    Data      any    `json:"data"`
    Timestamp string `json:"timestamp"`
}
```

**Flags**: Use cobra local flags on the subcommand. See existing commands for pattern — `--limit`, `--offset` as int flags.

## Report analysis types

```go
type ForensicsReport struct {
    TotalSpikes           int                 `json:"total_spikes"`
    MostFrequentProcesses []FrequencyEntry    `json:"most_frequent_processes"`
    MostFrequentURLs      []FrequencyEntry    `json:"most_frequent_urls"`
    SpikesByHour          [24]int             `json:"spikes_by_hour"`
    AvgCompressionAtSpike float64             `json:"avg_compression_at_spike"`
}

type FrequencyEntry struct {
    Name  string `json:"name"`
    Count int    `json:"count"`
}
```

## Files touched

| File | Action |
|------|--------|
| `cmd/perf-guard/forensics.go` | Create — list, show, report subcommands |
| `cmd/perf-guard/main.go` | Edit — add `root.AddCommand(forensicsCmd())` |
| `cmd/perf-guard/describe.go` | Edit — add forensics to schema |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.13 --append-notes "..." --json`

Leaf task — no downstream code dependencies.

## Acceptance criteria

- [ ] `go build ./cmd/perf-guard` compiles
- [ ] `perf-guard forensics list --output json` returns valid envelope with array data
- [ ] `perf-guard forensics list --limit 5 --offset 0` paginates correctly
- [ ] `perf-guard forensics show <prefix> --output json` returns full snapshot
- [ ] `perf-guard forensics report --days 7 --output json` returns aggregate analysis
- [ ] `perf-guard describe` includes forensics commands in schema
- [ ] All commands use the standard `output.Response` envelope
- [ ] Text output mode works — follow the existing pattern in main.go where each command has a `textFn` closure. Suggested text formats:
  - `list`: table with columns `TIMESTAMP  LEVEL  COMP_RATE  PROCS  TABS`
  - `show`: key-value pairs for metadata, then numbered process list
  - `report`: labeled sections with counts and percentages
