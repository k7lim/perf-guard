package pressure

// Level represents the current memory pressure severity.
type Level int

const (
	Normal   Level = iota
	Warning        // Level 1: notify
	High           // Level 2: stop VM
	Critical       // Level 3: SIGSTOP helpers
)

func (l Level) String() string {
	switch l {
	case Normal:
		return "normal"
	case Warning:
		return "warning"
	case High:
		return "high"
	case Critical:
		return "critical"
	default:
		return "unknown"
	}
}

// Thresholds defines the compression rate boundaries for each level.
type Thresholds struct {
	WarningRate  float64 // pages/s → Warning
	HighRate     float64 // pages/s → High
	CriticalRate float64 // pages/s → Critical
	MaxHelpers   int     // helper count → Critical
}

// DefaultThresholds returns the default threshold values from the diagnostic report.
func DefaultThresholds() Thresholds {
	return Thresholds{
		WarningRate:  500,
		HighRate:     1000,
		CriticalRate: 2000,
		MaxHelpers:   150,
	}
}

// Evaluate returns the pressure level given a compression rate and helper count.
// Helper count escalates the level by one step rather than jumping straight to
// critical, so many browser tabs alone won't trigger focus mode.
func (t Thresholds) Evaluate(compressionRate float64, helperCount int) Level {
	var level Level
	switch {
	case compressionRate >= t.CriticalRate:
		level = Critical
	case compressionRate >= t.HighRate:
		level = High
	case compressionRate >= t.WarningRate:
		level = Warning
	default:
		level = Normal
	}

	// High helper count bumps the level up by one step (but can't exceed Critical).
	if helperCount >= t.MaxHelpers && level < Critical {
		level++
	}

	return level
}
