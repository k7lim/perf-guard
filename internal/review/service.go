package review

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Service joins read-only provider observations. Providers must honor context;
// no control capability is accepted by this service.
type Service struct {
	Processes     ProcessSource
	Herdr         HerdrSource
	Worktrees     WorktreeSource
	Clock         Clock
	ControllerPID int
	// An injected exact controller identity is useful for callers with native
	// context and tests. Otherwise ControllerPID is resolved through the inventory.
	ControllerIdentity *ProcessIdentity
}

type ListOptions struct {
	Target        HerdrTarget
	Limit, Offset int
}

func ValidatePagination(limit, offset int) error {
	if limit < 1 || limit > MaxLimit || offset < 0 {
		return fmt.Errorf("limit must be 1..%d and offset nonnegative", MaxLimit)
	}
	return nil
}
func ValidateSelections(ids []string) error {
	if len(ids) == 0 {
		return errors.New("at least one --session ID is required")
	}
	for _, id := range ids {
		if id == "" || strings.IndexFunc(id, unicode.IsControl) >= 0 {
			return errors.New("session IDs must be nonempty and contain no control characters")
		}
	}
	return nil
}
func (s *Service) List(ctx context.Context, opts ListOptions) (ReviewResponse, error) {
	if err := ValidatePagination(opts.Limit, opts.Offset); err != nil {
		return ReviewResponse{}, err
	}
	r, err := s.collect(ctx, opts.Target)
	if err != nil {
		return r, err
	}
	r.Limit = opts.Limit
	r.Offset = opts.Offset
	start := min(opts.Offset, len(r.Records))
	end := start + min(opts.Limit, len(r.Records)-start)
	r.Records = r.Records[start:end]
	return r, nil
}
func (s *Service) Plan(ctx context.Context, target HerdrTarget, ids []string) (ReviewPlan, error) {
	if err := ValidateSelections(ids); err != nil {
		return ReviewPlan{}, err
	}
	r, err := s.collect(ctx, target)
	if err != nil {
		return ReviewPlan{}, err
	}
	p := ReviewPlan{SchemaVersion: SchemaVersion, CollectedAt: r.CollectedAt, Target: r.Target, ExecutionSupported: false, Selections: []PlanSelection{}, Sources: r.Sources, Warnings: r.Warnings}
	records := map[string]ReviewRecord{}
	for _, v := range r.Records {
		records[v.ID] = v
	}
	seen := map[string]bool{}
	for _, id := range ids {
		x := PlanSelection{RequestedID: id, Status: "missing", Reasons: []string{ReasonMissing}, PreserveShell: true}
		if seen[id] {
			x.Status = "duplicate"
			x.Reasons = []string{ReasonRepeated}
		} else if v, ok := records[id]; ok {
			x.Record = &v
			x.Status = "excluded"
			if v.Reviewable {
				x.Status = "reviewable"
			}
			x.Reasons = append([]string{}, v.Reasons...)
			x.RecoveryAvailable = v.ResumeReference != nil && *v.ResumeReference != ""
		}
		seen[id] = true
		p.Selections = append(p.Selections, x)
	}
	return p, nil
}
func warn(source, code, message string) Warning {
	return Warning{Source: source, Code: code, Message: message}
}
func unavailable(source string) SourceStatus {
	return SourceStatus{Source: source, Status: "unavailable", Provenance: "not-observed"}
}
func (s *Service) collect(parent context.Context, target HerdrTarget) (ReviewResponse, error) {
	ctx, cancel := context.WithTimeout(parent, DefaultCollectionTimeout)
	defer cancel()
	clock := s.Clock
	if clock == nil {
		clock = SystemClock{}
	}
	// Leave time inside the single overall deadline for cooperative optional
	// probes to return their partial inventory and for worktree inspection.
	providerBudget := 4 * time.Second
	if d, ok := ctx.Deadline(); ok {
		if remaining := time.Until(d) * 4 / 5; remaining < providerBudget {
			providerBudget = remaining
		}
	}
	providerCtx, stopProviders := context.WithTimeout(ctx, providerBudget)
	defer stopProviders()
	type processResult struct {
		v ProcessInventory
		e error
	}
	type herdrResult struct {
		v HerdrInventory
		e error
	}
	pc := make(chan processResult, 1)
	hc := make(chan herdrResult, 1)
	go func() {
		if s.Processes == nil {
			pc <- processResult{e: errors.New("process source unavailable")}
			return
		}
		v, e := s.Processes.Collect(providerCtx)
		pc <- processResult{v, e}
	}()
	go func() {
		if s.Herdr == nil {
			hc <- herdrResult{v: HerdrInventory{Target: target, Source: unavailable("herdr"), Warnings: []Warning{warn("herdr", "source_unavailable", "Herdr source unavailable")}}}
			return
		}
		v, e := s.Herdr.Collect(providerCtx, target)
		hc <- herdrResult{v, e}
	}()
	var p ProcessInventory
	var h HerdrInventory
	var pe, he error
	gotP, gotH := false, false
	for !gotP || !gotH {
		select {
		case x := <-pc:
			p, pe, gotP = x.v, x.e, true
		case x := <-hc:
			h, he, gotH = x.v, x.e, true
		case <-ctx.Done():
			if !gotP {
				select {
				case x := <-pc:
					p, pe, gotP = x.v, x.e, true
				default:
					pe = ctx.Err()
					gotP = true
				}
			}
			if !gotH {
				select {
				case x := <-hc:
					h, he, gotH = x.v, x.e, true
				default:
					he = ctx.Err()
					gotH = true
				}
			}
		}
	}
	if pe != nil {
		return ReviewResponse{}, fmt.Errorf("process discovery unavailable: %w", pe)
	}
	if p.SchemaVersion != SchemaVersion || p.Source.Status == "unavailable" || p.Source.Source != "process" {
		return ReviewResponse{}, errors.New("process discovery unavailable or incompatible")
	}
	if h.Source.Source == "" {
		h.Source = unavailable("herdr")
	}
	if h.Target.Endpoint == "" || h.Target.SessionID == "" {
		h.Target = target
	}
	if h.Target.Endpoint == "" || h.Target.SessionID == "" {
		h.Target = HerdrTarget{Endpoint: "unavailable", SessionID: "unavailable"}
	}
	if he != nil || h.SchemaVersion != SchemaVersion {
		h.Source.Status = "unavailable"
		h.Warnings = append(h.Warnings, warn("herdr", "source_unavailable", "Herdr collection unavailable or incompatible"))
	}
	now := clock.Now()
	r := ReviewResponse{SchemaVersion: SchemaVersion, CollectedAt: now, Target: h.Target, Sources: []SourceStatus{}, Records: []ReviewRecord{}, Limit: DefaultLimit, Warnings: []Warning{}}
	r.Warnings = append(r.Warnings, p.Warnings...)
	r.Warnings = append(r.Warnings, h.Warnings...)
	p.Source = normalizeSource(p.Source, now, &r.Warnings)
	h.Source = normalizeSource(h.Source, now, &r.Warnings)
	r.Sources = []SourceStatus{h.Source, p.Source}
	roots := map[ProcessIdentity]ProcessRecord{}
	knownProcesses := map[ProcessIdentity]ProcessRecord{}
	groups := map[ProcessIdentity]ProcessGroup{}
	pidCounts := map[int]int{}
	for _, v := range p.Processes {
		pidCounts[v.PID]++
	}
	for _, v := range p.Processes {
		if v.Identity != nil && v.Identity.PID == v.PID && pidCounts[v.PID] == 1 {
			knownProcesses[*v.Identity] = v
		}
	}
	for _, v := range p.Processes {
		if v.Kind == "terminal" && v.Identity != nil && v.Identity.PID == v.PID && pidCounts[v.PID] == 1 {
			roots[*v.Identity] = v
		}
	}
	for _, g := range p.Groups {
		groups[g.Root] = g
	}
	controller := s.ControllerIdentity
	if controller == nil && s.ControllerPID > 0 {
		for _, v := range p.Processes {
			if v.PID == s.ControllerPID && v.Identity != nil && pidCounts[v.PID] == 1 {
				if v.Owner != nil {
					controller = v.Owner
				} else if v.Kind == "terminal" {
					controller = v.Identity
				}
				break
			}
		}
	}
	if controller == nil {
		r.Warnings = append(r.Warnings, warn("review", "controller_unknown", "Current controlling terminal identity could not be established"))
	}
	// Count scoped panes and process assignments before joining. No winner is
	// selected for duplicate/contradictory observations.
	paneCounts := map[string]int{}
	processCounts := map[ProcessIdentity]int{}
	for _, o := range h.Occupants {
		paneCounts[paneKey(h.Target, o)]++
		if o.Process != nil && o.Correlation == "positive" {
			processCounts[*o.Process]++
		}
	}
	used := map[ProcessIdentity]bool{}
	for _, o := range h.Occupants {
		v := blankRecord()
		v.Lifecycle = o.Lifecycle
		v.UserActivity = o.UserActivity
		v.ConversationActivity = o.ConversationActivity
		v.ResumeReference = o.ResumeReference
		if v.UserActivity != nil && v.UserActivity.At.After(now) {
			v.UserActivity = nil
			v.Warnings = append(v.Warnings, warn("herdr", "future_observation", "User activity time is in the future"))
		}
		if v.ConversationActivity != nil && v.ConversationActivity.At.After(now) {
			v.ConversationActivity = nil
			v.Warnings = append(v.Warnings, warn("herdr", "future_observation", "Conversation activity time is in the future"))
		}
		v.Warnings = append(v.Warnings, o.Warnings...)
		v.ID = ExcludedID("herdr", h.Target.Endpoint, o.ServerID, o.SessionID, o.WorkspaceID, o.TabID, o.PaneID)
		duplicate := paneCounts[paneKey(h.Target, o)] > 1 || (o.Process != nil && processCounts[*o.Process] > 1)
		root, matched := ProcessRecord{}, false
		if o.Process != nil {
			root, matched = roots[*o.Process]
		}
		positive := matched && o.Correlation == "positive" && o.Revision != nil && *o.Revision != "" && o.ServerID != "" && o.SessionID == h.Target.SessionID && o.WorkspaceID != "" && o.TabID != "" && o.PaneID != "" && h.Target.Endpoint != "unavailable"
		if positive {
			copyProcess(&v, root, groups)
			used[*root.Identity] = true
			identity := OccupantIdentity{Endpoint: h.Target.Endpoint, ServerID: o.ServerID, SessionID: o.SessionID, WorkspaceID: o.WorkspaceID, TabID: o.TabID, PaneID: o.PaneID, Revision: *o.Revision, Process: *o.Process}
			v.Identity = &identity
			v.ID = ReviewID(identity)
		} else {
			v.Reasons = append(v.Reasons, ReasonUncorrelated, ReasonIdentity)
		}
		if duplicate {
			v.Reasons = append(v.Reasons, ReasonDuplicate)
			v.Warnings = append(v.Warnings, warn("review", "duplicate_identity", "Conflicting or duplicate identity observations are excluded"))
		}
		if !sourceFresh(h.Source, now) || !sourceFresh(p.Source, now) || !fresh(v.Lifecycle.ObservedAt, now) || (positive && !fresh(v.ProcessObservedAt, now)) {
			v.Reasons = append(v.Reasons, ReasonStale)
		}
		if v.Lifecycle.ObservedAt != nil && v.Lifecycle.ObservedAt.After(now) {
			v.Warnings = append(v.Warnings, warn("herdr", "future_observation", "Lifecycle observation is in the future"))
		}
		if positive && v.ProcessObservedAt != nil && v.ProcessObservedAt.After(now) {
			v.Warnings = append(v.Warnings, warn("process", "future_observation", "Process observation is in the future"))
		}
		if positive && controller != nil && *controller == *root.Identity {
			v.Reasons = append(v.Reasons, ReasonCurrent)
		}
		switch v.Lifecycle.State {
		case "idle", "done":
		case "working":
			v.Reasons = append(v.Reasons, ReasonWorking)
		case "blocked":
			v.Reasons = append(v.Reasons, ReasonBlocked)
		default:
			v.Lifecycle.State = "unknown"
			v.Reasons = append(v.Reasons, ReasonUnknown)
		}
		// A separately correlated descendant with fresh working lifecycle is direct
		// child-work evidence. Missing evidence remains unknown.
		if positive && sourceFresh(h.Source, now) && sourceFresh(p.Source, now) {
			for _, child := range h.Occupants {
				if child.Process == nil || child.Correlation != "positive" || child.Lifecycle.State != "working" || !fresh(child.Lifecycle.ObservedAt, now) {
					continue
				}
				observed, ok := knownProcesses[*child.Process]
				if !ok || !fresh(&observed.ObservedAt, now) {
					continue
				}
				for _, member := range groups[*root.Identity].Members {
					if member != *root.Identity && member == *child.Process {
						if v.ChildWork.ObservedAt == nil || child.Lifecycle.ObservedAt.After(*v.ChildWork.ObservedAt) {
							v.ChildWork = ChildWorkObservation{State: "working", ObservedAt: child.Lifecycle.ObservedAt, Provenance: "herdr-correlated-descendant"}
						}
						v.Reasons = append(v.Reasons, ReasonChildWork)
					}
				}
			}
		}
		finalizeRecord(&v)
		r.Records = append(r.Records, v)
	}
	for _, root := range p.Processes {
		if root.Kind != "terminal" || (root.Identity != nil && used[*root.Identity]) {
			continue
		}
		v := blankRecord()
		copyProcess(&v, root, groups)
		start := ""
		if root.Identity != nil {
			start = root.Identity.StartID
		}
		v.ID = ExcludedID("process", strconv.Itoa(root.PID), start)
		v.Reasons = []string{ReasonUncorrelated}
		if root.Identity == nil || pidCounts[root.PID] > 1 {
			v.Reasons = append(v.Reasons, ReasonIdentity)
		}
		if !sourceFresh(p.Source, now) || !fresh(v.ProcessObservedAt, now) {
			v.Reasons = append(v.Reasons, ReasonStale)
		}
		if controller != nil && root.Identity != nil && *controller == *root.Identity {
			v.Reasons = append(v.Reasons, ReasonCurrent)
		}
		v.Reasons = append(v.Reasons, ReasonUnknown)
		finalizeRecord(&v)
		r.Records = append(r.Records, v)
	}
	// Collapse collisions explicitly; conflicting observations lose noncommon
	// facts instead of letting input order pick one occupant.
	byID := map[string]ReviewRecord{}
	for _, v := range r.Records {
		if old, ok := byID[v.ID]; ok {
			v = collision(old, v)
			r.Warnings = append(r.Warnings, warn("review", "duplicate_identity", "Duplicate visible identity collapsed and excluded"))
		}
		byID[v.ID] = v
	}
	r.Records = []ReviewRecord{}
	for _, v := range byID {
		r.Records = append(r.Records, v)
	}
	s.probeWorktrees(ctx, r.Records)
	sort.Slice(r.Records, func(i, j int) bool { return r.Records[i].ID < r.Records[j].ID })
	for i := range r.Records {
		sortWarnings(r.Records[i].Warnings)
		if r.Records[i].Reviewable {
			r.ReviewableTotal++
		}
	}
	r.Total = len(r.Records)
	sortWarnings(r.Warnings)
	return r, nil
}
func blankRecord() ReviewRecord {
	return ReviewRecord{Agent: "unknown", ProcessProvenance: "not-observed", Memory: MemoryObservation{Provenance: "not-observed"}, Lifecycle: LifecycleObservation{State: "unknown", Provenance: "not-observed"}, ChildWork: ChildWorkObservation{State: "unknown", Provenance: "not-observed"}, Worktree: WorktreeObservation{State: "unknown", Provenance: "not-observed", Warnings: []Warning{}}, Reasons: []string{}, Warnings: []Warning{}}
}
func copyProcess(v *ReviewRecord, p ProcessRecord, groups map[ProcessIdentity]ProcessGroup) {
	v.Process = p.Identity
	v.Agent = p.Agent
	v.CWD = p.CWD
	v.AgeSeconds = p.AgeSeconds
	t := p.ObservedAt
	v.ProcessObservedAt = &t
	v.ProcessProvenance = p.FieldSources["identity"]
	if v.ProcessProvenance == "" {
		v.ProcessProvenance = "process-provider"
	}
	v.Memory = p.Memory
	v.Warnings = append(v.Warnings, p.Warnings...)
	if p.Identity != nil {
		if g, ok := groups[*p.Identity]; ok {
			v.Memory = g.Memory
		}
	}
	if p.CWD != nil {
		project := filepath.Base(*p.CWD)
		v.Project = &project
	}
}
func paneKey(t HerdrTarget, o HerdrOccupant) string {
	return ExcludedID("herdr", t.Endpoint, o.ServerID, o.SessionID, o.WorkspaceID, o.TabID, o.PaneID)
}
func fresh(t *time.Time, now time.Time) bool {
	return t != nil && !t.After(now) && now.Sub(*t) <= DefaultFreshness
}
func sourceFresh(s SourceStatus, now time.Time) bool {
	return (s.Status == "fresh" || s.Status == "partial") && fresh(s.CollectedAt, now)
}
func normalizeSource(s SourceStatus, now time.Time, w *[]Warning) SourceStatus {
	if s.Status == "fresh" || s.Status == "partial" {
		if !fresh(s.CollectedAt, now) {
			s.Status = "stale"
			if s.CollectedAt != nil && s.CollectedAt.After(now) {
				*w = append(*w, warn(s.Source, "future_observation", "Source observation is in the future"))
			}
		}
	}
	return s
}
func finalizeRecord(v *ReviewRecord) {
	v.Reasons = unique(v.Reasons)
	v.Reviewable = len(v.Reasons) == 0
	if v.Reviewable {
		if v.Lifecycle.State == "idle" {
			v.Reasons = []string{ReasonIdle}
		} else {
			v.Reasons = []string{ReasonDone}
		}
	}
}
func unique(a []string) []string {
	sort.Strings(a)
	r := []string{}
	for _, v := range a {
		if len(r) == 0 || r[len(r)-1] != v {
			r = append(r, v)
		}
	}
	return r
}
func collision(a, b ReviewRecord) ReviewRecord {
	v := blankRecord()
	v.ID = a.ID
	if reflect.DeepEqual(a.Identity, b.Identity) {
		v.Identity = a.Identity
	}
	if reflect.DeepEqual(a.Process, b.Process) {
		v.Process = a.Process
	}
	if a.Agent == b.Agent {
		v.Agent = a.Agent
	}
	if sameString(a.CWD, b.CWD) {
		v.CWD = a.CWD
	}
	if sameString(a.Project, b.Project) {
		v.Project = a.Project
	}
	if reflect.DeepEqual(a.AgeSeconds, b.AgeSeconds) {
		v.AgeSeconds = a.AgeSeconds
	}
	if reflect.DeepEqual(a.ProcessObservedAt, b.ProcessObservedAt) {
		v.ProcessObservedAt = a.ProcessObservedAt
	}
	if a.ProcessProvenance == b.ProcessProvenance {
		v.ProcessProvenance = a.ProcessProvenance
	}
	if reflect.DeepEqual(a.Memory, b.Memory) {
		v.Memory = a.Memory
	}
	if reflect.DeepEqual(a.Lifecycle, b.Lifecycle) {
		v.Lifecycle = a.Lifecycle
	}
	if reflect.DeepEqual(a.UserActivity, b.UserActivity) {
		v.UserActivity = a.UserActivity
	}
	if reflect.DeepEqual(a.ConversationActivity, b.ConversationActivity) {
		v.ConversationActivity = a.ConversationActivity
	}
	if reflect.DeepEqual(a.ChildWork, b.ChildWork) {
		v.ChildWork = a.ChildWork
	}
	if sameString(a.ResumeReference, b.ResumeReference) {
		v.ResumeReference = a.ResumeReference
	}
	reasons := append(append([]string{}, a.Reasons...), b.Reasons...)
	v.Reasons = []string{ReasonDuplicate}
	for _, r := range reasons {
		if r != ReasonIdle && r != ReasonDone {
			v.Reasons = append(v.Reasons, r)
		}
	}
	v.Reasons = unique(v.Reasons)
	v.Warnings = append(append([]Warning{}, a.Warnings...), b.Warnings...)
	v.Warnings = append(v.Warnings, warn("review", "duplicate_identity", "Duplicate visible identity collapsed and excluded"))
	return v
}
func sameString(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func sortWarnings(w []Warning) {
	sort.Slice(w, func(i, j int) bool {
		if w[i].Source != w[j].Source {
			return w[i].Source < w[j].Source
		}
		if w[i].Code != w[j].Code {
			return w[i].Code < w[j].Code
		}
		return w[i].Message < w[j].Message
	})
}
func (s *Service) probeWorktrees(ctx context.Context, records []ReviewRecord) {
	byCWD := map[string][]int{}
	for i, v := range records {
		if v.CWD != nil {
			byCWD[*v.CWD] = append(byCWD[*v.CWD], i)
		}
	}
	if s.Worktrees == nil {
		return
	}
	type result struct {
		cwd string
		v   WorktreeObservation
	}
	jobs := make(chan string)
	out := make(chan result, len(byCWD))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cwd := range jobs {
				out <- result{cwd, s.Worktrees.Inspect(ctx, cwd)}
			}
		}()
	}
	go func() {
		for cwd := range byCWD {
			jobs <- cwd
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()
	for x := range out {
		for _, i := range byCWD[x.cwd] {
			records[i].Worktree = x.v
		}
	}
}
