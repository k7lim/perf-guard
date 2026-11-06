# M1 review contract v1

Contract owner: #10. Process producer: #6. Herdr producer: #7. Reconciliation,
worktree probes and CLI producer: #8. Swift is a consumer (#11), never a second
eligibility engine. #12 accepts integrated behavior. Amendments return to #10's
owner and are accepted by the orchestrator before dependent edits.

## Executable validation

From the repository root:

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/review -count=1 -v
```

This compiles Draft 2020-12 JSON schemas with format assertions, validates all
manifest fixtures, decodes/re-encodes each through the matching Go DTO and
revalidates with structural equality. Negative cases falsify required fields,
versions, enums, timestamps, negative RSS, identity-less reviewability, executable
plans and shell loss. It needs Go 1.26.1, no Python packages or running Herdr.
The test-only schema dependency is pinned in go.mod/go.sum. No host installation.
`types.schema.json` is the shared definition source; the four root schemas refer
to it. `internal/review/types.go` is the matching Go boundary. Fixtures are
synthetic `/example` data, not private host inventory.

## Fields and compatibility

Every property listed under `required` is required on the wire, including nullable
properties. No Go `omitempty` is used. JSON `null` means not observed/unavailable;
`0` is a measured zero; `[]` is a successfully represented empty collection.
A failed source MUST NOT become a fresh empty inventory. Objects accept unknown
properties. Readers ignore additive fields, but reject a schema version greater
than 1 with visible incompatible-version error. Breaking removals, type changes
or semantic changes increment the version. v1 writers emit every defined field.

Timestamps are RFC3339 instants. Provider observation times are distinct from the
review's collection completion time. Each source has status and provenance;
process fields have `field_sources` keyed by JSON field name. Missing timestamps
stay null. Nullable memory values remain unknown unless actually measured.
`resident_rss_bytes` is resident memory, never total memory. Every group RSS has
`summed_rss_is_estimate=true`; it sums members once and is not unique physical RAM.
A process can belong to at most one group; root is included among members.
`owner` is the root process identity. All members, including intervening wrappers,
have actual OS identities where available; inaccessible/disappeared identities
remain null with warnings, never fabricated from `now - elapsed`.

Process age, user activity, conversation activity and lifecycle state are separate
facts. The two activity properties are nullable and may only carry corresponding
explicit evidence. Neither process age nor terminal redraw/output revisions can
populate activity. Child-work state is `working`, `none_observed`, or `unknown`;
absence of supported evidence is `unknown`, not `none_observed`.

## Identity, grouping and duplicate inputs

`ProcessIdentity` is PID plus actual OS start identity (opaque stable string with
source documented by provider). A PID alone is not a positive correlation.
`OccupantIdentity` includes endpoint, server/session, workspace/tab/pane, occupant
revision, and PID/start identity. `ReviewID` in Go hashes an unambiguous JSON array
of precisely those fields into `review-v1-<sha256>`. Same pane IDs on two servers
cannot collide. Any occupant replacement, revision change, or PID reuse changes
the review ID. Never resolve selections by pane ID, PID, row position or project.

`revision` means stable occupant generation, NOT terminal output revision or
lifecycle update sequence. A provider may derive generation from positively
correlated actual PID/start identity within the full scope when no native occupant
generation exists. It must document that derivation. If it cannot prove identity,
revision/process remain null and correlation is unknown/ambiguous. Positive
correlation requires all scope/generation fields and an exact OS identity match.

Process-only records and uncorrelated Herdr records remain visible but excluded.
Their IDs use `ExcludedID` and cannot become reviewable. For process-only records
use kind `process` and components decimal PID, actual start_id or empty string.
For uncorrelated panes use kind `herdr` and components endpoint, server_id,
session_id, workspace_id, tab_id, pane_id (in that order). These display IDs may
survive a refresh but never preserve reviewable selection across correlation.

Duplicate scoped occupant identities are fail-closed: collapse byte-identical
observations to one visible record with `reviewable=false`, reason
`duplicate_identity` and a warning reporting the collision; conflicting duplicate
observations also collapse to one excluded record without choosing a winner.
Keep common identity fields only; conflicted observations become unknown.
Distinct revisions/process starts for the same scoped pane are ambiguous: emit
excluded records, never choose newest by guess. Distinct servers are independent.
`total` counts resulting visible records, not raw duplicate input rows.

## Collection, freshness and failure

`Clock.Now()` is injectable. Each list/plan invocation creates ONE overall
five-second context deadline shared by process, Herdr and worktree collection;
subprocesses/lookups must honor cancellation. Providers are read-only collection
seams. No method may send input, signal a user process, close a pane/session,
start/stop/update a service or install anything. A bounded helper probe may be
started and cancelled; this is not permission to control existing processes.

Providers return partial inventories plus warnings when optional probes fail.
An error return denotes source-level failure; consumers retain any returned
partial observations but mark unavailable/partial rather than healthy. A required
process discovery failure fails the CLI envelope (exit 1), never zero sessions.
An optional Herdr outage, unsupported capability, cwd/worktree lookup failure or
deadline returns success with partial records and explicit warnings. A denied
explicit target is reported as unavailable; do not fall back to a focused session.
Empty successful process inventory is a valid success. Warnings use stable codes
such as `deadline_exceeded`, `process_disappeared`, `cwd_inaccessible`,
`partial_discovery`, `unsupported_capability`, `duplicate_identity`,
`source_unavailable`, `probe_failed`, `future_observation`.

Default freshness is 30 seconds at the single injected collection clock time.
A source or required per-process/lifecycle observation is fresh only when its
age is in [0,30s]. Future timestamps are unknown/stale and produce
`future_observation`; do not clamp into freshness. Null observation time cannot
prove freshness. Provider source `partial` may retain usable fresh individual
observations, but `stale`/`unavailable` cannot. Process and lifecycle observations
must each be fresh before reviewability. Ancillary missing cwd/activity/resume or
worktree state does not alone exclude review, and is never replaced by certainty.

`reviewable=true` requires fresh positive exact identity correlation and lifecycle
idle/done. Reviewable means suitable for user assessment; it says nothing about
abandonment or authorization to exit. Reasons: `fresh_idle`, `fresh_done`.
Exclusions (all applicable, lexicographically sorted): `working`, `blocked`,
`unknown_state`, `stale_source`, `uncorrelated`, `current_controller`,
`identity_unavailable`, `duplicate_identity`, `child_working`. When controller
identity is known, its owner group is excluded; producer must surface inability to
identify controller rather than claim certainty. Lifecycle working/blocked,
unknown correlation, duplicate identity and known child work always exclude.

## CLI and envelope

Freeze this additive CLI surface for #8 (not implemented by #10):

```sh
perf-guard review --output json [--limit 50] [--offset 0] [--herdr-endpoint ENDPOINT --herdr-session SESSION]
perf-guard review plan --output json --session ID [--session ID ...] [--herdr-endpoint ENDPOINT --herdr-session SESSION]
```

Endpoint/session flags select an explicit target. Supported inherited caller
context may provide both; otherwise return Herdr unavailable with an explanatory
warning and keep process-only observations. Never use UI focus or guess named
sessions. For an unavailable target use endpoint/session literal `unavailable`
in response target (these are labels, never an endpoint to connect to).
Only the provider resolves inherited context; document actual environment names
when implementing #7. Client/server versions remain separate. No restarts,
upgrades, authentication changes or host install are implied.

Both commands are sensors. `plan` is always a dry-run; it has no apply/exit mode
or execution capability. No `--dry-run` is needed for a read-only sensor.
Limit defaults 50, range 1..200; offset >=0. Sort all records lexicographically
by ID before pagination. Totals count all records and all reviewable records
before slicing. Sources sort by source name, warnings by source/code/message,
reason sets sort lexicographically. Plan always recollects both sources under a
new deadline and examines the full inventory, independent of pagination.

Repeatable `--session` IDs bind the displayed exact identities. Keep request
order and return one result per occurrence. First occurrence: `reviewable` or
`excluded`, with the fresh record snapshot and reasons. Missing or changed ID:
`missing`, record=null, reason `selection_missing_or_changed`. Repeated ID after
its first occurrence: `duplicate`, record=null, reason `duplicate_selection`.
Do not substitute the current occupant of an old pane. Missing/excluded/duplicate
selections are valid plan results (exit 0), not a transport error. No selections,
invalid flags/pagination, control characters in IDs or invalid target syntax:
error envelope and exit 1. A plan with no reviewable selections is still useful.
Recovery availability is true only for a nonempty native resume reference in the
fresh record; it is recovery context, not rollback of an exit. Every selection
has `preserve_shell=true`. Every M1 plan has `execution_supported=false`.

Use the existing output.Response/Printer envelope unchanged:
`command` = `review` or `review plan`, `success`, optional `error` (omitted or
empty on success), `data`, `timestamp`. Success has typed data and exit 0;
failure has nonempty error, data=null and exit 1. No new envelope shape.
Fixture files contain data payloads for reuse in Go and Swift; #8 wraps them.

## Fixture map

- `healthy-review`: idle/done/working/blocked, unknown activity/resume/memory.
- `empty-review`: successful empty inventory.
- `stale-partial-review`: stale lifecycle/source, optional deadline warning.
- `unavailable-herdr-review`: missing target, inaccessible cwd, unknown worktree.
- `dirty-review`: dirty worktree is visible and not silently treated as clean.
- `changed-occupant-review`: same pane/PID, new start identity and generation.
- `two-servers-review`: same public IDs in independent scopes.
- `duplicate-scoped-review`: collision explicitly excluded and warned.
- `example-plan`: accepted review, missing/changed, duplicate and excluded IDs.
- `process-inventory`: wrappers/helpers, independent TTYs, missing parent.
- `partial-process-inventory`: disappeared identity, partial discovery.
- `herdr-inventory`: distinct versions, supported-read capability metadata.
- `unknown-herdr-inventory`: unsupported correlation is explicitly unknown.

The example versions are synthetic contract fixtures, not an advertised Herdr
compatibility range; #7 must characterize and document real supported versions.
