# Read-only Herdr adapter (M1 #7)

`herdr.NewSource(process.ReadIdentity)` implements `review.HerdrSource` for #8.
The identity callback is a seam: this package does not depend on #6's collector.
It must return an actual OS PID/start identity and honor its context. Tests inject
a clock, transport and version reader. Production transport opens only the
explicit local Unix socket and sends `ping`, `session.snapshot` and
`pane.process_info`; it cannot send arbitrary params or control commands.

`ResolveTarget` accepts an absolute Unix socket path or `unix:///absolute/path`
and a nonempty explicit session label. A wholly absent argument target may use
both `PERF_GUARD_HERDR_ENDPOINT` and `PERF_GUARD_HERDR_SESSION` inherited from the
caller. A partial explicit target is rejected, never completed from environment.
There is no config discovery, default session, UI-focus fallback, SSH, server
startup or installation. #8 should call `ResolveTarget` to distinguish invalid
explicit CLI syntax from the optional unavailable-target case before collection.
A missing target in `Collect` returns an unavailable inventory and warning.

The server currently exposes no server UUID in the characterized API. `server_id`
is `endpoint-sha256-<digest>` of the canonical local Unix endpoint. The explicit
endpoint and caller-supplied session label are retained in the contract scope.
The characterized server does not echo a named-session identity, so the label is
explicit caller context rather than an independently authenticated server claim.
Different endpoint
sockets cannot collide even if public pane IDs match. The adapter does not claim
the hash is a server-provided UUID or server incarnation identifier.

The adapter brackets correlation with two session snapshots and two per-pane
process reads plus actual OS identity reads. It accepts a single foreground
process whose executable basename matches the pane's `codex`/`claude` agent.
Changed terminal/native-session metadata, changed PID/start identity, duplicate
scoped panes, multiple candidates, missing resolver or unsupported reads leave
correlation unknown/ambiguous. Process-only wrapper evidence (`nono`, shell,
`log`, etc.) cannot identify an agent and remains unknown. This is an intentional
coverage limit; it must not be turned into a positive wrapper correlation by
reconciliation. No same-cwd/age/TTY guess substitutes for evidence.

`revision` is a stable hash derived from verified PID/start identity and full
scope, terminal identity and agent. Native terminal output `revision` and
lifecycle redraw sequences are never decoded or used as occupant generation.
The latest matching snapshot supplies a lifecycle observation, not a user or
conversation activity timestamp. Native resume reference values, when present,
are preserved verbatim in the local DTO; they are context, not rollback.

Source states are fresh, partial or unavailable. This live-only adapter has no
cache or fabricated stale records: collection timestamps use the injected clock,
and #8 applies the frozen freshness policy. Optional lookup/read failures carry
warnings; failures never become a healthy empty inventory. Missing pane scope is
reported as partial and omitted because no valid scoped record can be emitted.
A successfully empty snapshot remains fresh with an empty array.

One inherited context bounds the full collection, capped at five seconds.
Each connection has the same or earlier deadline, cancellation closes only that
helper connection, responses are bounded to 16 MiB, IDs/types are checked, and
raw API error text is not logged or retained. The sole CLI subprocess is
`herdr --version`, under that context. It cannot start or attach a Herdr session.
The provider does not retain cwd, argv, cmdline, titles, scrollback or transcripts.
No production adapter method exposes signals, pane closure, input, daemon or
server control, upgrade, install, or configuration writes.

## Compatibility evidence

Empirically tested on the authorized Mac: **client 0.9.5 / running server 0.9.0 /
protocol 22**. The client's bundled JSON schema also describes protocol 22.
Scoped live reads confirmed `ping`, `session.snapshot`, and `pane.process_info`.
The sanitized `testdata/protocol22-server090-client095.json` fixture reproduces
only relevant field shapes from those supported reads; its names/IDs/resume data
are synthetic. The versioned unsupported-protocol fixture proves fail-closed
behavior; unsupported process-info reads retain lifecycle rows without identities.
No other server versions or protocol generations are claimed tested. Protocol 22
responses must satisfy the validated shape at runtime; an incompatible protocol
is unavailable with explicit warning and separately retained client/server facts.

## Verification

```sh
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/herdr -count=1 -v
GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache go test ./internal/review -count=1 -v
```

Pure tests use recording fakes and local temporary socket servers. Every normalized
fixture inventory is validated against the accepted HerdrInventory JSON schema.
The opt-in `TestReadOnlyHostSmoke` is skipped unless
`PERF_GUARD_HERDR_SMOKE_TARGET_FILE` names a private JSON file containing
`endpoint` and `session_id`. It never discovers a target. The test emits only
version, capability, status and aggregate correlation counts; no private runtime
inventory is written to source. This isolated provider smoke intentionally has no
OS identity resolver; #8/#12 must verify integrated positive correlation.
