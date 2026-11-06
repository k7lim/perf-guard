# Review consumer checks

From the repository root on macOS with Xcode command-line tools:

```sh
bash menubar/test-review.sh
bash menubar/test-review.sh --render /tmp/perf-guard-review-visual
```

Independent M1 acceptance also bridges actual fixture-backed Go CLI envelopes
through the production Swift model:

```sh
bash scripts/test-m1-acceptance.sh
```

This builds the existing Go CLI test executable with frozen provider injection,
runs real `review` and multi-selection `review plan` commands, then compares
Swift record IDs, reason sets, reviewability and selected plan identities with
the same frozen healthy input. It adds no eligibility policy or live target.

The standalone harness compiles only the review model, view and harness. It does
not launch the menu app, call its existing daemon controls, enable login items or
install anything. Its executable/module cache are temporary. Screenshots render
a real `NSHostingView`, including native controls and scroll content. Default
inputs are synthetic contract fixtures, never host session data.

Tests cover healthy/empty/all-excluded/stale/partial/unavailable/dirty/cross-server
records, explicit multi-selection, changed occupants/PID reuse, matching refresh,
missing/excluded plan outcomes, unknown activity/memory, error-envelope precedence,
incompatible versions, malformed JSON, delayed replies and late target responses.
A recording client checks exact list/plan flags. Disposable local CLI stubs test
the production subprocess runner and fallback from an older installed CLI; only
`describe` and `review` are observed. Control requests are rejected.

The view consumes CLI `reviewable` and `reasons`; it does not infer eligibility.
ID/identity comparison protects selection. Page/target changes clear selection
explicitly. Each refresh clears the previous plan. JSON decoding runs outside the
main actor. Each own CLI helper has an eight-second total budget including
capability discovery. Timeout cancellation applies only to that helper.

Installed CLI candidates advertising `review` in read-only `describe` are
preferred, with the existing bundled/adjacent fallback. The existing monitor model
and deployment configuration are unchanged by this review flow.

For #12, inject the freshly built CLI and an authorized explicit target into the
same model/view (replace these synthetic example arguments locally):

```sh
bash menubar/test-review.sh --live /tmp/m1-perf-guard unix:///example/herdr.sock session-a /tmp/private-real-review.png
```

This performs up to ten list requests and one read-only plan.
It navigates subsequent pages when needed to find a reviewable record. Failure to
find one fails the real-plan gate. With a screenshot argument, it captures the
review viewport and an additional `.plan.png` scrolled to the plan in the exact
production view; both files contain private live data and must stay local.
It never exits a session. Keep real screenshots/runtime paths outside the public
repository. No reviewable live record is reported explicitly, not passed as a
real plan. #12 owns live integration. Inherited context, when provided by the
caller, uses the CLI's `PERF_GUARD_HERDR_ENDPOINT` and `PERF_GUARD_HERDR_SESSION`.

Build the full app with writable temporary caches in a restricted workspace:

```sh
CLANG_MODULE_CACHE_PATH=/tmp/perf-guard-swift-module-cache \
SWIFT_MODULECACHE_PATH=/tmp/perf-guard-swift-module-cache \
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod \
GOCACHE=/tmp/perf-guard-go-cache bash menubar/build.sh
```

This assembles/signs the worktree app, without installing or launching it. Default
Swift cache paths may be denied in the agent sandbox; these cache paths avoid it.
