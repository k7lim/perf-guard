package process

import "testing"

func TestAgentInventoryDistinguishesSessionsAndHelpers(t *testing.T) {
	raw := `10 1 ttys001 44-00:00:00 400000 /opt/bin/codex
11 10 ttys001 43-00:00:00 10000 /opt/bin/codex
12 1 ?? 44-00:00:00 10000 /Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex
13 1 ?? 44-00:00:00 10000 /Users/example/.codex/packages/app-server-daemon/releases/v1/bin/codex
14 1 ttys002 01:02:03 200000 /opt/bin/claude
15 1 ?? 00:01 10 /usr/bin/other
`
	items := parseAgentProcesses(raw)
	if len(items) != 5 {
		t.Fatalf("entries=%d", len(items))
	}
	kinds := map[int]string{}
	for _, p := range items {
		kinds[p.PID] = p.Kind
	}
	for pid, want := range map[int]string{10: "terminal", 11: "terminal_helper", 12: "desktop_helper", 13: "app_server", 14: "terminal"} {
		if kinds[pid] != want {
			t.Errorf("pid %d kind=%s, want %s", pid, kinds[pid], want)
		}
	}
}

func TestElapsedSeconds(t *testing.T) {
	for raw, want := range map[string]int64{"44-01:02:03": 3805323, "01:02:03": 3723, "02:03": 123} {
		got, err := elapsedSeconds(raw)
		if err != nil || got != want {
			t.Errorf("%s=%d,%v want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"bogus", "-1:01", "a-01:02"} {
		if _, err := elapsedSeconds(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}
