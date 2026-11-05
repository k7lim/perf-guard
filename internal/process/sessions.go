package process

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// AgentProcess is an inventory entry, never an automatic termination candidate.
// RSS excludes compressed and swapped memory; age does not imply inactivity.
type AgentProcess struct {
	PID        int    `json:"pid"`
	ParentPID  int    `json:"parent_pid"`
	Agent      string `json:"agent"`
	Kind       string `json:"kind"`
	TTY        string `json:"tty"`
	AgeSeconds int64  `json:"age_seconds"`
	RSSBytes   uint64 `json:"rss_bytes"`
}

func AgentProcesses() ([]AgentProcess, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,tty=,etime=,rss=,comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("agent process discovery: %w", err)
	}
	return parseAgentProcesses(string(out)), nil
}

func parseAgentProcesses(raw string) []AgentProcess {
	var result []AgentProcess
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		name := strings.Join(f[5:], " ")
		agent := filepath.Base(name)
		if agent != "codex" && agent != "claude" {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(f[1])
		if err != nil {
			continue
		}
		age, err := elapsedSeconds(f[3])
		if err != nil {
			continue
		}
		rss, err := strconv.ParseUint(f[4], 10, 64)
		if err != nil {
			continue
		}
		kind := "background"
		switch {
		case strings.Contains(name, "/ChatGPT.app/"):
			kind = "desktop_helper"
		case strings.Contains(name, "/app-server-daemon/"):
			kind = "app_server"
		case f[2] != "??" && f[2] != "?" && f[2] != "-":
			kind = "terminal"
		}
		result = append(result, AgentProcess{pid, ppid, agent, kind, f[2], age, rss * 1024})
	}
	// Nested helpers attached to the same terminal are not extra sessions.
	parents := map[int]bool{}
	for _, p := range result {
		if p.Kind == "terminal" {
			parents[p.PID] = true
		}
	}
	for i := range result {
		if result[i].Kind == "terminal" && parents[result[i].ParentPID] {
			result[i].Kind = "terminal_helper"
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AgeSeconds > result[j].AgeSeconds })
	if result == nil {
		result = []AgentProcess{}
	}
	return result
}

func elapsedSeconds(s string) (int64, error) {
	var days int64
	if parts := strings.SplitN(s, "-", 2); len(parts) == 2 {
		v, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("invalid elapsed time %q", s)
		}
		days = v
		s = parts[1]
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid elapsed time %q", s)
	}
	seconds := int64(0)
	for _, p := range parts {
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("invalid elapsed time %q", s)
		}
		seconds = seconds*60 + v
	}
	return days*86400 + seconds, nil
}
