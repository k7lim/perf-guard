# Issue #8 handback

Issue: https://github.com/k7lim/perf-guard/issues/8
Branch: `m1/8-review`
Base: `84b9548e865bea308eff4350af0df4ef2d81af8f`
Disposition: READY_FOR_REVIEW. The tested HEAD accompanies the handback; the
orchestrator records the accepted merged SHA separately.

Achieved: shared read-only reconciliation/policy, bounded worktree inspection,
CLI `review` and `review plan`, unchanged JSON envelope, schema/help/docs and
fixture/executable/real-host verification. Every plan has no execution capability.
No session exits, signals to user processes, Herdr controls, daemon mode changes,
installation, login items, producer edits or Swift policy were added.

Contract: v1 from accepted #10 `ddf1b4436370325c58b9e5e7ad98d1d669e943c1`.
Providers: accepted #6 `85e8729b8d4fef40a874cdb3f663427792525169` and #7
`39130523bf880ff13d03e1a93667cd01793e1f63`, merged at base `84b9548`.
No schema, DTO or producer-interface amendment. #11 continues to consume the
same flags, response, reason codes and plan fields.

## Acceptance matrix

| Criterion | Evidence | Observed result |
|---|---|---|
| Frozen commands, envelope, pagination, totals, order and failures | `TestReviewCLIEnvelopePlanAndErrors`, `TestReviewHelpAndDescribe`, `TestServiceFrozenHealthyAndPagination`; actual CLI smoke | PASS. Successful data validates against v1 schemas; parse/flag/argument failures emit error envelope and exit 1. Totals precede slicing; records sort by exact ID. |
| Exact positive joins; PID/occupant replacements | `TestServiceExactIdentityAndDuplicates`, `TestServicePlanSelectionsRecollect` | PASS. Same PID/new start never joins; revision change returns missing for old selected ID. Helpers do not become independent terminal owners. |
| Working/blocked/unknown/stale/uncorrelated/current exclusions | `TestServiceExclusionsAndFreshness`, grouping/partial tests | PASS. Fresh idle/done alone can be reviewed. Controller identity/owner is verified through inventory; unavailable controller context has explicit warning. |
| Sourced age/activity, unknown distinct | `TestServiceSharedFixtureActivity`, frozen healthy fixtures, typed DTOs | PASS. Age remains age; user/conversation provenance stays separate; missing activity is null; impossible future activity becomes unknown with warning. |
| Bounded read-only cwd/worktree probes | `TestWorktreeReadOnlyActualGit`, `TestWorktreeDirtySubmoduleReadOnly`, `TestWorktreeFailureDeadlineAndInheritedGitScope` | PASS. Clean/dirty/untracked/submodule/non-repository/inaccessible/failure/timeout covered; hashes of all repository files including index bytes remain identical. |
| Fresh exact per-selection plans/recovery/shell intent | `TestServicePlanSelectionsRecollect`, schema validation, integrated host test | PASS. Full recollection for each plan, per-occurrence results, recovery only from native reference, shell preserved, execution_supported always false. |
| Missing/excluded/repeated/stale selections | Plan/exclusion and executable tests | PASS. Request order preserved; changed IDs missing; repeated occurrence duplicate; excluded snapshots retained; no pane substitution. |
| Fixtures and stale/partial behavior shared with Swift | Frozen v1 fixtures, service fixture tests, typed wire serialization and schema assertions | PASS. Same sanitized response/plan shapes consumed; future/stale/partial/empty/dirty/changed/duplicate cases verified without live Herdr. |
| Zero forbidden control calls and production-path inspection | Recording providers with collect/inspect-only assertions; production construction in `review.go`; producer allowlist tests retained | PASS. No input/close/signal/start/stop/update/install capability accepted by service. Only OS reads, three allowlisted Herdr reads and bounded Git/version helpers are wired. |
| Implemented help/schema/docs; existing behavior | `TestReviewHelpAndDescribe`, executable tests, full Go suites, README and service README | PASS. Describe adds review/plan and dedicated target env; existing envelopes/commands remain compatible. |

## Checks and environment

Darwin 24.6.0 arm64, Go 1.26.1. Use this environment prefix for all Go commands:

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache
```

Exact frozen schema/fixture validation, exit 0:

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/review -count=1 -v
```

- `go test ./cmd/perf-guard ./internal/review -count=1` with prefix: exit 0, pure fixtures/executable error tests and temporary Git repositories. Opt-in host test skips normally.
- `go test ./...` with prefix: exit 0 with authorized read-only host access for existing ps tests.
- `go test -race ./...` with prefix: exit 0 with authorized read-only host access.
- `go vet ./...` with prefix: exit 0.
- `python3 scripts/validate-milestone-1.py`: exit 0.
- `git diff --check`: exit 0.

Real host integration command, exit 0:

```sh
PERF_GUARD_REVIEW_SMOKE_TARGET_FILE=/private/tmp/perf-guard-m1-7-target-private.json GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./cmd/perf-guard -run TestReviewReadOnlyHostSmoke -count=1 -v
```

Observed client 0.9.5 / server 0.9.0 / protocol22; `ping`, `session.snapshot`,
`pane.process_info` supported. Integrated platform resolver positively correlated
45 identities. Review showed 110 visible records and 43 reviewable in the tested
sample; a fresh exact selected plan returned reviewable with execution false.
List+plan together took 6.289s (each invocation has its own five-second maximum).
Counts are live observations, not test expectations. Herdr source partial retains
wrapper-only unknowns; process source fresh. No runtime identifiers are emitted.

A separately built CLI was exercised via real OS subprocesses for list and plan:
exit 0/success true, 110 visible, 45 identity-bearing records and 42 reviewable
at that observation; a fresh selected plan was reviewable/execution false.
Private JSON artifacts remain `/private/tmp/perf-guard-m1-8-review-private.json`
and `/private/tmp/perf-guard-m1-8-plan-private.json`. These are not committed.
The build `/private/tmp/perf-guard-m1-8-cli` is local acceptance material, not an
installed host binary. #12 must rebuild and verify the final merged SHA.

## Side effects and production path

Service seams expose collection/inspection only. Recording fake calls are only
`collect`/`inspect`; their allowed sequence is asserted. Source review confirms
no process manager import or Herdr control adapter in the service. Production
construction is `process.NewReviewSource()`, `herdr.NewSource(process.ReadIdentity)`
and `review.GitWorktreeSource`; existing daemon/installer code is untouched.

Git probes use `--no-optional-locks`, disable fsmonitor/untracked cache/hooks,
remove inherited Git overrides, and inspect submodule dirtiness. Retained output
is capped at 64 KiB; each probe is 250ms maximum, four concurrent probes maximum.
No uncancellable stat lookup precedes the bounded helper. Temporary fixture
setup writes only test repositories, while production probes preserve files and
index bytes. Context cancellation affects only helpers created for that read.

One five-second overall deadline encloses concurrent providers and optional
worktree probes. Earlier provider deadlines (max four seconds, 80% of remaining
parent time) leave partial-result return/probe margin; cooperative optional cwd
expiry is not misreported as required discovery failure. Required discovery
errors still fail the envelope. The deadline/partial regression test verifies it.

## Integration and limits

Owned changes: new service/worktree files and tests in `internal/review/`, CLI
review/wiring/error handling/tests in `cmd/perf-guard/`, README/service docs and
this SITREP. `types.go`, schemas, fixtures, process/Herdr producers, Swift and
host configuration remain unchanged. Root construction was extracted to an
unexported helper so command/help tests can exercise actual wiring; the legacy
non-review error behavior stays unchanged.

Provider fakes must honor cancellation as required by the frozen contract.
Observations cannot be atomic across a live host; each plan revalidates by fresh
collection, and M1 never executes. Wrapper-only Herdr layouts remain unknown,
per #7, rather than becoming guessed correlations. Current controller cannot
always be established from a GUI caller, so a visible warning records that limit.
Missing child activity remains unknown; no transcript or process-activity heuristic
is introduced. Real-host tests require permissions the restricted sandbox may
deny; denied access is a failure, never zero healthy sessions.

The user-facing menu was implemented in #11; this ticket does not claim UI
visual acceptance or milestone completion. Orchestrator owns merge, push and
closure. Suggested next task: accept/merge/push #8, then dispatch independent #12
only after #11 is also accepted, merged and pushed.
