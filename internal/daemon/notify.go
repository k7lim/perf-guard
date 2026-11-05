package daemon

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

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
