package daemon

import (
	"testing"
	"time"

	"github.com/k7lim/perf-guard/internal/config"
	"github.com/k7lim/perf-guard/internal/pressure"
)

// Replay the reported warning/critical oscillation through the real threshold,
// hysteresis and notification decision path. No OS notifications are sent.
func TestNotificationPressureFlapping(t *testing.T) {
	cfg := config.Default()
	cfg.MonitorOnly = true
	d := New(cfg)
	at := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	var alerts []string
	observe := func(rate float64) {
		level := d.applyHysteresis(d.thresholds.Evaluate(rate, 216))
		if message := d.notificationForLevel(level, rate, 216, at); message != "" {
			alerts = append(alerts, message)
		}
		d.currentLevel = level
		at = at.Add(5 * time.Second)
	}
	observe(0)
	for range 12 {
		observe(3000)
		observe(0)
		observe(0)
		observe(0)
	}
	if len(alerts) != 2 {
		t.Fatalf("pressure flapping produced %d banners in four minutes; want initial warning and first critical escalation only", len(alerts))
	}
}

func TestNotificationCooldownAndEscalation(t *testing.T) {
	start := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	d := New(config.Default())
	check := func(at time.Duration, level pressure.Level, want bool) {
		t.Helper()
		got := d.notificationForLevel(level, 1000, 216, start.Add(at)) != ""
		if got != want {
			t.Fatalf("at %s, %s -> %s: notify=%t, want %t", at, d.currentLevel, level, got, want)
		}
		d.currentLevel = level
	}
	check(0, pressure.Warning, true)
	check(time.Second, pressure.Warning, false)
	check(2*time.Second, pressure.High, true)
	check(3*time.Second, pressure.Critical, true)
	check(4*time.Second, pressure.Warning, false)
	check(5*time.Second, pressure.Critical, false)
	check(6*time.Second, pressure.Normal, false)
	check(7*time.Second, pressure.Critical, false)
	check(notificationCooldown+2*time.Second, pressure.Warning, false)
	// Exactly five minutes since the last sent alert, a fresh escalation may alert.
	check(notificationCooldown+3*time.Second, pressure.Critical, true)
	check(2*notificationCooldown, pressure.Normal, false)
	check(2*notificationCooldown+3*time.Second, pressure.Warning, true)
}

func TestNotificationGateReservesBeforeDelivery(t *testing.T) {
	d := New(config.Default())
	at := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	if d.notificationForLevel(pressure.Critical, 3000, 216, at) == "" {
		t.Fatal("initial critical alert missing")
	}
	// Even before the loop advances currentLevel or delivery succeeds, retries
	// cannot enqueue another identical alert.
	if d.notificationForLevel(pressure.Critical, 3000, 216, at) != "" {
		t.Fatal("duplicate alert queued before delivery")
	}
	if d.notificationForLevel(pressure.Critical, 3000, 216, at.Add(-time.Second)) != "" {
		t.Fatal("backward clock bypassed cooldown")
	}
}
