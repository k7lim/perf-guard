//go:build darwin

package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/k7lim/perf-guard/internal/review"
	"golang.org/x/sys/unix"
)

type platformSnapshot struct{}
type platformCWD struct{}

func kernelProcess(ctx context.Context, pid int) (*unix.KinfoProc, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if pid <= 0 {
		return nil, errors.New("PID must be positive")
	}
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return nil, err
	}
	if p.Proc.P_pid != int32(pid) || p.Proc.P_starttime.Sec <= 0 {
		return nil, errors.New("process not present")
	}
	return p, nil
}
func kernelIdentity(p *unix.KinfoProc) review.ProcessIdentity {
	return review.ProcessIdentity{PID: int(p.Proc.P_pid), StartID: fmt.Sprintf("darwin-start:%d.%06d", p.Proc.P_starttime.Sec, p.Proc.P_starttime.Usec)}
}

// ReadIdentity reads the actual macOS kernel start timestamp, including
// microseconds. It never derives identity from process age or signals the PID.
func ReadIdentity(ctx context.Context, pid int) (review.ProcessIdentity, error) {
	p, e := kernelProcess(ctx, pid)
	if e != nil {
		return review.ProcessIdentity{}, e
	}
	return kernelIdentity(p), nil
}
func (platformSnapshot) Snapshot(ctx context.Context) ([]SnapshotProcess, []review.Warning, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	// Bracket ps with actual kernel identity/parent reads so PID reuse cannot
	// attach old ps memory/TTY facts to a replacement process.
	before, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, nil, fmt.Errorf("kernel process discovery: %w", err)
	}
	initial := map[int]unix.KinfoProc{}
	for _, p := range before {
		initial[int(p.Proc.P_pid)] = p
	}
	out, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,ppid=,tty=,rss=,comm=").Output()
	if err != nil {
		return nil, nil, err
	}
	rows, warnings := parseReviewPS(string(out))
	for i := range rows {
		r := &rows[i]
		if err := ctx.Err(); err != nil {
			warnings = append(warnings, warning("deadline_exceeded", "Kernel identity lookups incomplete"))
			break
		}
		b, ok := initial[r.PID]
		a, e := kernelProcess(ctx, r.PID)
		if !ok || e != nil || kernelIdentity(&b) != kernelIdentity(a) || int(a.Eproc.Ppid) != r.ParentPID || int(b.Eproc.Ppid) != r.ParentPID {
			r.Warnings = append(r.Warnings, warning("process_changed", "Process exited, reparented or changed during snapshot"))
			continue
		}
		id := kernelIdentity(a)
		r.Identity = &id
		t := time.Unix(a.Proc.P_starttime.Sec, int64(a.Proc.P_starttime.Usec)*1000)
		r.StartedAt = &t
	}
	return rows, warnings, nil
}
func parseReviewPS(raw string) ([]SnapshotProcess, []review.Warning) {
	rows := []SnapshotProcess{}
	warnings := []review.Warning{}
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) < 5 {
			warnings = append(warnings, warning("partial_discovery", "Malformed process row"))
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		parent, e2 := strconv.Atoi(f[1])
		rss, e3 := strconv.ParseUint(f[3], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || pid <= 0 || parent < 0 || rss > ^uint64(0)/1024 {
			warnings = append(warnings, warning("partial_discovery", "Invalid process metadata"))
			continue
		}
		rss *= 1024
		rows = append(rows, SnapshotProcess{PID: pid, ParentPID: parent, TTY: f[2], RSSBytes: &rss, Name: strings.Join(f[4:], " "), Warnings: []review.Warning{}})
	}
	return rows, warnings
}
func (platformCWD) Lookup(ctx context.Context, id review.ProcessIdentity, parentPID int) (string, error) {
	before, err := kernelProcess(ctx, id.PID)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrProcessChanged, err)
	}
	if kernelIdentity(before) != id || int(before.Eproc.Ppid) != parentPID {
		return "", ErrProcessChanged
	}
	// Bound each optional cwd probe as well as the caller's overall deadline.
	probe, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(probe, "/usr/sbin/lsof", "-a", "-p", strconv.Itoa(id.PID), "-d", "cwd", "-F0n").Output()
	after, e := kernelProcess(ctx, id.PID)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if e != nil || kernelIdentity(after) != id || after.Eproc.Ppid != before.Eproc.Ppid {
		return "", ErrProcessChanged
	}
	if err != nil {
		if probe.Err() != nil {
			return "", probe.Err()
		}
		return "", err
	}
	for _, line := range strings.Split(string(out), "\x00") {
		line = strings.TrimPrefix(line, "\n")
		if strings.HasPrefix(line, "n/") {
			return line[1:], nil
		}
	}
	return "", errors.New("cwd not available")
}
