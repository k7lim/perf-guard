# Shared State Contracts

This directory defines the JSON schemas for shared-state files used by perf-guard components.

## Files

- **live-status.json** -- daemon (writer) to menu bar app (reader)
- **forensic-snapshot.json** -- one file per spike, written by daemon, queried by CLI

## Concurrency Model

**live-status.json**: Single writer (Go daemon), single reader (SwiftUI menu bar). Last-write-wins via temp file + `os.Rename` (atomic on APFS). No locking required.

**forensic snapshots**: Atomic write via temp+rename. Files are write-once, never modified. Directory: `~/.config/perf-guard/forensics/<timestamp>.json`.

## Forward Compatibility

- **Reader MUST ignore unknown fields.** New fields may be added without bumping schema_version.
- **Writer MUST increment `schema_version` on breaking changes** (field removal, type change, semantic change).
- **Reader shows "daemon too new"** if `schema_version > highest_known`.

## Rotation

Forensic snapshots: keep last 100 files, delete oldest. Configurable via `forensic.max_keep` in config.toml.

## Locations

| File | Path |
|------|------|
| Live status | `~/.config/perf-guard/live-status.json` |
| Forensic snapshots | `~/.config/perf-guard/forensics/<timestamp>.json` |
| Config | `~/.config/perf-guard/config.toml` |
| State | `~/.config/perf-guard/state.json` |

## Read-only session review (M1)

The additive [review v1 contract](review-v1/README.md) defines validated process,
Herdr, review-response and plan schemas, matching Go provider boundaries and
sanitized executable fixtures. It does not change live-status or forensic files.
