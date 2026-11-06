package review

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitRead runs a bounded Git builtin and returns its combined output. It is a
// test seam, not an arbitrary user-command surface.
type GitRead func(context.Context, string, ...string) ([]byte, error)
type GitWorktreeSource struct {
	Clock Clock
	Read  GitRead
}

func gitRead(ctx context.Context, cwd string, args ...string) ([]byte, error) {
	base := []string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.hooksPath=/dev/null", "-C", cwd}
	base = append(base, args...)
	cmd := exec.CommandContext(ctx, "git", base...)
	// Scope probes to cwd and prevent inherited Git overrides from retargeting
	// the repository or index. No Git subcommand here mutates repository state.
	for _, v := range os.Environ() {
		key := strings.SplitN(v, "=", 2)[0]
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		cmd.Env = append(cmd.Env, v)
	}
	cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	var output boundedGitOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.Bytes(), err
}

// Status only needs empty/nonempty output. Cap retained diagnostics/listings so
// very large untracked inventories cannot consume unbounded agent memory.
type boundedGitOutput struct{ bytes.Buffer }

func (b *boundedGitOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (64 << 10) - b.Len()
	if remaining > 0 {
		_, _ = b.Buffer.Write(p[:min(remaining, n)])
	}
	return n, nil
}

func (s GitWorktreeSource) Inspect(parent context.Context, cwd string) WorktreeObservation {
	v := WorktreeObservation{State: "unknown", Provenance: "git-status-read-only", Warnings: []Warning{}}
	fail := func(code string) WorktreeObservation {
		v.Warnings = append(v.Warnings, warn("worktree", code, "Worktree state could not be observed"))
		return v
	}
	if !filepath.IsAbs(cwd) || strings.IndexByte(cwd, 0) >= 0 {
		return fail("probe_failed")
	}
	ctx, cancel := context.WithTimeout(parent, 250*time.Millisecond)
	defer cancel()
	read := s.Read
	if read == nil {
		read = gitRead
	}
	clock := s.Clock
	if clock == nil {
		clock = SystemClock{}
	}
	data, e := read(ctx, cwd, "rev-parse", "--is-inside-work-tree")
	if e != nil {
		if ctx.Err() != nil || errors.Is(e, context.DeadlineExceeded) || errors.Is(e, context.Canceled) {
			return fail("deadline_exceeded")
		}
		if strings.Contains(string(data), "cannot change to") {
			return fail("cwd_inaccessible")
		}
		if strings.Contains(string(data), "not a git repository") {
			v.State = "not_repository"
			t := clock.Now()
			v.ObservedAt = &t
			return v
		}
		return fail("probe_failed")
	}
	if strings.TrimSpace(string(data)) != "true" {
		v.State = "not_repository"
		t := clock.Now()
		v.ObservedAt = &t
		return v
	}
	data, e = read(ctx, cwd, "status", "--porcelain=v1", "-z", "--untracked-files=normal", "--ignore-submodules=none")
	if e != nil {
		if ctx.Err() != nil {
			return fail("deadline_exceeded")
		}
		return fail("probe_failed")
	}
	v.State = "clean"
	if len(data) > 0 {
		v.State = "dirty"
	}
	t := clock.Now()
	v.ObservedAt = &t
	return v
}
