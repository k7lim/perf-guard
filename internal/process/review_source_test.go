package process

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"bytes"
	"github.com/k7lim/perf-guard/internal/review"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"path/filepath"
)

type fixtureClock struct{ at time.Time }

func (c fixtureClock) Now() time.Time { return c.at }

type fixtureSnapshot struct {
	rows     []SnapshotProcess
	warnings []review.Warning
	err      error
	calls    []string
}

func (s *fixtureSnapshot) Snapshot(ctx context.Context) ([]SnapshotProcess, []review.Warning, error) {
	s.calls = append(s.calls, "snapshot")
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	return s.rows, s.warnings, s.err
}

type fixtureCWD struct {
	mu      sync.Mutex
	calls   []string
	changed map[int]bool
	denied  bool
	wait    bool
}

func (c *fixtureCWD) Lookup(ctx context.Context, id review.ProcessIdentity, parentPID int) (string, error) {
	c.mu.Lock()
	c.calls = append(c.calls, "cwd")
	c.mu.Unlock()
	if c.wait {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if c.changed[id.PID] {
		return "", ErrProcessChanged
	}
	if c.denied {
		return "", errors.New("denied")
	}
	return "/example/project", nil
}
func row(pid, parent int, name, tty string) SnapshotProcess {
	id := review.ProcessIdentity{PID: pid, StartID: "fixture-start-" + time.Unix(int64(pid), 0).Format(time.RFC3339)}
	started := time.Unix(100, 0)
	rss := uint64(pid)
	return SnapshotProcess{PID: pid, ParentPID: parent, Name: name, TTY: tty, Identity: &id, StartedAt: &started, RSSBytes: &rss, Warnings: []review.Warning{}}
}
func collectFixture(t *testing.T, rows []SnapshotProcess, cwd *fixtureCWD) review.ProcessInventory {
	t.Helper()
	s := &fixtureSnapshot{rows: rows}
	p := &ReviewSource{SnapshotSource: s, CWDSource: cwd, Clock: fixtureClock{time.Unix(200, 0)}}
	inv, e := p.Collect(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(s.calls, []string{"snapshot"}) {
		t.Fatal(s.calls)
	}
	for _, call := range cwd.calls {
		if call != "cwd" {
			t.Fatal("forbidden control call", call)
		}
	}
	rawJSON, e := json.Marshal(inv)
	if e != nil {
		t.Fatal(e)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, e := compiler.Compile(filepath.Join("../../contracts/review-v1/ProcessInventory.schema.json"))
	if e != nil {
		t.Fatal(e)
	}
	doc, e := jsonschema.UnmarshalJSON(bytes.NewReader(rawJSON))
	if e != nil {
		t.Fatal(e)
	}
	if e = schema.Validate(doc); e != nil {
		t.Fatal(e)
	}
	return inv
}
func TestReviewGrouping(t *testing.T) {
	rows := []SnapshotProcess{row(10, 1, "/bin/codex", "ttys1"), row(11, 10, "/bin/sh", "ttys1"), row(12, 11, "/bin/claude", "ttys1"), row(13, 12, "/bin/node", "??"), row(20, 10, "/bin/codex", "ttys2"), row(21, 20, "/bin/python", "ttys2"), row(22, 20, "/bin/node", "??"), row(30, 1, "/Applications/ChatGPT.app/codex", "ttys3"), row(31, 1, "/app-server-daemon/codex", "ttys4"), row(40, 999, "/bin/claude", "ttys5")}
	inv := collectFixture(t, rows, &fixtureCWD{})
	if len(inv.Groups) != 3 {
		t.Fatalf("groups=%+v", inv.Groups)
	}
	want := [][]int{{10, 11, 12, 13}, {20, 21, 22}, {40}}
	seen := map[int]bool{}
	for i, g := range inv.Groups {
		if !g.Memory.SummedRSSIsEstimate || g.Memory.PhysicalFootprintBytes != nil || g.Memory.CompressedBytes != nil || g.Memory.SwapBytes != nil {
			t.Fatal("memory claim")
		}
		sum := uint64(0)
		got := []int{}
		for _, m := range g.Members {
			if seen[m.PID] {
				t.Fatal("multiple owners", m)
			}
			seen[m.PID] = true
			got = append(got, m.PID)
			sum += uint64(m.PID)
		}
		if !reflect.DeepEqual(got, want[i]) || *g.Memory.ResidentRSSBytes != sum {
			t.Fatalf("group %d: %+v", i, g)
		}
	}
	for _, p := range inv.Processes {
		if p.PID == 12 && p.Kind != "helper" {
			t.Fatal("nested terminal not helper")
		}
		if p.PID == 30 && p.Kind != "desktop_helper" {
			t.Fatal(p.Kind)
		}
		if p.PID == 31 && p.Kind != "app_server" {
			t.Fatal(p.Kind)
		}
		if p.PID == 40 && len(p.Warnings) == 0 {
			t.Fatal("missing parent hidden")
		}
		if p.AgeSeconds == nil || *p.AgeSeconds != 100 || p.CWD == nil || p.ObservedAt != time.Unix(200, 0) {
			t.Fatal("observations", p)
		}
	}
	if inv.Source.Status != "partial" {
		t.Fatal("missing parent must be partial")
	}
}
func TestReviewChangedAndUnknown(t *testing.T) {
	rows := []SnapshotProcess{row(10, 1, "codex", "ttys1"), row(11, 10, "sh", "ttys1"), row(12, 11, "node", "ttys1")}
	inv := collectFixture(t, rows, &fixtureCWD{changed: map[int]bool{11: true}})
	if len(inv.Groups) != 1 || len(inv.Groups[0].Members) != 1 {
		t.Fatalf("changed wrapper allowed attribution %+v", inv.Groups)
	}
	if inv.Processes[1].Identity != nil || inv.Processes[1].CWD != nil || inv.Processes[1].Memory.ResidentRSSBytes != nil || inv.Processes[2].Owner != nil {
		t.Fatal("identity invalidation incomplete")
	}
	inv = collectFixture(t, rows, &fixtureCWD{changed: map[int]bool{10: true}})
	if len(inv.Groups) != 0 {
		t.Fatal("changed root grouped")
	}
	inv = collectFixture(t, rows, &fixtureCWD{denied: true})
	if inv.Source.Status != "partial" || inv.Processes[0].CWD != nil || inv.Processes[0].Identity == nil {
		t.Fatal("cwd unknown semantics")
	}
	// A later collection with a reused PID carries the new identity, never cache.
	rows[0].Identity = &review.ProcessIdentity{PID: 10, StartID: "replacement"}
	inv = collectFixture(t, rows, &fixtureCWD{})
	if inv.Groups[0].Root.StartID != "replacement" {
		t.Fatal("PID reuse")
	}
	rows[0].Identity = nil
	inv = collectFixture(t, rows, &fixtureCWD{})
	if len(inv.Groups) != 0 || inv.Source.Status != "partial" {
		t.Fatal("missing identity")
	}
}
func TestReviewEmptyPartialAndFailure(t *testing.T) {
	inv := collectFixture(t, nil, &fixtureCWD{})
	if inv.Source.Status != "fresh" || len(inv.Processes) != 0 || len(inv.Groups) != 0 || inv.Processes == nil || inv.Groups == nil {
		t.Fatal("healthy empty", inv)
	}
	s := &fixtureSnapshot{err: errors.New("discovery denied")}
	inv, e := (&ReviewSource{SnapshotSource: s}).Collect(context.Background())
	if e == nil || inv.Source.Status != "unavailable" {
		t.Fatal("failure became empty success")
	}
	s = &fixtureSnapshot{rows: []SnapshotProcess{row(10, 1, "codex", "ttys1")}, warnings: []review.Warning{warning("partial_discovery", "fixture partial")}}
	inv, e = (&ReviewSource{SnapshotSource: s, CWDSource: &fixtureCWD{}}).Collect(context.Background())
	if e != nil || inv.Source.Status != "partial" || len(inv.Groups) != 1 {
		t.Fatal("partial lost", e)
	}
	rows := []SnapshotProcess{row(10, 1, "codex", "ttys1"), row(10, 1, "codex", "ttys1")}
	inv = collectFixture(t, rows, &fixtureCWD{})
	if len(inv.Groups) != 0 || inv.Processes[0].Identity != nil {
		t.Fatal("duplicate PID not excluded")
	}
}
func TestReviewDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	p := &ReviewSource{SnapshotSource: &fixtureSnapshot{rows: []SnapshotProcess{row(10, 1, "codex", "ttys1"), row(11, 10, "node", "ttys1")}}, CWDSource: &fixtureCWD{wait: true}}
	start := time.Now()
	inv, e := p.Collect(ctx)
	if e != nil || time.Since(start) > time.Second || inv.Source.Status != "partial" || len(inv.Processes) != 2 {
		t.Fatal("deadline lost partial", e)
	}
	for _, p := range inv.Processes {
		if p.CWD != nil || len(p.Warnings) == 0 {
			t.Fatal("deadline not explicit")
		}
	}
}
func TestReviewFrozenInventoryFixture(t *testing.T) {
	b, e := os.ReadFile("../../contracts/review-v1/fixtures/process-inventory.json")
	if e != nil {
		t.Fatal(e)
	}
	var frozen review.ProcessInventory
	if e = json.Unmarshal(b, &frozen); e != nil {
		t.Fatal(e)
	}
	raw := []SnapshotProcess{}
	for _, p := range frozen.Processes {
		tty := ""
		if p.TTY != nil {
			tty = *p.TTY
		}
		name := p.Agent
		if name == "" {
			name = "wrapper"
		}
		r := row(p.PID, p.ParentPID, name, tty)
		r.Identity = p.Identity
		r.RSSBytes = p.Memory.ResidentRSSBytes
		raw = append(raw, r)
	}
	got := collectFixture(t, raw, &fixtureCWD{})
	if len(got.Groups) != len(frozen.Groups) {
		t.Fatalf("frozen grouping differs: got=%d want=%d", len(got.Groups), len(frozen.Groups))
	}
}

func TestReviewPartialSnapshotError(t *testing.T) {
	s := &fixtureSnapshot{rows: []SnapshotProcess{row(10, 1, "codex", "ttys1")}, err: errors.New("truncated snapshot")}
	inv, e := (&ReviewSource{SnapshotSource: s, CWDSource: &fixtureCWD{}}).Collect(context.Background())
	if e == nil || len(inv.Processes) != 1 || inv.Source.Status != "partial" || len(inv.Warnings) == 0 {
		t.Fatal("partial rows on failure discarded", inv, e)
	}
}
