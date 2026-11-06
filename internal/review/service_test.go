package review

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

type testClock struct{ at time.Time }

func (c testClock) Now() time.Time { return c.at }

type recordingProcess struct {
	inventory ProcessInventory
	err       error
	calls     []string
	wait      bool
}

func (p *recordingProcess) Collect(ctx context.Context) (ProcessInventory, error) {
	p.calls = append(p.calls, "collect")
	if p.wait {
		<-ctx.Done()
		p.inventory.Source.Status = "partial"
		p.inventory.Warnings = append(p.inventory.Warnings, warn("process", "deadline_exceeded", "optional cwd timed out"))
	}
	return p.inventory, p.err
}

type recordingHerdr struct {
	inventory HerdrInventory
	err       error
	calls     []string
}

func (h *recordingHerdr) Collect(ctx context.Context, t HerdrTarget) (HerdrInventory, error) {
	h.calls = append(h.calls, "collect")
	return h.inventory, h.err
}

type recordingWorktree struct {
	mu    sync.Mutex
	calls []string
	state string
}

func (w *recordingWorktree) Inspect(ctx context.Context, cwd string) WorktreeObservation {
	w.mu.Lock()
	w.calls = append(w.calls, "inspect")
	w.mu.Unlock()
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	return WorktreeObservation{State: w.state, ObservedAt: &at, Provenance: "git-status-read-only", Warnings: []Warning{}}
}
func serviceFixture(t *testing.T) (*Service, *recordingProcess, *recordingHerdr, *recordingWorktree) {
	t.Helper()
	var frozen ReviewResponse
	if e := json.Unmarshal(fixture(t, "healthy-review.json"), &frozen); e != nil {
		t.Fatal(e)
	}
	p := &recordingProcess{inventory: ProcessInventory{SchemaVersion: 1, Source: frozen.Sources[0], Processes: []ProcessRecord{}, Groups: []ProcessGroup{}, Warnings: []Warning{}}}
	h := &recordingHerdr{inventory: HerdrInventory{SchemaVersion: 1, Target: frozen.Target, Source: frozen.Sources[1], Occupants: []HerdrOccupant{}, Warnings: []Warning{}}}
	for _, r := range frozen.Records {
		p.inventory.Processes = append(p.inventory.Processes, ProcessRecord{PID: r.Process.PID, Identity: r.Process, Owner: r.Process, Agent: r.Agent, Kind: "terminal", CWD: r.CWD, AgeSeconds: r.AgeSeconds, ObservedAt: *r.ProcessObservedAt, Memory: r.Memory, FieldSources: map[string]string{"identity": r.ProcessProvenance}, Warnings: []Warning{}})
		i := r.Identity
		h.inventory.Occupants = append(h.inventory.Occupants, HerdrOccupant{ServerID: i.ServerID, SessionID: i.SessionID, WorkspaceID: i.WorkspaceID, TabID: i.TabID, PaneID: i.PaneID, Revision: &i.Revision, Process: r.Process, Correlation: "positive", Lifecycle: r.Lifecycle, Warnings: []Warning{}})
	}
	w := &recordingWorktree{state: "clean"}
	return &Service{Processes: p, Herdr: h, Worktrees: w, Clock: testClock{frozen.CollectedAt}, ControllerIdentity: &ProcessIdentity{PID: 9999, StartID: "other-controller"}}, p, h, w
}
func listAll(t *testing.T, s *Service) ReviewResponse {
	t.Helper()
	r, e := s.List(context.Background(), ListOptions{Limit: 200})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	if e = schema(t, "ReviewResponse").Validate(document(t, raw)); e != nil {
		t.Fatal(e)
	}
	return r
}
func reason(v ReviewRecord, r string) bool {
	for _, x := range v.Reasons {
		if x == r {
			return true
		}
	}
	return false
}
func byPID(t *testing.T, r ReviewResponse, pid int) ReviewRecord {
	t.Helper()
	for _, v := range r.Records {
		if v.Process != nil && v.Process.PID == pid {
			return v
		}
	}
	t.Fatalf("PID %d absent", pid)
	return ReviewRecord{}
}
func TestServiceFrozenHealthyAndPagination(t *testing.T) {
	s, p, h, w := serviceFixture(t)
	r := listAll(t, s)
	if r.Total != 4 || r.ReviewableTotal != 2 {
		t.Fatal(r.Total, r.ReviewableTotal)
	}
	if !byPID(t, r, 101).Reviewable || !byPID(t, r, 102).Reviewable || !reason(byPID(t, r, 103), ReasonWorking) || !reason(byPID(t, r, 104), ReasonBlocked) {
		t.Fatal("lifecycle policy")
	}
	for _, v := range r.Records {
		if v.UserActivity != nil || v.ConversationActivity != nil || v.AgeSeconds == nil || v.ProcessProvenance == "" || v.Worktree.State != "clean" {
			t.Fatal("fact provenance/unknowns")
		}
	}
	ids := []string{}
	for _, v := range r.Records {
		ids = append(ids, v.ID)
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatal("order")
	}
	page, e := s.List(context.Background(), ListOptions{Limit: 1, Offset: 1})
	if e != nil || len(page.Records) != 1 || page.Total != 4 || page.ReviewableTotal != 2 || page.Records[0].ID != r.Records[1].ID {
		t.Fatal("pagination", e)
	}
	for _, calls := range [][]string{p.calls, h.calls} {
		for _, call := range calls {
			if call != "collect" {
				t.Fatal("forbidden call", call)
			}
		}
	}
	for _, call := range w.calls {
		if call != "inspect" {
			t.Fatal("forbidden call", call)
		}
	}
	if len(w.calls) != 2 {
		t.Fatal("cwd probes were not deduplicated")
	}
}
func TestServiceExclusionsAndFreshness(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Service, *recordingProcess, *recordingHerdr)
		pid    int
		want   string
	}{
		{"controller", func(s *Service, p *recordingProcess, h *recordingHerdr) {
			s.ControllerIdentity = h.inventory.Occupants[0].Process
		}, 101, ReasonCurrent},
		{"unknown", func(s *Service, p *recordingProcess, h *recordingHerdr) {
			h.inventory.Occupants[0].Lifecycle.State = "unknown"
		}, 101, ReasonUnknown},
		{"source stale", func(s *Service, p *recordingProcess, h *recordingHerdr) { h.inventory.Source.Status = "stale" }, 101, ReasonStale},
		{"lifecycle stale", func(s *Service, p *recordingProcess, h *recordingHerdr) {
			x := s.Clock.Now().Add(-31 * time.Second)
			h.inventory.Occupants[0].Lifecycle.ObservedAt = &x
		}, 101, ReasonStale},
		{"process stale", func(s *Service, p *recordingProcess, h *recordingHerdr) {
			p.inventory.Processes[0].ObservedAt = s.Clock.Now().Add(-31 * time.Second)
		}, 101, ReasonStale},
		{"future process", func(s *Service, p *recordingProcess, h *recordingHerdr) {
			p.inventory.Processes[0].ObservedAt = s.Clock.Now().Add(time.Second)
		}, 101, ReasonStale},
		{"source future", func(s *Service, p *recordingProcess, h *recordingHerdr) {
			x := s.Clock.Now().Add(time.Second)
			h.inventory.Source.CollectedAt = &x
		}, 101, ReasonStale},
		{"source unknown time", func(s *Service, p *recordingProcess, h *recordingHerdr) { h.inventory.Source.CollectedAt = nil }, 101, ReasonStale},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, p, h, _ := serviceFixture(t)
			c.mutate(s, p, h)
			r := listAll(t, s)
			v := byPID(t, r, c.pid)
			if v.Reviewable || !reason(v, c.want) {
				t.Fatal(v.Reasons)
			}
		})
	}
	t.Run("helper controller ancestry", func(t *testing.T) {
		s, p, _, _ := serviceFixture(t)
		owner := p.inventory.Processes[0].Identity
		id := ProcessIdentity{PID: 777, StartID: "cli-helper"}
		p.inventory.Processes = append(p.inventory.Processes, ProcessRecord{PID: 777, Identity: &id, Owner: owner, Kind: "helper"})
		s.ControllerIdentity = nil
		s.ControllerPID = 777
		r := listAll(t, s)
		if !reason(byPID(t, r, 101), ReasonCurrent) {
			t.Fatal("controller owner not excluded")
		}
	})
	t.Run("unknown controller visible", func(t *testing.T) {
		s, _, _, _ := serviceFixture(t)
		s.ControllerIdentity = nil
		r := listAll(t, s)
		if len(r.Warnings) == 0 || r.Warnings[0].Code != "controller_unknown" {
			t.Fatal(r.Warnings)
		}
	})
}
func TestServiceExactIdentityAndDuplicates(t *testing.T) {
	t.Run("pid reused", func(t *testing.T) {
		s, p, _, _ := serviceFixture(t)
		p.inventory.Processes[0].Identity = &ProcessIdentity{PID: 101, StartID: "replacement"}
		r := listAll(t, s)
		for _, v := range r.Records {
			if v.Identity != nil && v.Identity.Process.PID == 101 {
				t.Fatal("PID reuse joined")
			}
		}
	})
	t.Run("unknown correlation", func(t *testing.T) {
		s, _, h, _ := serviceFixture(t)
		h.inventory.Occupants[0].Correlation = "unknown"
		r := listAll(t, s)
		if r.ReviewableTotal != 1 {
			t.Fatal(r.ReviewableTotal)
		}
	})
	t.Run("duplicate exact", func(t *testing.T) {
		s, _, h, _ := serviceFixture(t)
		h.inventory.Occupants = append(h.inventory.Occupants, h.inventory.Occupants[0])
		r := listAll(t, s)
		v := byPID(t, r, 101)
		if v.Reviewable || !reason(v, ReasonDuplicate) || r.Total != 4 {
			t.Fatal("duplicate winner")
		}
	})
	t.Run("conflicting pane", func(t *testing.T) {
		s, _, h, _ := serviceFixture(t)
		x := h.inventory.Occupants[0]
		revision := "new-occupant"
		x.Revision = &revision
		h.inventory.Occupants = append(h.inventory.Occupants, x)
		r := listAll(t, s)
		for _, v := range r.Records {
			if v.Process != nil && v.Process.PID == 101 && v.Reviewable {
				t.Fatal("ambiguous occupant won")
			}
		}
	})
	t.Run("same public pane other server", func(t *testing.T) {
		s, _, h, _ := serviceFixture(t)
		h.inventory.Occupants[1].PaneID = h.inventory.Occupants[0].PaneID
		h.inventory.Occupants[1].ServerID = "server-b"
		r := listAll(t, s)
		if r.ReviewableTotal != 2 || byPID(t, r, 101).ID == byPID(t, r, 102).ID {
			t.Fatal("cross server collision")
		}
	})
	t.Run("deterministic conflict order", func(t *testing.T) {
		s, _, h, _ := serviceFixture(t)
		x := h.inventory.Occupants[0]
		x.Lifecycle.State = "working"
		h.inventory.Occupants = append(h.inventory.Occupants, x)
		a := listAll(t, s)
		for i, j := 0, len(h.inventory.Occupants)-1; i < j; i, j = i+1, j-1 {
			h.inventory.Occupants[i], h.inventory.Occupants[j] = h.inventory.Occupants[j], h.inventory.Occupants[i]
		}
		b := listAll(t, s)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("input order changed collision result")
		}
	})
}
func TestServiceChildWorkPartialAndUnknowns(t *testing.T) {
	s, p, h, w := serviceFixture(t)
	root := *p.inventory.Processes[0].Identity
	child := *p.inventory.Processes[3].Identity
	p.inventory.Processes[3].Kind = "helper"
	p.inventory.Processes[3].Owner = &root
	p.inventory.Groups = []ProcessGroup{{Root: root, Members: []ProcessIdentity{root, child}, Memory: MemoryObservation{SummedRSSIsEstimate: true, Provenance: "sum-member-resident-rss"}}}
	w.state = "dirty"
	r := listAll(t, s)
	v := byPID(t, r, 101)
	if v.Reviewable || !reason(v, ReasonChildWork) || v.ChildWork.State != "working" || v.Worktree.State != "dirty" || !v.Memory.SummedRSSIsEstimate {
		t.Fatal(v.Reasons)
	}
	h.inventory.Source.Status = "partial"
	h.inventory.Occupants[3].Lifecycle.State = "done"
	r = listAll(t, s)
	if !byPID(t, r, 101).Reviewable {
		t.Fatal("partial source fresh record lost")
	}
	h.inventory.Source.Status = "unavailable"
	h.inventory.Occupants = nil
	r = listAll(t, s)
	if r.ReviewableTotal != 0 || r.Total == 0 {
		t.Fatal("outage became healthy empty")
	}
	p.err = errors.New("discovery denied")
	if _, e := s.List(context.Background(), ListOptions{Limit: 50}); e == nil {
		t.Fatal("required failure hidden")
	}
}
func TestServicePlanSelectionsRecollect(t *testing.T) {
	s, _, h, _ := serviceFixture(t)
	r := listAll(t, s)
	idle := byPID(t, r, 101)
	working := byPID(t, r, 103)
	resume := "native-reference"
	h.inventory.Occupants[0].ResumeReference = &resume
	plan, e := s.Plan(context.Background(), HerdrTarget{}, []string{idle.ID, "missing", idle.ID, working.ID})
	if e != nil {
		t.Fatal(e)
	}
	if plan.ExecutionSupported || len(plan.Selections) != 4 {
		t.Fatal("execution capability")
	}
	for n, status := range []string{"reviewable", "missing", "duplicate", "excluded"} {
		if plan.Selections[n].Status != status || !plan.Selections[n].PreserveShell {
			t.Fatal(plan.Selections[n])
		}
	}
	if !plan.Selections[0].RecoveryAvailable {
		t.Fatal("native context lost")
	}
	raw, _ := json.Marshal(plan)
	if e = schema(t, "ReviewPlan").Validate(document(t, raw)); e != nil {
		t.Fatal(e)
	}
	revision := "replacement"
	h.inventory.Occupants[0].Revision = &revision
	plan, e = s.Plan(context.Background(), HerdrTarget{}, []string{idle.ID})
	if e != nil || plan.Selections[0].Status != "missing" || plan.Selections[0].Record != nil {
		t.Fatal("selection substituted", e)
	}
	if len(h.calls) != 3 {
		t.Fatal("plan did not recollect", h.calls)
	}
}
func TestServiceEmptyValidationAndOptionalDeadline(t *testing.T) {
	s, p, h, _ := serviceFixture(t)
	p.inventory.Processes = nil
	h.inventory.Occupants = nil
	r := listAll(t, s)
	if r.Total != 0 || r.Records == nil {
		t.Fatal("empty")
	}
	for _, o := range []ListOptions{{Limit: 0}, {Limit: 201}, {Limit: 1, Offset: -1}} {
		if _, e := s.List(context.Background(), o); e == nil {
			t.Fatal("invalid pagination")
		}
	}
	for _, ids := range [][]string{nil, {""}, {"bad\nID"}} {
		if _, e := s.Plan(context.Background(), HerdrTarget{}, ids); e == nil {
			t.Fatal("invalid selection")
		}
	}
	s, p, _, _ = serviceFixture(t)
	p.wait = true
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, e := s.List(ctx, ListOptions{Limit: 50})
	if e != nil || r.Total == 0 || time.Since(start) > time.Second {
		t.Fatal("optional deadline became discovery failure", e)
	}
	if len(r.Warnings) == 0 {
		t.Fatal("partial warning lost")
	}
}
func TestServiceSharedFixtureActivity(t *testing.T) {
	// Explicit user/conversation observations survive without confusing process age.
	s, _, h, _ := serviceFixture(t)
	at := s.Clock.Now().Add(-time.Hour)
	h.inventory.Occupants[0].UserActivity = &UserActivityObservation{At: at, Provenance: "explicit-user-event"}
	h.inventory.Occupants[0].ConversationActivity = &ConversationActivityObservation{At: at, Provenance: "native-conversation-event"}
	v := byPID(t, listAll(t, s), 101)
	if v.UserActivity == nil || v.ConversationActivity == nil || v.UserActivity.Provenance == v.ConversationActivity.Provenance {
		t.Fatal("activity conflated")
	}
	future := s.Clock.Now().Add(time.Second)
	h.inventory.Occupants[0].UserActivity = &UserActivityObservation{At: future, Provenance: "bad-clock"}
	h.inventory.Occupants[0].ConversationActivity = &ConversationActivityObservation{At: future, Provenance: "bad-clock"}
	v = byPID(t, listAll(t, s), 101)
	if v.UserActivity != nil || v.ConversationActivity != nil || len(v.Warnings) == 0 {
		t.Fatal("future activity became fact")
	}
	// Frozen dirty and changed-occupant payloads remain directly consumable by UI.
	for _, name := range []string{"dirty-review.json", "changed-occupant-review.json", "stale-partial-review.json"} {
		b, e := os.ReadFile(contractDir + "/fixtures/" + name)
		if e != nil {
			t.Fatal(e)
		}
		if e = schema(t, "ReviewResponse").Validate(document(t, b)); e != nil {
			t.Fatal(e)
		}
	}
}
