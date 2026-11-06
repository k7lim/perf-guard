package daemon

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/k7lim/perf-guard/internal/pressure"
)

const notificationCooldown = 5 * time.Minute

// Only the monitor loop accesses this state. A temporary recovery must not
// reset it: otherwise normal/critical flapping could bypass the cooldown.
type notificationGate struct {
	lastSent time.Time
	highest  pressure.Level
}

// notificationForLevel is the notification decision used by the monitor loop.
// Reserve the notification before asynchronous delivery, so repeated readings
// cannot enqueue a burst. A failed OS delivery also respects the cooldown.
func (d *Daemon) notificationForLevel(level pressure.Level, rate float64, helpers int, now time.Time) string {
	if level <= d.currentLevel || level < pressure.Warning {
		return ""
	}
	g := &d.notifications
	if !g.lastSent.IsZero() && now.Sub(g.lastSent) < notificationCooldown {
		if level <= g.highest {
			return ""
		}
	} else {
		g.highest = pressure.Normal
	}
	g.lastSent = now
	g.highest = level
	return fmt.Sprintf("%s pressure: %.0f pages/sec compressed; %d helpers. Monitor only: %t.", level, rate, helpers, d.cfg.MonitorOnly)
}

// Pass text as an argument, never interpolate process names into AppleScript.
func notify(message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const script = `on run argv
display notification (item 1 of argv) with title "Performance Guard"
end run`
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script, message).CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript: %w: %s", err, out)
	}
	return nil
}
