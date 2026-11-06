package process

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/k7lim/perf-guard/internal/review"
)

// SnapshotProcess is a read-only OS observation. Name is the executable name,
// never command arguments. A nil identity is an explicit failed identity probe.
type SnapshotProcess struct {
	PID       int
	ParentPID int
	Name      string
	TTY       string
	Identity  *review.ProcessIdentity
	StartedAt *time.Time
	RSSBytes  *uint64
	Warnings  []review.Warning
}

type SnapshotSource interface {
	Snapshot(context.Context) ([]SnapshotProcess, []review.Warning, error)
}

// CWDSource must verify that identity still names the same process before and
// after reading cwd, including the expected snapshot parent PID. Implementations
// must support four concurrent calls. ErrProcessChanged invalidates the observation.
type CWDSource interface {
	Lookup(context.Context, review.ProcessIdentity, int) (string, error)
}

var ErrProcessChanged = errors.New("process identity or parent changed during collection")

// ReviewSource implements the frozen review.ProcessSource. Its seams only read;
// this path never calls Manager or any existing process-control methods.
type ReviewSource struct {
	SnapshotSource SnapshotSource
	CWDSource      CWDSource
	Clock          review.Clock
}

var _ review.ProcessSource = (*ReviewSource)(nil)

func NewReviewSource() *ReviewSource {
	return &ReviewSource{SnapshotSource: platformSnapshot{}, CWDSource: platformCWD{}, Clock: review.SystemClock{}}
}

func warning(code, message string) review.Warning {
	return review.Warning{Code: code, Source: "process", Message: message}
}
func (s *ReviewSource) Collect(ctx context.Context) (review.ProcessInventory, error) {
	ctx, cancel := context.WithTimeout(ctx, review.DefaultCollectionTimeout)
	defer cancel()
	clock := s.Clock
	if clock == nil {
		clock = review.SystemClock{}
	}
	inv := review.ProcessInventory{SchemaVersion: review.SchemaVersion, Source: review.SourceStatus{Source: "process", Status: "fresh", Provenance: "os-process-snapshot"}, Processes: []review.ProcessRecord{}, Groups: []review.ProcessGroup{}, Warnings: []review.Warning{}}
	if s.SnapshotSource == nil {
		inv.Source.Status = "unavailable"
		return inv, errors.New("process snapshot provider unavailable")
	}
	raw, warnings, err := s.SnapshotSource.Snapshot(ctx)
	inv.Warnings = append(inv.Warnings, warnings...)
	now := clock.Now()
	inv.Source.CollectedAt = &now
	collectionErr := err
	if err != nil {
		inv.Source.Status = "unavailable"
		inv.Warnings = append(inv.Warnings, warning("source_unavailable", "Process discovery failed"))
		if len(raw) == 0 {
			return inv, fmt.Errorf("process discovery: %w", err)
		}
		inv.Source.Status = "partial"
	}
	byPID := map[int]SnapshotProcess{}
	duplicate := map[int]bool{}
	for _, p := range raw {
		if _, ok := byPID[p.PID]; ok {
			duplicate[p.PID] = true
		}
		byPID[p.PID] = p
	}
	for pid := range duplicate {
		p := byPID[pid]
		p.Identity = nil
		p.Warnings = append(p.Warnings, warning("duplicate_identity", "Duplicate PID observations cannot be correlated"))
		byPID[pid] = p
	}
	// Candidate terminal roots are exact executable matches. Desktop/app-server
	// paths remain observations but can never establish an owner group.
	kinds := map[int]string{}
	agents := map[int]string{}
	for pid, p := range byPID {
		a := filepath.Base(p.Name)
		if a != "codex" && a != "claude" {
			continue
		}
		agents[pid] = a
		kinds[pid] = "background"
		switch {
		case strings.Contains(p.Name, "/ChatGPT.app/"):
			kinds[pid] = "desktop_helper"
		case strings.Contains(p.Name, "/app-server-daemon/"):
			kinds[pid] = "app_server"
		case terminalTTY(p.TTY):
			kinds[pid] = "terminal"
		}
	}
	// Walking the whole ancestry handles intervening shell/runtime wrappers.
	rootFor := func(pid int) int {
		p := byPID[pid]
		tty := p.TTY
		seen := map[int]bool{}
		root := 0
		for cur := pid; cur > 1; {
			if seen[cur] {
				return 0
			}
			seen[cur] = true
			q, ok := byPID[cur]
			if !ok {
				break
			}
			if kinds[cur] == "desktop_helper" || kinds[cur] == "app_server" {
				break
			}
			if !terminalTTY(tty) && terminalTTY(q.TTY) {
				tty = q.TTY
			}
			if terminalTTY(tty) && terminalTTY(q.TTY) && q.TTY != tty {
				break
			}
			if kinds[cur] == "terminal" {
				root = cur
			}
			cur = q.ParentPID
		}
		return root
	}
	pids := []int{}
	for pid := range byPID {
		if _, ok := kinds[pid]; ok || rootFor(pid) != 0 {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	for _, pid := range pids {
		p := byPID[pid]
		kind := kinds[pid]
		if kind == "" {
			kind = "helper"
		}
		root := rootFor(pid)
		if kind == "terminal" && root != pid {
			kind = "helper"
		}
		r := review.ProcessRecord{PID: pid, ParentPID: p.ParentPID, Identity: p.Identity, Agent: agents[pid], Kind: kind, ObservedAt: now, FieldSources: map[string]string{"pid": "os-process-snapshot", "parent_pid": "os-process-snapshot", "tty": "ps-tty", "identity": "os-kernel-start-time", "age_seconds": "os-kernel-start-time", "memory": "ps-resident-rss", "cwd": "not_observed"}, Warnings: append([]review.Warning{}, p.Warnings...), Memory: review.MemoryObservation{ResidentRSSBytes: p.RSSBytes, Provenance: "ps-resident-rss"}}
		if terminalTTY(p.TTY) {
			tty := p.TTY
			r.TTY = &tty
		}
		if p.StartedAt != nil {
			age := now.Sub(*p.StartedAt).Seconds()
			if age >= 0 {
				r.AgeSeconds = &age
			} else {
				r.Warnings = append(r.Warnings, warning("future_observation", "Process start time is in the future"))
			}
		}
		if p.StartedAt == nil {
			r.FieldSources["age_seconds"] = "unavailable"
		}
		if p.RSSBytes == nil {
			r.Warnings = append(r.Warnings, warning("rss_unavailable", "Resident RSS was not observed"))
			r.FieldSources["memory"] = "unavailable"
		}
		if p.Identity == nil {
			r.FieldSources["identity"] = "unavailable"
			r.Warnings = append(r.Warnings, warning("process_disappeared", "Actual process start identity unavailable"))
		}
		if p.ParentPID > 1 {
			if _, ok := byPID[p.ParentPID]; !ok {
				r.Warnings = append(r.Warnings, warning("parent_missing", "Parent absent from snapshot; ancestry is partial"))
			}
		}

		inv.Processes = append(inv.Processes, r)
	}

	// Four bounded workers avoid serial lsof latency while capping helper count.
	if s.CWDSource != nil {
		jobs := make(chan int)
		var workers sync.WaitGroup
		for worker := 0; worker < 4; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for i := range jobs {
					r := &inv.Processes[i]
					if r.Identity != nil {
						if ctx.Err() != nil {
							r.Warnings = append(r.Warnings, warning("deadline_exceeded", "Cwd lookup deadline expired"))
						} else {
							cwd, e := s.CWDSource.Lookup(ctx, *r.Identity, r.ParentPID)
							if e == nil && cwd != "" {
								r.CWD = &cwd
								r.FieldSources["cwd"] = "identity-checked-cwd"
							} else {
								code := "cwd_inaccessible"
								if errors.Is(e, context.DeadlineExceeded) || errors.Is(e, context.Canceled) || ctx.Err() != nil {
									code = "deadline_exceeded"
								}
								if errors.Is(e, ErrProcessChanged) {
									code = "process_changed"
									r.Identity = nil
									r.AgeSeconds = nil
									r.Memory.ResidentRSSBytes = nil
									r.FieldSources["identity"] = "invalidated"
								}
								r.Warnings = append(r.Warnings, warning(code, "Cwd unavailable; process may have exited, changed or denied access"))
							}
						}
					}
				}
			}()
		}
		for i := range inv.Processes {
			jobs <- i
		}
		close(jobs)
		workers.Wait()
	}

	// Assign ownership only after all identity checks; a changed root cannot lend
	// its old identity to surviving children. Members occur exactly once.
	records := map[int]*review.ProcessRecord{}
	for i := range inv.Processes {
		records[inv.Processes[i].PID] = &inv.Processes[i]
	}
	groups := map[int]*review.ProcessGroup{}
	for i := range inv.Processes {
		r := &inv.Processes[i]
		root := rootFor(r.PID)
		rr := records[root]
		if root == 0 || rr == nil || rr.Identity == nil || r.Identity == nil {
			continue
		}
		// Do not cross a disappeared/replaced wrapper while attributing a child.
		valid := true
		for cur := r.PID; cur != root; {
			p := records[cur]
			if p == nil || p.Identity == nil {
				valid = false
				break
			}
			cur = p.ParentPID
		}
		if !valid {
			r.Warnings = append(r.Warnings, warning("ancestry_unknown", "Ownership crosses a changed or unavailable ancestor"))
			continue
		}
		owner := *rr.Identity
		r.Owner = &owner
		g := groups[root]
		if g == nil {
			g = &review.ProcessGroup{Root: owner, Members: []review.ProcessIdentity{}, Memory: review.MemoryObservation{SummedRSSIsEstimate: true, Provenance: "sum-member-resident-rss"}}
			groups[root] = g
		}
		g.Members = append(g.Members, *r.Identity)
	}
	roots := []int{}
	for root := range groups {
		roots = append(roots, root)
	}
	sort.Ints(roots)
	for _, root := range roots {
		g := groups[root]
		sum := uint64(0)
		known := true
		for _, id := range g.Members {
			m := records[id.PID].Memory.ResidentRSSBytes
			if m == nil {
				known = false
				break
			}
			if ^uint64(0)-sum < *m {
				known = false
				inv.Warnings = append(inv.Warnings, warning("partial_discovery", "Summed RSS overflow"))
				break
			}
			sum += *m
		}
		if known {
			g.Memory.ResidentRSSBytes = &sum
		}
		inv.Groups = append(inv.Groups, *g)
	}
	for _, r := range inv.Processes {
		if len(r.Warnings) > 0 {
			inv.Source.Status = "partial"
		}
	}
	if ctx.Err() != nil {
		inv.Warnings = append(inv.Warnings, warning("deadline_exceeded", "Process collection reached its deadline"))
	}
	if len(inv.Warnings) > 0 {
		inv.Source.Status = "partial"
	}
	sortWarnings(inv.Warnings)
	for i := range inv.Processes {
		sortWarnings(inv.Processes[i].Warnings)
	}
	if collectionErr != nil {
		return inv, fmt.Errorf("process discovery: %w", collectionErr)
	}
	return inv, nil
}
func terminalTTY(tty string) bool { return tty != "" && tty != "?" && tty != "??" && tty != "-" }
func sortWarnings(w []review.Warning) {
	sort.Slice(w, func(i, j int) bool {
		if w[i].Code != w[j].Code {
			return w[i].Code < w[j].Code
		}
		return w[i].Message < w[j].Message
	})
}
