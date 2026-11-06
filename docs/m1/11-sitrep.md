# Issue #11 handback

Issue: https://github.com/k7lim/perf-guard/issues/11
Branch: `m1/11-menu`
Base/contract: `ddf1b4436370325c58b9e5e7ad98d1d669e943c1`
Head SHA: supplied with final handback; orchestrator records accepted merge SHA.
Disposition: READY_FOR_REVIEW, fixture-backed UI completion only.

Achieved: Review Sessions menu entry, asynchronous review window, explicit or
inherited targets, pagination, exact identity multi-selection and shared CLI
dry-run plans. No exit/apply/cleanup control. No Go collector/adapter/CLI/policy,
daemon, installer or contract changes. Existing live-status reading is preserved.

## Acceptance matrix

| Criterion | Evidence | Result |
|---|---|---|
| Project/agent/age/sourced activity/lifecycle/child work/worktree/RSS | `ReviewRecordView`, healthy/dirty/unavailable fixtures; `11-visual/healthy-review.png` | All rendered; unknown activity/memory remain unknown; resident RSS labeled |
| Exclusions/reasons and no age selection | Harness healthy/all-excluded/stale/duplicate cases; healthy/stale visuals | CLI flags/reasons consumed directly; initial selection empty; only checkbox disabled |
| Exact identity multi-selection and visible refresh invalidation | Harness matching-refresh, changed occupant/PID reuse, target/page change and late replies; `11-visual/selection-invalidated.png` | Matching exact identities retained; replacements removed; old plan cleared; notice visible |
| Shared plan and execution false | Recorded exact flags; reviewable/missing/excluded plan tests; whole example-plan decodes all four statuses; `11-visual/plan.png` | Exact repeatable IDs; mismatched identity/executable plans rejected; no execution control |
| Async execution/decoding, delayed and unavailable states | Delayed fixture plus main-actor liveness; late response discarded; failed envelope checked before data; `11-visual/error-stale.png` | Responsive; old data marked stale on failure; selections/plan cleared |
| Queried target and unknown versus zero | Target shown separately from editable inputs; empty/unavailable checks | Missing cwd/activity/memory unknown; successful empty inventory zero |
| Installed CLI and bundled fallback | Disposable old/current CLI stubs through production runner; candidate order inspected | Capable installed CLI preferred; unsupported installed CLI falls back; no daemon/login/install calls |
| Injectable scenario coverage | `menubar/Tests/ReviewHarness.swift`, `bash menubar/test-review.sh` | PASS without Herdr/private data |
| Visual artifacts and build | Five committed PNGs under `11-visual`; native render and full build commands below | Healthy/plan/stale/changed/error views personally inspected; native controls readable; scrolling supports window viewport |
| Frozen boundary and no duplicate policy | DTOs/command arguments, schema validation, source inspection | No amendment; no eligibility inferred from age/lifecycle; #12 owns real integration |

## Exact checks

Environment: macOS 15.7.7 / Darwin 24.6.0 arm64; Apple Swift 6.2.4; Go 1.26.1.
Final commands below exited 0:

```sh
bash menubar/test-review.sh
bash menubar/test-review.sh --render /private/tmp/perf-guard-m1-11-visual
CLANG_MODULE_CACHE_PATH=/tmp/perf-guard-swift-module-cache SWIFT_MODULECACHE_PATH=/tmp/perf-guard-swift-module-cache GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache bash menubar/build.sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/review -count=1 -v
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./...
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test -race ./...
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go vet ./...
python3 scripts/validate-milestone-1.py
bash -n menubar/build.sh menubar/install.sh menubar/test-review.sh
git diff --check
```

Full Go normal/race suites used authorized read-only host access for existing ps
tests. The first default app build exited 1 because the default Swift module cache
is outside sandbox writable paths; explicit temporary caches succeeded. An initial
ImageRenderer prototype omitted AppKit controls/scroll content; final native
NSHostingView artifacts were regenerated and inspected after replacing it.

Artifacts: model/view/harness/docs and five synthetic PNGs committed. Additional
empty/all-excluded/dirty/unavailable renders are reproducible in the local render
directory. No private runtime inventories, cwd/resume metadata, transcript data
or real session screenshots are committed.

Side effects: recording fixture client sees frozen review/list/plan only.
Disposable executable stubs confirm read-only describe discovery then review,
and control requests are rejected. Production launches only its own read helper;
timeout termination targets that helper only. No existing user process signaling,
session input/closure, daemon start/stop/update, login registration or installation
is performed. No real app/daemon was launched for this QA.

Integration: only Swift consumer/views, menu entry/window files, isolated harness,
script/docs and sanitized visuals. No shared Go schemas or CLI registration edits.
Contract remains accepted #10; branch merges independently of backend tasks.
Orchestrator owns accepted merge, combined-tree verification and push.

Limitations: live Herdr/CLI/menu parity is not claimed. #8 must land before #12
checks the combined real path. The harness supports explicit built-binary/target
injection for #12; see `menubar/Tests/README.md`. Native fixture rendering does not
establish an installed companion or real session behavior. No host installation
is required or performed. Suggested next task: #12 after #8/#11 accepted and pushed.
