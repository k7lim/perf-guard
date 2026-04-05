# perf-guard -- Agent Integration

perf-guard monitors macOS memory pressure and manages processes that cause it. It exists because this machine's memory compressor runs billions of compress/decompress cycles, 300+ browser helpers pile up, and swap thrashing drags everything down. The tool watches compression rates and helper counts, then takes graduated action: notify, stop VMs, SIGSTOP helpers.

An agent's job here is to check system health, decide whether intervention is needed, and execute the right command. Every mutation is reversible. The tool is designed to make the wrong action cheap to undo rather than hard to take.

## Structured output

```bash
export OUTPUT_FORMAT=json   # or pass --output json per-command
```

Output auto-detects: JSON when stdout is piped, text when TTY. All JSON responses share an envelope:

```json
{
  "command": "status",
  "success": true,
  "error": "",
  "data": { ... },
  "timestamp": "2026-04-01T10:00:00-07:00"
}
```

Always check `success` before reading `data`. On failure, `data` is null and `error` explains why.

## Runtime introspection

```bash
perf-guard describe
```

Returns the full schema: every command, its flags, output shape, which commands mutate, and which are reversible. Always JSON. Use this instead of hardcoding assumptions about the CLI.

## Commands

### status (read-only, ~2s latency)

Takes two vm_stat snapshots 2 seconds apart to compute rates. This is the starting point for any decision.

```bash
perf-guard status --output json
perf-guard status --output json --fields level,compression_rate,browser_helpers
```

Key fields:
- `level`: `normal` | `warning` | `high` | `critical` -- the single most useful signal
- `compression_rate`: pages/sec being compressed. >500 = warning, >2000 = critical
- `browser_helpers`: count of renderer processes. >150 = critical regardless of compression rate
- `suspended`: array of processes perf-guard has already suspended
- `vm_stopped`: whether perf-guard stopped the Lima VM

Use `--fields` to request only what you need. The full status response has 14 fields; most decisions only need 2-3.

### focus (mutates, reversible with `restore`)

Suspends all non-essential processes in priority order: stop Lima VM first, then browser helpers, then editor helpers.

```bash
# Always preview first
perf-guard focus --dry-run --output json

# Then execute
perf-guard focus --output json
```

Dry-run discovers processes and reports what would happen without signaling anything. Each action in the response has a `status` field: `planned` (dry-run), `ok`, `error`, or `skipped`.

### restore (mutates)

Resumes everything perf-guard suspended. Verifies each PID still belongs to the same process before sending SIGCONT (guards against PID reuse after a process exits).

```bash
perf-guard restore --dry-run --output json
perf-guard restore --output json
```

### suspend vm / suspend browsers (mutates, reversible with `restore`)

Targeted versions of focus. Use when you want to free resources from one category without touching others.

```bash
perf-guard suspend browsers --dry-run --output json
perf-guard suspend vm --dry-run --output json
```

### install (mutates)

Installs perf-guard as a LaunchAgent so the daemon starts on login and auto-restarts. Not something an agent should run without explicit user request.

```bash
perf-guard install --dry-run --output json
```

### forensics list (read-only)

Paginated list of forensic snapshots.

```bash
perf-guard forensics list --output json
perf-guard forensics list --output json --limit 10 --offset 0
```

Key fields in each entry: `timestamp`, `level`, `compression_rate`, `process_count`, `tab_count`.

### forensics show (read-only)

Full forensic snapshot by timestamp prefix.

```bash
perf-guard forensics show 2026-04-04T2347 --output json
```

Returns the complete snapshot in the `data` field.

### forensics report (read-only)

Aggregate analysis across stored snapshots.

```bash
perf-guard forensics report --output json --days 7
```

Key fields: `most_frequent_processes`, `most_frequent_urls`, `spikes_by_hour`, `avg_compression_at_spike`.

## Decision framework

When should an agent intervene? Here's the intended logic:

1. **Run `status`**. Read `level`.
2. If `normal`: do nothing.
3. If `warning`: inform the user. The daemon will notify too, but the agent may have more context about what's running and why.
4. If `high`: suggest `suspend vm` if the user isn't actively using the VM. The daemon handles this automatically, but if the daemon isn't running, the agent should.
5. If `critical`: suggest `focus` to suspend all non-essential processes. Always dry-run first to show the user what will be suspended.
6. After any intervention, run `forensics list` to check if a spike was captured. If so, `forensics show <timestamp>` gives the agent deep context about what caused the pressure.

The daemon and CLI can conflict if both try to manage the same processes. If the daemon is running (`perf-guard daemon` or via LaunchAgent), prefer reading status over taking CLI actions -- the daemon has hysteresis logic and will handle escalation/de-escalation automatically. Use CLI actions when the daemon isn't running or when the user explicitly asks.

## Rules

1. **Always `--dry-run` before any mutation.** Show the user what will happen. The cost of a dry-run is <1 second; the cost of suspending the wrong process is a frozen window.
2. **Always check `status` before deciding to act.** Don't suspend browsers because they "might" be using memory -- check the actual compression rate.
3. **Never run `focus` or `suspend` without telling the user.** These freeze visible application windows. The user needs to know why their browser tabs stopped responding.
4. **Treat `restore` as low-risk.** Resuming suspended processes is always safe. If in doubt, restore.
5. **Don't run `install` proactively.** Installing a LaunchAgent is a persistent system change. Only do it when asked.

## Configuration

Config lives at `~/.config/perf-guard/config.toml`. Optional -- defaults work without it. The config controls:

- Polling intervals per pressure level
- Compression rate thresholds for each level
- Protected process list (substring match, never touched)
- Process groups with match patterns, actions, and priority

State (suspended PIDs) lives at `~/.config/perf-guard/state.json`. This is how `restore` knows what to resume even if the daemon crashes.

## Privileges

- `status`, `describe`: no special privileges
- `focus`, `suspend`, `restore`: must own the target processes (same user)
- `daemon` with mlockall: elevated privileges (optional, warns if unavailable)
- `install`: writes to `~/Library/LaunchAgents/` (user-writable, no sudo)

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
