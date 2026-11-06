# Issue #10 handback

Issue: https://github.com/k7lim/perf-guard/issues/10
Branch: `m1/10-contract`
Base: `9da4fd0a9a7e4043f33641b0b3ee0373fd834152`
Disposition: READY_FOR_REVIEW. Head SHA accompanies the handback; the orchestrator
records the accepted merge SHA after independent verification.

Achieved: additive v1 schemas, matching Go DTO/provider interfaces, deterministic
identity IDs, 13 sanitized manifest fixtures, schema/serialization/negative tests,
and the exact consumer contract. No production collector, reconciliation, CLI
registration, menu, process control, daemon or installer changes.

Contract: this issue owns `contracts/review-v1/` and `internal/review/`. No upstream
M1 dependency. Amendments return to this owner and require orchestrator acceptance.

## Acceptance matrix

| Criterion | Evidence | Result |
|---|---|---|
| Schemas and Go round-trip same fixtures | `TestContractFixtures`, 13 manifest entries, JSON Schema Draft 2020-12 compiler with date-time format assertions | PASS |
| Healthy idle/done/working/blocked, unavailable/stale/partial/duplicate/changed/PID reuse/inaccessible cwd/missing resume/dirty/empty | `TestNamedScenarioEvidence`, `TestContractFactsStayDistinct`, named fixtures documented in contract README | PASS; fixture semantics asserted rather than only counted |
| Cross-server scope and activity facts | `TestReviewIDBindsEveryIdentityComponent`, two-server fixture, separate typed user/conversation observations, age remains separate | PASS |
| Resident RSS label, aggregate estimate, unknown extra memory | `MemoryObservation`, process groups, `TestContractFactsStayDistinct` | PASS |
| Fresh positive idle/done means reviewable, not abandonment/exit permission | Contract README freshness/eligibility section, healthy fixtures, schema rejects identity-null reviewability | PASS at contract level; production policy belongs to #8 |
| Required/optional/ownership/compatibility/failure semantics | Schema required lists and nullable anyOf, contract README, additive-field test, invalid-schema cases | PASS |
| Sanitized review and plan published | `contracts/review-v1/fixtures/healthy-review.json` and `example-plan.json` | PASS; all synthetic `/example` values |
| Commit before dispatch, amendment owner | Worker handback HEAD and orchestrator merge gate | Worker commits before handback; orchestrator controls acceptance/dispatch |

## Checks and environment

Darwin 24.6.0 arm64, Go 1.26.1. All contract checks are fixture-only, require no
Herdr or private runtime data and perform no host configuration writes.

Exact schema/fixture validation command (exit 0):

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/review -count=1 -v
```

The same environment prefix was used for `go test ./...`, `go test -race ./...`
and `go vet ./...`. Full normal and race gates exit 0 with read-only host access.
Initial sandbox full normal/race attempts fail only existing `TestTopByRSS`
because `/bin/ps` is denied; this is not represented as zero processes or waived.
`go vet ./...`, `python3 scripts/validate-milestone-1.py`, and `git diff --check`
exit 0. No legacy live-daemon/UI smoke scripts were run.

Artifacts: committed schemas, fixtures, test source and contract README. There is
no private runtime inventory or screenshot in this task. Validation rejects nine
mutated invalid payload classes, including execution_supported=true.

Side effects: no production adapters exist in this ticket, so no live control
transport is invoked. Reflection checks assert provider interfaces have exactly
one collection/inspection method and no control methods. Downstream must also
supply recording transport fakes and inspect real production wiring; this
contract check does not claim to prove unimplemented adapters are read-only.

Integration: new boundary files only, plus additive contracts index link and
pinned test schema validator dependencies in go.mod/go.sum. Existing commands and
output envelope untouched. No conflicts observed. Orchestrator owns merging and
pushing; worker does not close issues or publish source independently.

Limitations: contract fixtures do not establish a real Herdr compatibility range;
#7 must characterize supported reads. Contract eligibility/freshness rules are
specifications, not a hidden second policy implementation. #8 implements and
proves them. #11 visual QA and #12 real-host/merged-tree acceptance remain pending.

Suggested next tasks: after accepted merged/pushed #10, dispatch #6/#7/#11 in
parallel consuming this exact contract SHA and validation command.
