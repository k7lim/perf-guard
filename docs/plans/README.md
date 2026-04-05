# perf-ect Tickets

Split from plan: `~/.claude/plans/cozy-mixing-galaxy.md`

## Dependency Graph

```
00 Contracts + AGENTS.md
├── 01 LiveStatus package ──────┐
│                               ├── 04a Daemon: add live status writer ─── 04b Remove notify
02 TopByRSS (independent) ─────┘                                          │
                                                                           │
03 Forensic config (independent) ──┐                                       │
05 Spike detector ─────────────────┤                                       │
06 Process snapshot (independent) ─┤                                       │
07 CDP facade ─────────────────────┼── 09 Snapshot assembly ── 10 Wire into daemon
08 IO stats facade ────────────────┘                           │
                                                               │
                                              09 ──── 11 CLI forensics commands

00 + 01 ──── 12 SwiftUI menu bar (parallel with 05-11)
```

## Tickets

| # | Title | Depends on | Files | Atomic? |
|---|-------|-----------|-------|---------|
| [00](00-contracts-and-agents-md.md) | Contracts + AGENTS.md update | — | AGENTS.md, contracts/ | Yes |
| [01](01-livestatus-package.md) | Live status package | 00 | internal/livestatus/ | Yes |
| [02](02-top-by-rss.md) | TopByRSS in process pkg | — | internal/process/process.go | Yes |
| [03](03-forensic-config.md) | Forensic config additions | — | internal/config/config.go | Yes |
| [04a](04a-daemon-livestatus.md) | Daemon: add live status writer | 01, 02 | internal/daemon/daemon.go | Yes |
| [04b](04b-remove-notify.md) | Remove notification system | 04a | internal/daemon/daemon.go, internal/notify/ (delete) | Yes |
| [05](05-spike-detector.md) | Spike detector | 03 | internal/forensic/spike.go | Yes |
| [06](06-process-snapshot.md) | Process snapshot | — | internal/forensic/procs.go | Yes |
| [07](07-cdp-facade.md) | CDP tab facade | 03 | internal/forensic/cdp.go | Yes |
| [08](08-io-stats-facade.md) | IO stats facade | 03 | internal/forensic/iostats.go | Yes |
| [09](09-snapshot-assembly.md) | Snapshot assembly + storage | 05, 06, 07, 08 | internal/forensic/snapshot.go | Yes |
| [10](10-wire-forensics-into-daemon.md) | Wire forensics into daemon | 04a, 05, 09, 03 | internal/daemon/daemon.go | Yes |
| [11](11-cli-forensics-commands.md) | CLI forensics commands | 09 | cmd/perf-guard/forensics.go, describe.go, main.go | Yes |
| [12](12-swiftui-menu-bar.md) | SwiftUI menu bar app | 01, 00 | menubar/ | Yes |

## Parallelization opportunities

These groups can be worked simultaneously:

- **Group A** (independent): 00, 02, 03
- **Group B** (after 00): 01
- **Group C** (after 03): 05, 06, 07, 08 (all four in parallel)
- **Group D** (after 01+02): 04a → 04b (sequential)
- **Group E** (after 05-08): 09
- **Group F** (after 04a+09): 10
- **Group G** (after 09): 11
- **Group H** (after 01, parallel with C-G): 12

## Engineering principles compliance

Each ticket was QA'd against `engineering_core.md`:

| Principle | How it's addressed |
|-----------|--------------------|
| P2 One Envelope | All CLI commands (tickets 00, 11) use `output.Response` envelope |
| P3 SKILL.md First | Ticket 00 updates AGENTS.md (the SKILL.md equivalent) before any code |
| P4 Facade Externals | Tickets 07 (CDP), 08 (ioreg) are explicit facades with unexported upstream types |
| P5 Paginate Always | Ticket 11 `forensics list` has limit/offset/total |
| P7 Edge Validation | Tickets 03, 07 validate config/port at boundaries |
| P9 Append-Only | Forensic snapshots are write-once (ticket 09) |
| P11 Shared State | Contracts (ticket 00) define live-status.json + snapshot schemas with schema_version |
| P14 Concurrency | Atomic writes (tickets 01, 09), mutex on lastSpikeTime (ticket 10), polling model (ticket 12) |
| P17 Safety Rails | All actuators have --dry-run; forensic.enabled flag (ticket 10) |
| P18 Single-Goal | Ticket 04 split into 04a (add) + 04b (remove) |
| P19 Edit-Test | Tickets include test counts and edit-test cadence guidance |

## Shared context

Tickets reference these files when they need cross-ticket context:
- `contracts/live-status.json` — JSON shape for daemon↔menu bar communication
- `contracts/forensic-snapshot.json` — JSON shape for forensic captures
- `AGENTS.md` — CLI command reference and decision framework
