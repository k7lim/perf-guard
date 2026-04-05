# Ticket 05: Forensic Spike Detector

**Depends on**: Ticket 03 (forensic config — for `WindowSize` and `Cooldown` settings)

## Goal

Create `internal/forensic/spike.go` — a rolling-window spike detector that triggers when the current compression rate exceeds 3x the rolling average. This is the "should I capture?" decision logic for the forensic system.

## Context

The daemon polls `vm_stat` every 1-30 seconds depending on pressure level. Each poll produces a compression rate (pages/sec). The spike detector maintains a rolling window of these rates and fires when it sees a sudden jump — indicating an acute moment worth capturing forensically.

This is a pure-logic module with no I/O, no system calls, no dependencies on other internal packages beyond config values.

**New package directory**: `internal/forensic/` does not exist yet. Create it: `mkdir -p internal/forensic`

## Deliverables

### New file: `internal/forensic/spike.go`

```go
package forensic

import "time"

type SpikeDetector struct {
    windowSize int
    cooldown   time.Duration
    window     []float64
    lastSpike  time.Time
}

func NewSpikeDetector(windowSize int, cooldown time.Duration) *SpikeDetector

// Record adds a compression rate to the rolling window and returns true
// if this reading constitutes a spike. A spike is:
//   1. Window is full (have enough baseline data)
//   2. Current rate > 3x rolling average of the window
//   3. At least `cooldown` has elapsed since last spike
// After detecting a spike, updates lastSpike time.
func (s *SpikeDetector) Record(rate float64) bool

// LastSpikeTime returns when the most recent spike was detected.
// Returns zero time if no spike has occurred.
func (s *SpikeDetector) LastSpikeTime() time.Time
```

**Implementation details**:
- Window is a circular buffer (slice of `windowSize`, append and trim).
- Rolling average: sum of window / len(window). Simple mean, not weighted.
- The spike threshold is hardcoded at 3x (not configurable — simple is better until we have data showing otherwise).
- `Record` appends to window AFTER checking for spike (current reading compared against existing window, then added).
- `windowSize` and `cooldown` come from `config.ForensicConfig` but are passed as constructor args (no config import needed in this package).

### New file: `internal/forensic/spike_test.go`

Test cases:
- Window not full → never triggers
- Steady readings (e.g., 100, 100, 100...) → no spike
- Sudden jump (100, 100, ..., 500) → spike detected
- Cooldown: spike, then immediate high reading → no second spike until cooldown expires
- Edge: all zeros then rate=50 → no spike (50 > 3*0 but 50 < 100 floor)
- Edge: all zeros then rate=150 → spike (150 > 3*0 AND 150 > 100 floor)
- Edge: window size 3, exactly 3 readings at 100, 4th at 301 → spike (301 > 300)
- `LastSpikeTime` returns zero before any spike, correct time after

## Design decisions

- **Absolute floor of 100 pages/s**: A spike requires BOTH (1) `rate > 3 * rolling_average` AND (2) `rate > 100`. The floor prevents spurious triggers during idle→light-use transitions (e.g., baseline 0, rate 1 would pass the 3x check but not the floor). 100 is well below the warning threshold of 500, so real pressure events always clear it.
- **Thread safety**: NOT thread-safe. The daemon calls `Record()` from its main loop only. If ticket 10 (wire into daemon) needs concurrent access, it should add its own mutex.

## Files touched

| File | Action |
|------|--------|
| `internal/forensic/spike.go` | Create |
| `internal/forensic/spike_test.go` | Create |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.5 --append-notes "..." --json`

Downstream: ticket 09 (snapshot assembly) imports `SpikeDetector` types. Ticket 10 creates a `SpikeDetector` in the daemon and calls `Record()`.

## Acceptance criteria

- [ ] `go test ./internal/forensic/...` passes
- [ ] Steady-state readings never trigger
- [ ] 3x+ jump after full window triggers spike
- [ ] Cooldown prevents rapid re-triggering
- [ ] Rates below 100 pages/s absolute never trigger (idle floor)
- [ ] `LastSpikeTime()` accurate
