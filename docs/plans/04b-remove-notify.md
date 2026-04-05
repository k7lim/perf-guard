# Ticket 04b: Remove Notification System

**Depends on**: Ticket 04a (daemon live-status writer must be in place first — it's the replacement channel)

## Goal

Remove all `notify.Send()` calls from the daemon and delete the `internal/notify/` package. This is the second half of replacing notifications with live-status.json.

**Per P18 (Single-Goal Edits)**: This ticket does ONE thing: remove the old notification system. The replacement (live-status.json) was added in ticket 04a.

## Context

The daemon (`internal/daemon/daemon.go`) calls `notify.Send()` to show macOS `osascript` popups on pressure level transitions. Now that ticket 04a writes `live-status.json` every poll cycle, the menu bar app (ticket 12) replaces these intrusive notifications.

## Deliverables

### Modify `internal/daemon/daemon.go`

1. **Remove import**: `"github.com/k7lim/perf-guard/internal/notify"` (line 14 of current file)

2. **Remove all 4 `notify.Send()` calls** from `act()` method. Locate them:
```bash
rg -n "notify.Send" internal/daemon/daemon.go
```
As of this writing, they are at approximately lines 135, 140, 152, 160. Remove the calls but **keep the surrounding action logic** (VM stop, SIGSTOP helpers, resume, etc.) intact.

Example — line 135 area:
```go
// BEFORE:
notify.Send("Memory Pressure Rising", "...")
// AFTER: (line deleted, surrounding code unchanged)
```

### Delete `internal/notify/notify.go`

The entire file is 17 lines — a single `Send(title, body string)` function that shells out to `osascript`. Delete the file and the `internal/notify/` directory.

### Verify no other callers

```bash
rg "notify" --type go
```

The ONLY reference should be in `daemon.go`. If anything else imports `"github.com/k7lim/perf-guard/internal/notify"`, remove that import too. As of this writing, no other file imports it.

## Files touched

| File | Action |
|------|--------|
| `internal/daemon/daemon.go` | Edit — remove 4 `notify.Send()` calls + import |
| `internal/notify/notify.go` | **Delete** |
| `internal/notify/` | **Delete directory** |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.11 --append-notes "..." --json`

No downstream dependencies — this is a leaf task (terminal cleanup).

## Acceptance criteria

- [ ] `go build ./...` compiles with no reference to `internal/notify`
- [ ] `rg "notify" --type go` returns zero results
- [ ] No `osascript` notifications on level transitions
- [ ] Existing daemon behavior (VM stop, SIGSTOP, hysteresis) unchanged
- [ ] `act()` method still performs all non-notification actions (VM stop, SIGSTOP, resume)
