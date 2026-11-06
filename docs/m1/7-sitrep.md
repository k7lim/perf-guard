# Issue #7 handback

Issue: https://github.com/k7lim/perf-guard/issues/7
Branch: `m1/7-herdr`
Base/contract: `ddf1b4436370325c58b9e5e7ad98d1d669e943c1`
Disposition: READY_FOR_REVIEW. The tested commit SHA accompanies the handback;
the orchestrator records its accepted merged SHA separately.

Achieved: explicitly scoped local Unix Herdr provider, bounded allowlisted
transport, client/server facts, lifecycle/unknown normalization, exact verified
PID/start correlation seam and occupant generation, sanitized compatibility
fixtures and recording/real-socket tests. No CLI registration, process collector,
shared schema amendment, Swift, daemon, installation or exit behavior.

Contract: accepted v1 from #10, no amendments. `herdr.NewSource` accepts an
`IdentityResolver` callback with the signature of #6's `process.ReadIdentity`.
`internal/herdr/README.md` records #8 wiring and inherited target env names.

## Acceptance matrix

| Criterion | Evidence | Observed result |
|---|---|---|
| Explicit/inherited target only | `TestExplicitAndInheritedTarget`, missing-target test, `ResolveTarget`, `SocketTransport` | PASS; partial target never guessed, no UI/default session fallback |
| Scoped panes, distinct versions | `TestReadOnlyNormalizationAndStableOccupants`, `TestScopedServersAndDuplicates`, live smoke | PASS; endpoint-derived server scope documented; client 0.9.5/server 0.9.0 separately reported |
| Lifecycle states and unavailable/stale distinction | Normalization test covers working/blocked/idle/done/unknown; disconnected/empty/deadline cases; live-only source documentation | PASS; no cached stale data fabricated, #8 owns age-based freshness policy |
| Positive PID/start correlation or explicit unknown | Changed identity tests, repeated snapshot/process/OS identity reads | PASS; missing resolver/start identity, wrappers, ambiguous candidates, PID reuse/changed occupant never positive |
| Missing/incompatible/partial/duplicate/resume | Missing/incompatible/empty tests, malformed/partial tests, native resume checks, duplicate tests | PASS; compatible data remains partial with exclusions, resume absent stays null |
| Deadline/cancellation | Deadline fake, socket cancellation and oversize tests | PASS; shared five-second cap, earlier caller cancellation honored |
| Recording fake allowlist/zero controls | All recording calls checked; `TestTransportRejectsControlBeforeDial`; production wiring inspection | PASS; no input/close/signal/start/stop/update/install calls |
| Versioned older-server/newer-client/unsupported fixtures | `testdata/protocol22-server090-client095.json`, protocol21 unsupported fixture, adapter README | PASS; empirical range limited to 0.9.5 client + 0.9.0 server protocol22; no speculative range |
| Authorized explicit read-only smoke | `TestReadOnlyHostSmoke` with private explicit target file | PASS on Mac; all three reads supported, 76 occupants; zero positive/76 unknown without resolver, not misreported as verified identities |
| No retained arguments/transcripts/secrets | Selected wire structs, `private-arguments-discarded` test, generic transport errors | PASS; unknown argv/cmdline/cwd/title fields discarded, native resume remains local DTO context |

## Checks and environment

Darwin 24.6.0 arm64, Go 1.26.1. Pure tests do not require live Herdr or OS process
access. Local socket tests use short unique `/tmp` socket paths for macOS limits.

Exact accepted schema/fixture command, exit 0:

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/review -count=1 -v
```

With that environment prefix: `go test ./internal/herdr -count=1 -v`,
`go test ./...`, `go test -race ./...`, `go vet ./...`,
`python3 scripts/validate-milestone-1.py`, and `git diff --check` exit 0.
Full tests/race use authorized read-only Mac access for the pre-existing `ps`
test; sandbox denial is not treated as zero processes.

Real host smoke command, exit 0:

```sh
PERF_GUARD_HERDR_SMOKE_TARGET_FILE=/private/tmp/perf-guard-m1-7-target-private.json GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/herdr -run TestReadOnlyHostSmoke -count=1 -v
```

Observed client 0.9.5 / server 0.9.0 / protocol22. Capability facts include
`ping`, `session.snapshot`, `pane.process_info`; server-reported flags are
separate prefixed metadata, not permissions to invoke their control methods.
Source was partial with unknown correlation because this isolated smoke omits
the OS resolver. No inventory/PIDs/cwd/resume values are emitted by the smoke.

Artifacts: committed provider/transport/tests, two versioned synthetic wire
fixtures and README. Private evidence remains under `/private/tmp` in target,
ping/snapshot/process characterization files; none is committed or published.

Side effects: recording-fake forbidden call list is empty. Real production path
is socket encode/read for three fixed methods plus `herdr --version`; transport
rejects controls before dialing and builds its own exact read params. No user
process signal, terminal input/closure, service control, config/index write or
installation path exists. Cancellation closes only this adapter's sockets or
its own bounded version subprocess.

Integration: only `internal/herdr/` and this SITREP are added. No shared files,
contract or dependency changes. #8 owns production construction and review policy.
Orchestrator owns merge, push, issue acceptance and closure.

Limitations: foreground wrapper-only layouts remain uncorrelated; ancestry
heuristics are intentionally not guessed. Source-only smoke establishes reads,
not integrated positive identities; #8/#12 must exercise injected platform
identity and combined review/plan paths. No transcript/activity inference and no
cache. Server identity is a documented endpoint scope derivation, not upstream
UUID. Unsupported protocol generations fail closed. No UI or milestone
completion claimed.

Suggested next task: accept/merge/push #6 and #7, then dispatch #8 with these
constructor/target seams and the unchanged v1 contract.
