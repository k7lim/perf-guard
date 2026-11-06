# Issue #12 independent acceptance

Issue: https://github.com/k7lim/perf-guard/issues/12
Branch: `m1/12-acceptance`
Tested merged production base: `086f7dfad11bd8910f64e195009bef2ceff28ea0`.
Acceptance head: supplied in final handback; only tests and documentation differ
from that production base. Orchestrator owns merged-head verification and closure.
Disposition: READY_FOR_REVIEW, subject to orchestrator acceptance.

Onboarding completed from a clean isolated worktree: AGENTS.md, engineering
principles, M1 guide, issue #12, frozen v1 README and all dependency SITREPs read.
Input contract is accepted #10 `ddf1b44`; dependencies are accepted #6 `85e8729`,
#7 `3913052` / merge `84b9548`, #11 `53fe9e4` / merge `6ab41c4`, and #8
`b831a5f` / merge `086f7df`. No amendment or production policy change.

Deliverable: independent acceptance evidence plus a fixture CLI-to-Swift bridge
and stronger live visual harness. No implementation feature was owned here.

## Complete criterion matrix

| #12 criterion | Evidence | Observed result |
|---|---|---|
| Frozen validation, full Go/race/static, macOS Swift build on merged source | Commands below; production base above | PASS, exit 0. Host-authorized process reads, no empty-inventory substitution. |
| Healthy, empty, missing/incompatible/disconnected Herdr, partial discovery, stale, changed occupant/PID reuse, unknown activity, dirty cwd, missing recovery | Named scenario matrix below; existing suites inspected independently | PASS; all requested cases asserted, not merely fixture decoding. |
| CLI and Swift same fixture records/reasons/selections | `bash scripts/test-m1-acceptance.sh`; Go CLI child uses existing frozen fixture providers and real command/envelope path; production Swift model consumes those envelopes | PASS: ordered exact IDs, lifecycle, reviewability, reasons, two explicit selected identities, repeated exact flags and fresh selected plan identities/reasons agree. No second policy engine. |
| Stale/changed identity invalidation; no execution | `TestServicePlanSelectionsRecollect`, `TestServiceExactIdentityAndDuplicates`; Swift matching refresh/replacement/error/executable-plan rejection cases | PASS: old IDs missing, replacements never substituted; changed or stale rows clear selection/plan; all emitted plans false; executable plans rejected. |
| Zero forbidden controls, no repository writes | Recording process/Herdr/worktree fakes, `TestTransportRejectsControlBeforeDial`, production path inspection below, `TestWorktreeReadOnlyActualGit` and dirty submodule test | PASS: only collection/inspection and three allowed Herdr reads; forbidden sequences empty; files and Git index bytes unchanged. |
| Bounded scoped real-host smoke, versions/capabilities | Opt-in integrated host command below, explicit private target, newly built CLI in live native harness | PASS: client 0.9.5 / server 0.9.0 / protocol 22; ping, session.snapshot, pane.process_info. Positive OS identities; fresh selected read-only plan. |
| Personally inspect fixture menu and real review/plan | Native `NSHostingView` fixture PNGs; private real viewport and scrolled exact production view | PASS: reasons, unknowns, sourced age/RSS, exclusions and plan false/shell/recovery/identity content readable. Long inventories require scrolling; harness navigates actual model pages. Private live evidence never committed. |
| Monitor-only unchanged; no exits or broader resource work | Read-only live status, deployment SHA-256 comparison, diff and production-path inspection | PASS: monitor_only true, daemon stale false, deployment hash unchanged. Only acceptance tests/docs changed. No installation or live daemon/app launch. |
| Fresh-worktree onboarding; host-only gates/Beads exception | Clean isolated worktree at exact accepted base; fresh committed-head check described in handback | PASS: temporary Go/Swift cache commands work. macOS/Xcode/process permissions/authorized Herdr required for host gates. Per-command hooks exception only; no bd migration. |
| Complete evidence, defects and exact SHA | This matrix, command logs retained locally, final head in handback | PASS: exact tested production base above. No production defects found; live first-page-only and off-viewport capture gaps corrected in test harness. Root must rerun merged gates before closure. |

## Scenario evidence

| Scenario | Named tests/harness assertions |
|---|---|
| Healthy idle/done/working/blocked, empty | `TestServiceFrozenHealthyAndPagination`, `TestServiceEmptyValidationAndOptionalDeadline`, Swift healthy/empty/all-excluded |
| Missing/incompatible/disconnected Herdr | `TestMissingIncompatibleEmptyAndCancellation`, socket failure tests, `TestServiceEmptyValidationAndOptionalDeadline`, Swift unavailable/error/newer-schema |
| Partial process failure/disappearance | `TestReviewEmptyPartialAndFailure`, `TestReviewPartialSnapshotError`, `TestServiceChildWorkPartialAndUnknowns`, required CLI error-envelope assertion |
| Stale/future/unknown observations | `TestServiceExclusionsAndFreshness`; Swift stale fixture and error clears selection/plan |
| Changed occupant, PID reuse, duplicates, two servers | `TestServiceExactIdentityAndDuplicates`, `TestServicePlanSelectionsRecollect`, `TestChangedIdentityAndUnknownCorrelation`; Swift changed-occupant/matching refresh/identity rejection |
| Unknown age/activity/memory, no redraw inference | `TestServiceSharedFixtureActivity`, `TestContractFactsStayDistinct`; Swift separate unknown activity and compressed-memory assertions |
| Dirty/inaccessible cwd, preserve repository/index | `TestWorktreeReadOnlyActualGit`, `TestWorktreeDirtySubmoduleReadOnly`, `TestWorktreeFailureDeadlineAndInheritedGitScope`; Swift dirty/unknown state |
| Missing native resume | Frozen healthy idle null reference, plan recovery false; `TestServicePlanSelectionsRecollect`; native fixture/live plan says unavailable |
| Read-only selection outcomes and shells | Example plan statuses reviewable/missing/duplicate/excluded; service recollection tests, schema negatives, CLI errors and Swift acceptance/rejection tests |

## Commands and environment

macOS 15.7.7 (24G720), Darwin 24.6.0 arm64; Go 1.26.1; Apple Swift 6.2.4.
Every Go command below uses exactly:

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache
```

Exact frozen validation (exit 0):

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/review -count=1 -v
```

Other gates, each exit 0:

```sh
# Apply the Go environment above to each Go invocation.
go test ./...
go test -race ./...
go vet ./...
python3 scripts/validate-milestone-1.py
bash -n menubar/build.sh menubar/install.sh menubar/test-review.sh scripts/test-m1-acceptance.sh
bash menubar/test-review.sh
bash scripts/test-m1-acceptance.sh
bash menubar/test-review.sh --render /private/tmp/m1-12-fixture-visual
CLANG_MODULE_CACHE_PATH=/tmp/perf-guard-swift-module-cache SWIFT_MODULECACHE_PATH=/tmp/perf-guard-swift-module-cache GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache bash menubar/build.sh
git diff --check
```

Full normal/race gates require authorized read-only host `ps` access; vet and
fixture/contract checks do not depend on live Herdr. Default host smokes skip;
the explicitly enabled command below ran and passed separately, not waived.

```sh
PERF_GUARD_REVIEW_SMOKE_TARGET_FILE=/private/tmp/perf-guard-m1-7-target-private.json GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./cmd/perf-guard -run TestReviewReadOnlyHostSmoke -count=1 -v
```

Observed 45 positive identities, 110 visible and 44 reviewable records; selected
plan reviewable, execution false, list+plan 6.384s (separate five-second collection
budgets). Counts are changing observations, never fixed expectations. Herdr partial
includes deliberately uncorrelated wrapper-only occupants, process source fresh.
Real native harness uses the freshly built worktree CLI, with private target
loaded by a local Python subprocess wrapper, then actual pagination, exact
selection and requestPlan. Invocation shape is documented in menubar/Tests;
private endpoint/session arguments are deliberately omitted here.

## Visuals, side effects and limits

Personally inspected native healthy and selected-plan fixture PNGs; read-only
badge, readable native checkboxes, disabled exclusions, distinct unknown activity,
resident RSS, dry-run false, shell preservation and unavailable recovery all
visible. Also inspected live review and scrolled plan from the exact production
ReviewView. Evidence stays in a private temporary directory; no runtime inventory,
cwd, resume context, endpoint or live screenshot is published. Fixture artifacts
are reproducible; existing sanitized #11 PNGs remain available in the repository.

Production wiring inspected: `cmd/perf-guard/review.go` constructs only
`process.NewReviewSource()`, `herdr.NewSource(process.ReadIdentity)` and
`review.GitWorktreeSource`. Process collection reads kernel start identity,
`ps` and bounded `lsof`. Socket transport allows only three fixed read methods;
Herdr CLI is used solely for bounded `--version`. Service has no manager/control
capability. Git probes disable optional locks/hooks/fsmonitor, strip inherited
Git overrides and call only rev-parse/status. Swift starts only its own describe/
review helper and timeout-cancels that helper, never an existing user process.
Recording fakes assert no input, close, signal, service start/update/stop or
install. No legacy live-daemon or installation test scripts were executed.

Live observations cannot be atomic; plans recollect identities and M1 never
executes. A GUI caller can have unknown controller identity, surfaced explicitly
by warning rather than inferred. Unsupported wrapper layouts stay excluded.
These documented limits are visible behavior, not waived prerequisite checks.

No production defects or remaining implementation work found. Acceptance branch
does not merge, push, close #12 or close M1. The orchestrator personally reviews,
reruns critical cases on its merged SHA, pushes, verifies tracking state and owns
the final decision. Temporary legacy Beads exception is per-command
`git -c core.hooksPath=/dev/null`; no global/repository setting is changed.

Committed acceptance check point: `6c0535c9857aa4acc24cf5846619630d1abb4825`
passed frozen validation, full Go normal/race suites, vet, parity bridge, full
Swift build and opt-in live smoke. A second clean detached worktree at that SHA
passed frozen tests, the complete Swift harness (including stale/changed existing
plan invalidation), graph validation and clean status. Final documentation-only
head is supplied in the handback, with critical gates rerun there.

Development failures were confined to the new acceptance harness: its initial
CLI child launch used the wrong fixture-relative cwd, and an initial live-plan
capture attempted to reference a private Swift view type. Both were corrected in
test code before the committed checks; production source stayed unchanged. The
first scrolled capture clipped plan details due to lazy layout; final native
scroll-settled capture was regenerated and personally inspected by worker and
orchestrator. Default optional live smokes skip by design in full suites; the
explicit target smoke and native live gate ran separately and passed. No final
required gate is failed or blocked.
