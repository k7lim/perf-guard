# Issue #6 handback

Issue: https://github.com/k7lim/perf-guard/issues/6
Branch: `m1/6-process`
Base and consumed contract: `ddf1b4436370325c58b9e5e7ad98d1d669e943c1`
Disposition: READY_FOR_REVIEW. The final handback supplies the tested HEAD; the
orchestrator records the accepted merge SHA.

Achieved: a read-only `process.NewReviewSource()` implementing the frozen
`review.ProcessSource`, with injectable `SnapshotSource`, `CWDSource` and clock.
`process.ReadIdentity(ctx, pid)` is available for #7/#8 correlation. Existing
sessions output and all process-management paths remain unchanged.

## Acceptance matrix

| Criterion | Evidence | Observed result |
|---|---|---|
| Actual OS PID/start identity, parent/TTY/cwd/age/RSS/time/provenance | `review_darwin.go`, `TestReviewGrouping`, `TestReviewHostSmoke` | PASS. Kernel `kern.proc.pid` start seconds+microseconds; age uses that fact, never creates identity from age. Unknowns remain null. |
| Wrappers and terminal-aware helpers; independent sessions preserved | `TestReviewGrouping`, `TestReviewFrozenInventoryFixture` | PASS. Nested agent through shell wrapper and no-TTY descendant grouped once; a new TTY stays independent; desktop/app-server agents never become terminal owners. |
| Unique membership and labeled RSS estimate | `TestReviewGrouping`, schema validation of emitted inventories | PASS. Every member appears once; group RSS is summed resident RSS with estimate=true; footprint/compressed/swap unknown. |
| Missing/reparented/disappearing/PID-reused process, inaccessible cwd, partial failure | `TestReviewChangedAndUnknown`, `TestReviewEmptyPartialAndFailure`, `TestReviewPartialSnapshotError` | PASS. Invalidated intermediate ancestry is not crossed; raw rows survive discovery error with nonnil error and partial status; empty success differs from failure. |
| Injectable providers and clock | `ReviewSource` seams, pure fixture tests | PASS without live Mac access; frozen process fixture consumed directly. |
| Bounded work and deadline | `TestReviewDeadline`, platform command contexts and per-cwd 200ms deadline | PASS. One overall maximum 5s context, at most four concurrent cwd helpers; timeout returns partial observations with explicit warnings. |
| Required fixture scenarios | Named tests above plus `TestReviewPSParsing` | PASS. Wrappers, independent TTYs, missing parents, duplicates, changed root/wrapper, PID reuse, malformed partial discovery, zero sessions and cancellation. |
| Read-only Mac smoke | opt-in `TestReviewHostSmoke` | PASS on Darwin 24.6.0 arm64, Go 1.26.1; stable self kernel identity, fresh source, 144 observations/77 groups in 2.861s. Only aggregate counts emitted. |

## Checks

All commands run from this worktree. Prefix used below:

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache
```

- Exact frozen validation: `GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/review -count=1 -v`: exit 0.
- `go test ./internal/process -run TestReview -count=1 -v` with prefix: exit 0; normal runs skip the explicitly opt-in host smoke.
- `go test -race ./internal/process -run TestReview -count=1` with prefix: exit 0, fixture-only.
- `PERF_GUARD_PROCESS_SMOKE=1` plus prefix and `go test ./internal/process -run TestReviewHostSmoke -count=1 -v`: exit 0 with authorized read-only host access.
- `go test ./...` and `go test -race ./...` with prefix: exit 0 with authorized host access for existing `ps` tests.
- `go vet ./...` with prefix: exit 0.
- `python3 scripts/validate-milestone-1.py`: exit 0.
- `git diff --check`: exit 0.

An earlier sequential-cwd smoke returned an honest partial inventory at 5.001s.
The final bounded-worker version completed fresh in 2.861s. The sandbox denies
`ps`; real-host checks require escalation, not an empty-inventory fallback.
No live daemon, installer, or legacy operational test scripts were executed.

## Artifacts and side effects

Sanitized fixtures are constructed in `review_source_test.go`, and tests consume
`contracts/review-v1/fixtures/process-inventory.json`. Every emitted pure-test
inventory validates against the accepted ProcessInventory JSON schema. No private
runtime identifiers, cwd, arguments, resume data or screenshots were saved.

Recording fakes record only `snapshot` and `cwd` calls; the allowed call sequence
is asserted. Their boundaries expose no input/close/signal/start/stop/update/
install operations. Production-path inspection: `ReviewSource` never calls
`Manager`, `Suspend`, `Resume`, `unix.Kill` or Herdr. The Darwin adapter only uses
kernel reads, `/bin/ps` and `/usr/sbin/lsof`. Context cancellation can stop only
these newly created bounded helper probes, never an existing user process.
No daemon mode, host configuration, login item or installed binary was changed.

## Integration and limits

Only new process-provider/test files and this handback are owned here. No shared
contract amendments or CLI/Swift/Herdr edits. #8 wires `NewReviewSource()`;
#7/#8 can inject `ReadIdentity` as an identity resolver. Provider fakes must honor
context cancellation; cwd fakes must support up to four concurrent calls.

The snapshot brackets ps with kernel identity and parent reads, and cwd lookup
checks the expected identity and parent before and after probing. This detects
observed PID reuse/reparenting; M1 cannot make live observations atomic. Later
plan collection must revalidate identity as required by the frozen contract.
Unrecognized executables are only included when descended from an identified
terminal codex/claude owner. Other resource-owner categories remain out of scope.
Non-macOS production providers explicitly return unsupported errors; pure tests
use injected seams. Unknown cwd or denied optional probes remain partial data.

Suggested next task: after orchestrator acceptance, merge and push #6. Start #8
only after #7 is also accepted, merged and pushed.
