# macOS Performance Diagnostic Report

**Date:** 2026-03-29
**Machine:** Apple M4 Pro, 48 GB RAM, macOS 15.7.4
**Displays:** Built-in Liquid Retina XDR (3024x1964) + LG HDR 4K external (3840x2160, scaled to 1920x1080 @ 60Hz)

---

## Symptoms Reported

- `kernel_task` spiking to 186% CPU (intermittent)
- `WindowServer` consuming massive cumulative CPU time (1852+ hours)
- General system sluggishness during heavy dev workflows

---

## Diagnostic Data

### Thermal Status

```
pmset -g therm → No thermal warning level has been recorded
                 No performance warning level has been recorded
```

**Verdict:** Not thermally throttled. `kernel_task` spikes are not temperature-driven.

### Memory Pressure (CRITICAL)

```
vm_stat snapshot:
  Pages in compressor:        7,288,402 (~110 GB compressed)
  Compressions:              4,703,256,896 (4.7 billion)
  Decompressions:            4,171,267,428 (4.2 billion)
  Swapins:                   53,615,107
  Swapouts:                  58,090,942

memory_pressure → System-wide memory free: 42%
```

**Verdict:** Extreme memory compression thrash. The compressor is running continuously, burning CPU cycles on every compress/decompress cycle. This is the primary driver of `kernel_task` CPU expansion.

### Disk I/O

```
iostat snapshot:
  disk0: 902 TPS, 26.81 MB/s (spike)
  disk0: 233 TPS,  9.47 MB/s (baseline)
```

**Verdict:** Elevated I/O, secondary to swap thrashing. Will improve if memory pressure is reduced.

### Top Resource Consumers

| Process | % CPU | RAM | Cumulative CPU Time | Notes |
|---------|-------|-----|---------------------|-------|
| **iTerm2** | 34–43% | 1.2–1.4 GB | 1,560 hrs (old) / 36 min (current instance) | Metal rendering overhead |
| **WindowServer** | 27–47% | 4.9 GB | 1,852 hrs | Dual 4K + 10+ windows (expected) |
| **kernel_task** | 18.6% (snapshot) | 99 MB | — | Spikes to 186% under pressure |
| **Brave Browser** + helpers | 24% total | ~1.5 GB | — | **235 helper processes** |
| **com.apple.Virtualization** | 5.1% | 1.4 GB | 756 hrs | Lima VM |
| **Firefox** | 3.1% | 1.1 GB | 151 hrs | Secondary browser |
| **VS Code** helpers | ~4% | ~0.8 GB | — | Multiple renderer processes |

### iTerm2 Rendering Profile

```
sample iTerm2 → 100% of main thread time in:
  iTermMetalView.draw()
    → iTermMetalDriver.reallyDrawInMTKView
      → newFrameDataForView
        → MetalPerFrameState init
```

**Verdict:** iTerm2's Metal renderer is spending all CPU time on GPU synchronization (`GetDrawable` blocking), not actual text rendering. This is a known issue (GitLab #6767, #7856, #6917) with dual-display setups.

### Brave Browser Helper Sprawl

```
235 Brave Browser Helper processes detected
Combined CPU: 24%
Combined RAM: ~1.5 GB across all helpers
```

**Verdict:** Process explosion from many open tabs. Each tab spawns a renderer process that WindowServer must track and render.

### WindowServer Logs

```
No error spams. Normal event processing (button clicks, connection debug).
```

**Verdict:** WindowServer is behaving normally given the load. 27–47% CPU with dual 4K and 10+ windows is within expected range. Not a problem to fix.

---

## Root Cause Ranking

| Priority | Issue | Impact | Automatable? |
|----------|-------|--------|--------------|
| **1** | iTerm2 Metal rendering overhead | 40%+ CPU wasted on GPU sync | Yes — settings change or terminal swap |
| **2** | Memory compression thrash | 4.7B compressions → swap → kernel_task expansion | Yes — suspend RAM hogs |
| **3** | Browser helper sprawl | 235 processes, 24% CPU, 1.5 GB RAM | Yes — kill/suspend helpers |
| **4** | Disk I/O (swap-driven) | 900 TPS spikes | Resolved by fixing #2 |
| **5** | WindowServer load | 27–47% CPU | No — expected with dual 4K |

---

## Proposed Fixes

### Fix 1: iTerm2 Settings Changes (Immediate, No Code)

> **Verified against iTerm2 3.6.9 source code** (`gnachman/iterm2`, Preferences UI + AdvancedSettingsModel)

**Action items (in order of likely impact):**

1. **Disable GPU rendering**
   - **Settings > General** → uncheck **"GPU rendering"**
   - This is a global toggle (key: `UseMetal`, default: YES) — not per-profile as sometimes assumed
   - M4 Pro CPU cores are fast enough; Core Text may be snappier without Metal sync overhead
   - One user reported `cat` of large files: 7–10s (GPU on) → 4s (GPU off)

2. **Try "Maximize throughput at the cost of higher latency"**
   - **Settings > General** → check **"Maximize throughput at the cost of higher latency"**
   - Key: `metalMaximizeThroughput` — may help if you keep GPU rendering on

3. **Try synchronized Metal drawing (experimental)**
   - **Edit > Edit Advanced Settings** → search "synchronized"
   - Enable **"Use synchronized Metal drawable presentation?"** (key: `metalSynchronizedDrawing`, default: NO)
   - Defers drawable acquisition until rendering completes; may improve frame rates

4. **Check adaptive framerate settings**
   - **Edit > Edit Advanced Settings** → search "Drawing"
   - `Drawing: Use adaptive framerate.` (default YES — leave this on)
   - `Drawing: When adaptive framerate is enabled and using the Metal renderer, refresh at this rate during high throughput conditions (FPS).` — default is 30, can lower to 15
   - `Drawing: Frame rate (FPS) when adaptive framerate is enabled and throughput is low but not 0.` — default 60, try 30

5. **Disable transparency and blur on sandbox profiles**
   - **Settings > Profiles > [profile] > Window** → set **"Transparency:"** slider to 0, uncheck **"Blur"**
   - Transparency/blur are very expensive with Metal on external 4K displays
   - iTerm2 shows a warning: *"Large radius may reduce performance"*

6. **Reduce scrollback buffer**
   - **Settings > Profiles > [profile] > Terminal** → uncheck **"Unlimited scrollback"**, set **"Scrollback lines:"** to 10,000–50,000

7. **Profile-specific rendering note:**
   - Badge rendering itself is NOT the bottleneck (0.0ms in traces)
   - The culprit is likely different profiles using different transparency/blur settings, forcing full re-renders
   - Cursor guide rendering is more expensive than badges

**If still slow after tweaks, consider:**
- **Kitty** or **Ghostty** for heavy terminal work (native GPU pipelines, no Core Animation overhead)
- Keep iTerm2 for features you depend on (Shell Integration, Triggers, Password Manager)

**Sources:**
- [iTerm2 #6767 — Slow scrolling with GPU rendering](https://gitlab.com/gnachman/iterm2/-/issues/6767)
- [iTerm2 #7856 — Terrible UI performance with large windows](https://gitlab.com/gnachman/iterm2/-/issues/7856)
- [iTerm2 #6917 — GPU rendering much slower on my setup](https://gitlab.com/gnachman/iterm2/-/issues/6917)
- Source-verified: `Interfaces/PreferencePanel.xib`, `sources/iTermAdvancedSettingsModel.m`

---

### Fix 2: Build `perf-guard` (Automated Memory & Process Management)

**Stack:** Go — single binary, no runtime dependencies, works as both CLI and LaunchDaemon

**What it does:**

```
┌──────────────────────────────────────────────────────────────┐
│  perf-guard                                                  │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  MONITORS:                                                   │
│  • vm_stat compression rate (pages/sec) — every 5s          │
│  • Brave/Firefox helper process count                        │
│  • Swap activity rate                                        │
│                                                              │
│  THRESHOLD ACTIONS (configurable):                           │
│  • Level 1 (>500 pages/s compressed):                       │
│      → macOS notification: "Memory pressure rising"         │
│  • Level 2 (>1000 pages/s compressed):                      │
│      → Auto-suspend Lima VM via `limactl stop`              │
│  • Level 3 (>2000 pages/s compressed OR >150 helpers):      │
│      → SIGSTOP browser helpers                              │
│      → SIGSTOP VS Code helper renderers                     │
│                                                              │
│  CLI COMMANDS:                                               │
│  • perf-guard status     — current pressure + stats         │
│  • perf-guard focus      — one-click suspend all non-essential │
│  • perf-guard restore    — resume all suspended processes    │
│  • perf-guard suspend vm — suspend Lima VM only             │
│  • perf-guard suspend browsers — SIGSTOP browser helpers    │
│                                                              │
│  DAEMON MODE:                                                │
│  • Install as LaunchDaemon via com.user.perf-guard.plist    │
│  • Logs to /tmp/perf-guard.log                              │
│  • Config file: ~/.config/perf-guard/config.toml            │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

**Key design decisions:**
- Non-destructive by default: notifies first, suspends (SIGSTOP) rather than kills
- SIGSTOP preserves process state; SIGCONT resumes instantly
- Lima VM suspension is safe — `limactl stop` gracefully shuts down the VM
- Process groups are configurable so you can protect critical workflows

**Existing tools evaluated:**

| Tool | Stars | Why it doesn't solve this |
|------|-------|--------------------------|
| [exelban/stats](https://github.com/exelban/stats) | 37K | Passive monitoring only — no action on thresholds |
| [MonitorControl](https://github.com/MonitorControl/MonitorControl) | 33K | Display control, not process management |
| [heimdall-mcp](https://github.com/sarva-20/heimdall-mcp) | 13 | MCP integration layer, no automation |
| [macos-focus-mode](https://github.com/arodik/macos-focus-mode) | 46 | System Focus Mode, not process management |

**Gap:** No existing tool combines memory pressure monitoring with automated process suspension. All monitors are passive; none take action.

---

### Fix 3: Browser Tab Management (Low Effort, Quick Win)

**Browser extensions:**
- **Auto Tab Discard** (Firefox/Chrome/Brave) — discards inactive tabs, reduces helper count
- **The Marvellous Suspender** — suspends inactive tabs after timeout

**Or via CLI:**
```bash
# One-liner to SIGSTOP all Brave helpers
pkill -STOP -f "Brave Browser Helper \(Renderer\)"
# Resume
pkill -CONT -f "Brave Browser Helper \(Renderer\)"
```

This would be integrated into `perf-guard`'s "focus mode."

---

### Fix 4: Usage Pattern Recommendations

| Recommendation | Why |
|----------------|-----|
| Consolidate to one browser | Running Brave + Firefox doubles renderer overhead |
| Use `tmux` inside fewer iTerm2 windows | Reduces Metal draw calls and WindowServer tracking |
| Restart iTerm2 periodically | Prevents memory accumulation (peaked at 6.1 GB) |
| Close VM when not actively developing | 1.4 GB RAM freed immediately |
| Consider Kitty/Ghostty for heavy terminal work | Native GPU pipelines, no Core Animation sync overhead |

---

## Summary: What to Do

1. **Now (5 min):** Change iTerm2 settings — disable GPU rendering on sandbox profiles, set FPS to 30, kill transparency
2. **Today (30 min):** Install browser tab discard extension, consolidate to one browser
3. **This week:** Build `perf-guard` MVP — vm_stat reader + threshold alerts + `focus`/`restore` CLI
4. **Ongoing:** Evaluate Kitty or Ghostty as primary terminal if iTerm2 tweaks are insufficient
