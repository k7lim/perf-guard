package forensic

import (
	"testing"
	"time"
)

func TestSpikeDetector_WindowNotFull(t *testing.T) {
	sd := NewSpikeDetector(5, time.Second)
	// Fill less than window — should never trigger
	for i := 0; i < 4; i++ {
		if sd.Record(1000) {
			t.Fatalf("triggered spike with only %d readings", i+1)
		}
	}
}

func TestSpikeDetector_SteadyReadings(t *testing.T) {
	sd := NewSpikeDetector(5, time.Second)
	for i := 0; i < 20; i++ {
		if sd.Record(100) {
			t.Fatalf("spike on steady readings at iteration %d", i)
		}
	}
}

func TestSpikeDetector_SuddenJump(t *testing.T) {
	sd := NewSpikeDetector(5, time.Second)
	// Fill window with baseline
	for i := 0; i < 5; i++ {
		sd.Record(100)
	}
	// Sudden jump: 500 > 3*100 and > 100 floor
	if !sd.Record(500) {
		t.Error("expected spike on sudden jump from 100 to 500")
	}
}

func TestSpikeDetector_Cooldown(t *testing.T) {
	sd := NewSpikeDetector(3, 10*time.Second)
	for i := 0; i < 3; i++ {
		sd.Record(100)
	}
	if !sd.Record(500) {
		t.Fatal("expected first spike")
	}
	// Fill window again
	for i := 0; i < 3; i++ {
		sd.Record(100)
	}
	// Second jump within cooldown — should NOT trigger
	if sd.Record(500) {
		t.Error("spike should be suppressed within cooldown")
	}
}

func TestSpikeDetector_AbsoluteFloor_ZeroBaseline(t *testing.T) {
	sd := NewSpikeDetector(3, time.Second)
	for i := 0; i < 3; i++ {
		sd.Record(0)
	}
	// 50 > 3*0 but 50 < 100 floor
	if sd.Record(50) {
		t.Error("50 should not trigger spike (below floor)")
	}
}

func TestSpikeDetector_AbsoluteFloor_HighEnough(t *testing.T) {
	sd := NewSpikeDetector(3, time.Second)
	for i := 0; i < 3; i++ {
		sd.Record(0)
	}
	// 150 > 3*0 and 150 > 100 floor
	if !sd.Record(150) {
		t.Error("expected spike: 150 > floor and > 3*0")
	}
}

func TestSpikeDetector_ExactThreshold(t *testing.T) {
	sd := NewSpikeDetector(3, time.Second)
	for i := 0; i < 3; i++ {
		sd.Record(100)
	}
	// 301 > 3*100=300
	if !sd.Record(301) {
		t.Error("expected spike at 301 (> 3*100)")
	}
}

func TestSpikeDetector_LastSpikeTime(t *testing.T) {
	sd := NewSpikeDetector(3, time.Second)
	if !sd.LastSpikeTime().IsZero() {
		t.Error("LastSpikeTime should be zero before any spike")
	}
	for i := 0; i < 3; i++ {
		sd.Record(100)
	}
	before := time.Now()
	sd.Record(500)
	after := time.Now()

	last := sd.LastSpikeTime()
	if last.Before(before) || last.After(after) {
		t.Errorf("LastSpikeTime %v not in range [%v, %v]", last, before, after)
	}
}
