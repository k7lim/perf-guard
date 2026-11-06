package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/k7lim/perf-guard/internal/herdr"
	"github.com/k7lim/perf-guard/internal/output"
	"github.com/k7lim/perf-guard/internal/process"
	"github.com/k7lim/perf-guard/internal/review"
)

// Opt-in integration smoke. This never discovers a Herdr target, controls user
// processes, or emits private scope/PID/cwd/resume observations.
func TestReviewReadOnlyHostSmoke(t *testing.T) {
	path := os.Getenv("PERF_GUARD_REVIEW_SMOKE_TARGET_FILE")
	if path == "" {
		t.Skip("opt-in explicit-target read-only Mac smoke")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var target review.HerdrTarget
	if e = json.Unmarshal(b, &target); e != nil {
		t.Fatal(e)
	}
	resolved, e := herdr.ResolveTarget(target, nil)
	if e != nil {
		t.Fatal("invalid explicit smoke target")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	inventory, e := herdr.NewSource(process.ReadIdentity).Collect(ctx, resolved)
	if e != nil || inventory.Source.Status == "unavailable" {
		t.Fatal("Herdr source unavailable")
	}
	client, server := "unknown", "unknown"
	if inventory.ClientVersion != nil {
		client = *inventory.ClientVersion
	}
	if inventory.ServerVersion != nil {
		server = *inventory.ServerVersion
	}
	positive := 0
	for _, v := range inventory.Occupants {
		if v.Correlation == "positive" {
			positive++
		}
	}
	s := newReviewService()
	start := time.Now()
	r, e := s.List(ctx, review.ListOptions{Target: resolved, Limit: 200})
	if e != nil {
		t.Fatal(e)
	}
	validateCLIData(t, output.Response{Data: r}, "ReviewResponse")
	selected := "missing"
	for _, v := range r.Records {
		if v.Identity != nil && v.ID != review.ReviewID(*v.Identity) {
			t.Fatal("identity ID mismatch")
		}
		if v.Reviewable {
			selected = v.ID
			break
		}
	}
	if selected == "missing" && len(r.Records) > 0 {
		selected = r.Records[0].ID
	}
	p, e := s.Plan(ctx, resolved, []string{selected})
	if e != nil {
		t.Fatal(e)
	}
	validateCLIData(t, output.Response{Data: p}, "ReviewPlan")
	if p.ExecutionSupported || len(p.Selections) != 1 || !p.Selections[0].PreserveShell {
		t.Fatal("invalid read-only plan")
	}
	t.Logf("read-only integrated smoke: client=%s server=%s capabilities=%v source_positive=%d visible=%d reviewable=%d selected_status=%s list_plan_elapsed=%s; private identifiers omitted", client, server, inventory.Capabilities, positive, r.Total, r.ReviewableTotal, p.Selections[0].Status, time.Since(start).Round(time.Millisecond))
}
