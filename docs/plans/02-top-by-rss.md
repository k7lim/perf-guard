# Ticket 02: Add `TopByRSS` to Process Package

**Depends on**: Nothing (independent)

## Goal

Add a `TopByRSS(n int) ([]Info, error)` method to `process.Manager` that returns the top N processes sorted by RSS descending. This is needed by the live-status writer (ticket 04) and the forensic process snapshot (ticket 07).

## Context

`internal/process/process.go` currently has a `parsePS` function (line 125) that runs `ps -eo pid,rss,%cpu,comm`, parses the output, and filters by a substring. `FindBySubstring` is the only caller. We need to refactor so both `FindBySubstring` and the new `TopByRSS` can share the parsing logic without the substring filter baked in.

## Deliverables

### Modify `internal/process/process.go`

1. **Rename `parsePS` to `parsePSAll`** — remove the `substr string` parameter. Return ALL parsed processes (no filtering). Keep the same `ps` command: `ps -eo pid,rss,%cpu,comm`.

2. **Update `FindBySubstring`** — call `parsePSAll()`, then filter the result by substring in a loop. Existing behavior unchanged.

3. **Add `TopByRSS(n int) ([]Info, error)`** — call `parsePSAll()`, sort by `RSS` descending, return top `n` entries. If fewer than `n` processes exist, return all. Skip protected processes (they're uninteresting for display). `n <= 0` returns empty slice.

4. **Add test file or extend existing tests** if present.

### Existing code (key lines in `internal/process/process.go`)

```go
// Line 14-19
type Info struct {
    PID  int
    Name string
    RSS  uint64  // bytes
    CPU  float64 // percent
}

// Line 22-24
type Manager struct {
    Protect []string
}

// Line 32-42 — FindBySubstring calls parsePS with substring filter
// Line 125-156 — parsePS: runs ps, parses, filters by substring
```

The `parsePS` function at line 125 has signature `parsePS(output, substr string) []Info` — takes BOTH the raw ps output string and a substring filter. The refactor splits this into: `parsePSAll(output string) []Info` (no filter), then callers filter.

**Onboarding**: Read the full file before starting:
```bash
rg "func.*parsePS\|func.*FindBySubstring\|func.*TopByRSS" internal/process/process.go
```

**Key detail**: The `sort` package is NOT currently imported — you must add it for `TopByRSS`.

**Protected-process filtering for TopByRSS**: Look at `safetyCheck()` (line 93-106) for the pattern. It iterates `m.Protect` and checks `strings.Contains(name, p)`. In `TopByRSS`, apply the same substring match as a filter (exclude from results), not as an error.

**Test file**: `internal/process/process_test.go` does NOT exist. Create it from scratch.

## Files touched

| File | Action |
|------|--------|
| `internal/process/process.go` | Edit — refactor `parsePS` → `parsePSAll`, add `TopByRSS` |
| `internal/process/process_test.go` | Create or edit — test `TopByRSS` sorting and boundary cases |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.2 --append-notes "..." --json`

Downstream: ticket 04a calls `mgr.TopByRSS(5)` from the daemon's `writeLiveStatus()`.

## Acceptance criteria

- [ ] `FindBySubstring` behavior unchanged (existing callers unaffected)
- [ ] `TopByRSS(5)` returns at most 5 processes, sorted by RSS desc
- [ ] `TopByRSS(0)` returns empty slice
- [ ] Protected processes are excluded from `TopByRSS` results
- [ ] `go test ./internal/process/...` passes
- [ ] `go build ./...` compiles (no broken callers)

## Edit-test cadence (P19)

After each step, run tests and verify:
1. After `parsePSAll` refactor + `FindBySubstring` update: `go build ./...` still compiles, existing behavior unchanged. Expected: 0 test failures (no tests yet, but build must pass).
2. After adding `TopByRSS`: `go test ./internal/process/...` — expect all new tests pass. Target: 4-6 test cases covering sorting, n=0, n>len, protected filtering.
3. After all changes: `go build ./...` to verify no broken callers in daemon or CLI.
