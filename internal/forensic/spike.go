package forensic

import "time"

const spikeFloor = 100.0 // absolute minimum rate to trigger a spike

// SpikeDetector maintains a rolling window of compression rates and detects
// sudden jumps that indicate acute memory pressure worth capturing.
type SpikeDetector struct {
	windowSize int
	cooldown   time.Duration
	window     []float64
	lastSpike  time.Time
}

// NewSpikeDetector creates a spike detector with the given window size and cooldown.
func NewSpikeDetector(windowSize int, cooldown time.Duration) *SpikeDetector {
	return &SpikeDetector{
		windowSize: windowSize,
		cooldown:   cooldown,
	}
}

// Record adds a compression rate to the rolling window and returns true
// if this reading constitutes a spike. A spike requires:
//  1. Window is full (enough baseline data)
//  2. Current rate > 3x rolling average of the window
//  3. Current rate > absolute floor (100 pages/s)
//  4. At least cooldown has elapsed since last spike
func (s *SpikeDetector) Record(rate float64) bool {
	spike := false

	if len(s.window) >= s.windowSize {
		avg := s.average()
		if rate > 3*avg && rate > spikeFloor {
			if s.lastSpike.IsZero() || time.Since(s.lastSpike) >= s.cooldown {
				spike = true
				s.lastSpike = time.Now()
			}
		}
	}

	// Add to window after spike check
	s.window = append(s.window, rate)
	if len(s.window) > s.windowSize {
		s.window = s.window[len(s.window)-s.windowSize:]
	}

	return spike
}

// LastSpikeTime returns when the most recent spike was detected.
// Returns zero time if no spike has occurred.
func (s *SpikeDetector) LastSpikeTime() time.Time {
	return s.lastSpike
}

func (s *SpikeDetector) average() float64 {
	if len(s.window) == 0 {
		return 0
	}
	var sum float64
	for _, v := range s.window {
		sum += v
	}
	return sum / float64(len(s.window))
}
