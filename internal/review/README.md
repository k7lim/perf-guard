# Read-only review service (#8)

`Service` consumes the frozen [v1 boundary](../../contracts/review-v1/README.md).
Production wiring lives only in `cmd/perf-guard/review.go`: process inventory,
Herdr with actual `process.ReadIdentity`, and `GitWorktreeSource`. No process
manager or Herdr control surface is accepted by the service.

List and plan concurrently collect both sources under one five-second overall
context. Providers receive an earlier deadline (maximum four seconds, 80% of
remaining caller time), leaving time for optional probes to return partial data
and bounded worktree inspection. A required process failure is an error; an
optional Herdr/probe outage preserves partial observations and warnings. The
collector must honor context. A source's own earlier deadline never extends the
overall bound. Worktree probes are deduplicated by cwd, use at most four workers,
and each has a 250ms maximum within that same overall context.

Only positively correlated Herdr PID/start identities matching a terminal root
are joined. A helper, same PID with a changed start, similar cwd, TTY or process
age cannot establish a session. Unknown panes and uncorrelated terminal roots
remain explicit excluded records. Desktop/app-server processes are not separate
terminal sessions. Group RSS is the producer's resident sum estimate; other
memory values stay nullable. Source and process/lifecycle times are checked at
one injected clock time against the frozen 30-second freshness rule. Future
required observations are excluded with provenance/warnings. Missing activity
stays unknown; explicit activity facts never derive from age or terminal output.

Duplicate panes, conflicting generations, or a process correlated to multiple
panes are excluded before selection. Visible ID collisions are collapsed with an
explicit warning; conflicting facts become unknown, common facts survive. No
input order chooses a winner. Records, reasons, sources and warnings have stable
ordering; pagination follows reconciliation and totals precede slicing.

The current CLI helper's verified inventory identity/owner identifies the
controlling terminal group when possible. An injected exact controller identity
is supported for native caller context and tests. Inability to establish that
identity is a visible `controller_unknown` warning, never a claim of certainty.
Known current groups are excluded. Known fresh, positively correlated working
members provide child-work evidence; missing evidence remains unknown.

`GitWorktreeSource` runs only `rev-parse --is-inside-work-tree` and porcelain
`status` with optional locks, fsmonitor, untracked cache and hooks disabled.
Inherited `GIT_*` overrides are removed so the cwd, worktree and index cannot be
retargeted. Dirty tracked files, untracked files and dirty submodules stay visible.
Tests hash every fixture repository file including `.git/index` before/after.
No checkout, stash, add, index refresh, cleanup or repository write occurs in the
production probe. Timeout, access failure and non-repository have distinct results.
Only newly created bounded helper processes may be cancelled.

Every plan recollects the full inventory independently of pagination. IDs bind
the full exact occupant identity, and a changed occupant returns `missing` with
`selection_missing_or_changed`. Request order and duplicate occurrences survive.
Excluded and missing selections are useful success results. Every result preserves
the shell and every plan reports `execution_supported=false`. Recovery references
are local context, not rollback. There is no apply or exit operation.

## CLI

```sh
perf-guard review --output json --limit 50 --offset 0 \
  --herdr-endpoint /absolute/herdr.sock --herdr-session explicit-session
perf-guard review plan --output json --session REVIEW_ID --session OTHER_ID \
  --herdr-endpoint /absolute/herdr.sock --herdr-session explicit-session
```

Both target flags must be supplied together. When absent, only the dedicated
inherited pair `PERF_GUARD_HERDR_ENDPOINT` / `PERF_GUARD_HERDR_SESSION` is used.
Missing context yields process-only exclusions and a Herdr-unavailable warning;
there is no discovery or focused-session fallback. Only absolute local Unix
endpoints are supported by the characterized adapter. Its supported versions and
wrapper-only correlation limit remain documented in `internal/herdr/README.md`.

The existing envelope is unchanged: command `review` / `review plan`, success
plus typed data and exit 0; failure with nonempty error, null data and exit 1.
Review argument/flag parsing failures use that envelope too. `--limit` is 1..200,
default 50, offset is nonnegative. Plan needs at least one nonempty exact ID;
control characters are rejected. Missing, repeated, stale or excluded selections
are not transport failures. `describe` and command help expose this implemented
sensor surface. Existing commands keep their behavior.

## Checks

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/review -count=1 -v
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./cmd/perf-guard -run TestReview -count=1 -v
```

Pure service and executable-envelope tests use sanitized fixtures. The opt-in
`TestReviewReadOnlyHostSmoke` requires `PERF_GUARD_REVIEW_SMOKE_TARGET_FILE` pointing
to a private JSON object with `endpoint` and `session_id`; it never discovers the
target. It reports versions, capabilities and counts without runtime identities.
The Mac's sandbox can deny kernel/ps access; real-host checks must run with
suitable read-only permissions and must not pass an unavailable host as empty.
