# Ticket 00: Shared State Contracts + AGENTS.md Update

## Goal

Define the machine-readable contracts for two new shared-state files (`live-status.json` and forensic snapshots) and update `AGENTS.md` with the new forensic sensor commands. This is a documentation-only ticket — no Go or Swift code.

## Why this matters

Every subsequent ticket depends on these contracts. The live-status file is the interface between the Go daemon (writer) and a future SwiftUI menu bar app (reader). The forensic snapshot schema is the interface between the capture system and the CLI query commands. Getting these wrong means rework across multiple packages.

## Deliverables

### 1. Update `AGENTS.md`

Add a new **Sensors** section in `AGENTS.md` **after line 93** (where `### install` ends) and **before line 95** (`## Decision framework`). Group forensic commands as read-only sensors:

**Onboarding**: Run `rg -n '### install|## Decision framework' AGENTS.md` to confirm insertion points. As of this writing, `### install` section's last content line is 93 and `## Decision framework` starts at line 95. Insert the new Sensors block between these.

```
### forensics list (read-only)

Paginated list of forensic snapshots.

    perf-guard forensics list --output json
    perf-guard forensics list --output json --limit 10 --offset 0

Key fields in each entry: `timestamp`, `level`, `compression_rate`, `process_count`, `tab_count`.

### forensics show (read-only)

Full forensic snapshot by timestamp prefix.

    perf-guard forensics show 2026-04-04T2347 --output json

Returns the complete snapshot in the `data` field.

### forensics report (read-only)

Aggregate analysis across stored snapshots.

    perf-guard forensics report --output json --days 7

Key fields: `most_frequent_processes`, `most_frequent_urls`, `spikes_by_hour`, `avg_compression_at_spike`.
```

All commands return the existing envelope: `{"command", "success", "error", "data", "timestamp"}`.

Also update the **Decision framework** section to add step 6. Verify with `rg -n 'If .critical.:' AGENTS.md` — step 5 is at line 103. The paragraph after step 5 (lines 105-106) discusses daemon/CLI conflict. Insert step 6 between step 5 and that paragraph:

> 6. After any intervention, run `forensics list` to check if a spike was captured. If so, `forensics show <timestamp>` gives the agent deep context about what caused the pressure.

Note: The forensic commands referenced in step 6 are defined in the new Sensors section you added above.

### 2. Create `contracts/live-status.json`

Create a new `contracts/` directory at project root. Write a JSON example file documenting the live-status contract:

```json
{
  "_comment": "Contract: Go daemon writes, menu bar app reads. See tickets/00 for rules.",
  "schema_version": 1,
  "level": "warning",
  "compression_rate": 842.5,
  "helper_count": 127,
  "top_processes": [
    {"pid": 1234, "name": "Brave Browser Helper (Renderer)", "rss_bytes": 1288000000, "cpu_pct": 34.2}
  ],
  "last_spike_time": "2026-04-04T23:47:12-07:00",
  "forensic_count": 7,
  "suspended": [],
  "vm_stopped": false,
  "updated_at": "2026-04-04T23:50:01-07:00"
}
```

**Rules** (add as comments or a companion `contracts/README.md`):
- **Location**: `~/.config/perf-guard/live-status.json`
- **Concurrency**: Single writer (daemon), single reader (menu bar). Last-write-wins via temp file + `os.Rename` (atomic on APFS).
- **Forward compat**: Reader MUST ignore unknown fields. Writer MUST increment `schema_version` on breaking changes. Reader shows "daemon too new" if `schema_version > highest_known`.

### 3. Create `contracts/forensic-snapshot.json`

```json
{
  "_comment": "Contract: one file per spike, append-only. See tickets/00 for rules.",
  "schema_version": 1,
  "timestamp": "2026-04-04T23:47:12-07:00",
  "level": "critical",
  "compression_rate": 3200.5,
  "decompression_rate": 1800.2,
  "swap_in_rate": 45.0,
  "swap_out_rate": 120.3,
  "processes": [
    {"pid": 1234, "name": "Brave Browser Helper (Renderer)", "rss_bytes": 1288000000, "vsz_bytes": 4500000000, "cpu_pct": 34.2, "mem_pct": 2.7}
  ],
  "tabs": [
    {"url": "https://youtube.com/watch?v=abc", "title": "Some Video", "type": "page"}
  ],
  "io": {
    "read_bytes": 0,
    "write_bytes": 0,
    "source": "ioreg"
  },
  "vmstat": {}
}
```

**Rules**:
- **Location**: `~/.config/perf-guard/forensics/<timestamp>.json` (e.g., `2026-04-04T234712.json`)
- **Concurrency**: Atomic write via temp+rename. Files are write-once, never modified.
- **Rotation**: Keep last 100 files, delete oldest.
- Same `schema_version` rules as live-status.

## Files touched

| File | Action |
|------|--------|
| `AGENTS.md` | Edit — add forensic sensor commands + decision framework step |
| `contracts/live-status.json` | Create |
| `contracts/forensic-snapshot.json` | Create |
| `contracts/README.md` | Create — concurrency model, forward-compat rules |

## Scratchpad

Use the bead's notes field for progress tracking: `bd update perf-ect-j0x.1 --append-notes "..." --json`

No code artifacts to hand off — this is a docs-only ticket. Downstream tickets (01, 12) reference `contracts/live-status.json` by path.

## Acceptance criteria

- [ ] `AGENTS.md` lists `forensics list`, `forensics show`, `forensics report` with correct flags and example output
- [ ] `contracts/` contains both JSON examples and a README with the concurrency/compat rules
- [ ] No Go code changes

## Note on P3 (SKILL.md First)

Per engineering_core.md principle 3, machine-readable contracts should be written before code. This ticket fulfills that role: `AGENTS.md` IS the SKILL.md equivalent for perf-guard (it documents sensors, actuators, output format, and workflows). The `contracts/` directory provides the shared-state schema contracts per P11. No separate SKILL.md file is needed — AGENTS.md already serves that purpose and is referenced by CLAUDE.md.
