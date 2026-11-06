package review

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const contractDir = "../../contracts/review-v1"

func schema(t *testing.T, kind string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	s, err := c.Compile(filepath.Join(contractDir, kind+".schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func document(t *testing.T, raw []byte) any {
	t.Helper()
	x, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return x
}
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(contractDir, "fixtures", name))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestContractFixtures(t *testing.T) {
	var manifest []struct{ File, Kind string }
	if err := json.Unmarshal(fixture(t, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) < 13 {
		t.Fatal("fixture coverage unexpectedly removed")
	}
	for _, entry := range manifest {
		t.Run(entry.File, func(t *testing.T) {
			b := fixture(t, entry.File)
			s := schema(t, entry.Kind)
			if e := s.Validate(document(t, b)); e != nil {
				t.Fatal(e)
			}
			var typed any
			switch entry.Kind {
			case "ReviewResponse":
				typed = &ReviewResponse{}
			case "ReviewPlan":
				typed = &ReviewPlan{}
			case "ProcessInventory":
				typed = &ProcessInventory{}
			case "HerdrInventory":
				typed = &HerdrInventory{}
			default:
				t.Fatal(entry.Kind)
			}
			if e := json.Unmarshal(b, typed); e != nil {
				t.Fatal(e)
			}
			round, e := json.Marshal(typed)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.Validate(document(t, round)); e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(document(t, b), document(t, round)) {
				t.Fatal("schema fixture differs after Go round trip")
			}
			if r, ok := typed.(*ReviewResponse); ok {
				for _, v := range r.Records {
					if v.Identity != nil && v.ID != ReviewID(*v.Identity) {
						t.Fatalf("identity-bound id mismatch: %s", v.ID)
					}
				}
			}
		})
	}
}
func TestContractRejectsInvalid(t *testing.T) {
	cases := []struct {
		name, file, kind string
		mutate           func(map[string]any)
	}{
		{"missing-required", "healthy-review.json", "ReviewResponse", func(m map[string]any) { delete(m, "records") }},
		{"unknown-version", "healthy-review.json", "ReviewResponse", func(m map[string]any) { m["schema_version"] = 2 }},
		{"null-list", "healthy-review.json", "ReviewResponse", func(m map[string]any) { m["records"] = nil }},
		{"bad-enum", "healthy-review.json", "ReviewResponse", func(m map[string]any) { m["sources"].([]any)[0].(map[string]any)["status"] = "ok" }},
		{"bad-time", "healthy-review.json", "ReviewResponse", func(m map[string]any) { m["collected_at"] = "yesterday" }},
		{"negative-rss", "healthy-review.json", "ReviewResponse", func(m map[string]any) {
			m["records"].([]any)[0].(map[string]any)["memory"].(map[string]any)["resident_rss_bytes"] = -1
		}},
		{"unknown-identity-reviewable", "healthy-review.json", "ReviewResponse", func(m map[string]any) {
			r := m["records"].([]any)[0].(map[string]any)
			r["reviewable"] = true
			r["identity"] = nil
		}},
		{"plan-cannot-execute", "example-plan.json", "ReviewPlan", func(m map[string]any) { m["execution_supported"] = true }},
		{"shell-preserved", "example-plan.json", "ReviewPlan", func(m map[string]any) { m["selections"].([]any)[0].(map[string]any)["preserve_shell"] = false }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := document(t, fixture(t, c.file)).(map[string]any)
			c.mutate(m)
			if e := schema(t, c.kind).Validate(m); e == nil {
				t.Fatal("invalid fixture accepted")
			}
		})
	}
	// Additive fields remain readable in Go and valid in schema.
	raw := fixture(t, "empty-review.json")
	raw = bytes.Replace(raw, []byte(`"schema_version": 1`), []byte(`"schema_version": 1, "future_field": "ignored"`), 1)
	if e := schema(t, "ReviewResponse").Validate(document(t, raw)); e != nil {
		t.Fatal(e)
	}
	var r ReviewResponse
	if e := json.Unmarshal(raw, &r); e != nil {
		t.Fatal(e)
	}
}
func TestReviewIDBindsEveryIdentityComponent(t *testing.T) {
	var r ReviewResponse
	if e := json.Unmarshal(fixture(t, "healthy-review.json"), &r); e != nil {
		t.Fatal(e)
	}
	base := *r.Records[0].Identity
	id := ReviewID(base)
	mutations := []func(*OccupantIdentity){func(i *OccupantIdentity) { i.Endpoint += "b" }, func(i *OccupantIdentity) { i.ServerID += "b" }, func(i *OccupantIdentity) { i.SessionID += "b" }, func(i *OccupantIdentity) { i.WorkspaceID += "b" }, func(i *OccupantIdentity) { i.TabID += "b" }, func(i *OccupantIdentity) { i.PaneID += "b" }, func(i *OccupantIdentity) { i.Revision += "b" }, func(i *OccupantIdentity) { i.Process.PID++ }, func(i *OccupantIdentity) { i.Process.StartID += "b" }}
	for n, mutate := range mutations {
		v := base
		mutate(&v)
		if ReviewID(v) == id {
			t.Fatalf("component %d not identity-bound", n)
		}
	}
	a, b := base, base
	a.ServerID = "a/b"
	a.SessionID = "c"
	b.ServerID = "a"
	b.SessionID = "b/c"
	if ReviewID(a) == ReviewID(b) {
		t.Fatal("separator collision")
	}
}
func TestContractFactsStayDistinct(t *testing.T) {
	var r ReviewResponse
	json.Unmarshal(fixture(t, "healthy-review.json"), &r)
	states := map[string]bool{}
	for _, v := range r.Records {
		states[v.Lifecycle.State] = true
		if v.UserActivity != nil || v.ConversationActivity != nil {
			t.Fatal("age became activity")
		}
		if v.Memory.PhysicalFootprintBytes != nil || v.Memory.CompressedBytes != nil || v.Memory.SwapBytes != nil {
			t.Fatal("unknown became zero")
		}
	}
	for _, s := range []string{"idle", "done", "working", "blocked"} {
		if !states[s] {
			t.Fatal("missing state", s)
		}
	}
	var p ProcessInventory
	json.Unmarshal(fixture(t, "process-inventory.json"), &p)
	for _, g := range p.Groups {
		if !g.Memory.SummedRSSIsEstimate {
			t.Fatal("aggregate RSS not estimated")
		}
	}
	if DefaultCollectionTimeout != 5*time.Second {
		t.Fatal("deadline drift")
	}
	for _, typ := range []reflect.Type{reflect.TypeOf((*ProcessSource)(nil)).Elem(), reflect.TypeOf((*HerdrSource)(nil)).Elem(), reflect.TypeOf((*WorktreeSource)(nil)).Elem()} {
		if typ.NumMethod() != 1 || !(strings.HasPrefix(typ.Method(0).Name, "Collect") || typ.Method(0).Name == "Inspect") {
			t.Fatal("control capability added", typ)
		}
	}
}

func TestNamedScenarioEvidence(t *testing.T) {
	readReview := func(name string) ReviewResponse {
		var r ReviewResponse
		if e := json.Unmarshal(fixture(t, name), &r); e != nil {
			t.Fatal(e)
		}
		return r
	}
	healthy := readReview("healthy-review.json")
	var original ReviewRecord
	for _, r := range healthy.Records {
		if r.Process.PID == 101 {
			original = r
		}
	}
	changed := readReview("changed-occupant-review.json").Records[0]
	if changed.Process.PID != original.Process.PID || changed.Process.StartID == original.Process.StartID || changed.ID == original.ID || changed.Identity.PaneID != original.Identity.PaneID {
		t.Fatal("changed occupant fixture fails PID reuse scenario")
	}
	two := readReview("two-servers-review.json").Records
	if len(two) != 2 || two[0].ID == two[1].ID || two[0].Identity.PaneID != two[1].Identity.PaneID || two[0].Identity.ServerID == two[1].Identity.ServerID {
		t.Fatal("two-server collision scenario missing")
	}
	duplicate := readReview("duplicate-scoped-review.json")
	if duplicate.Records[0].Reviewable || duplicate.Records[0].Reasons[0] != ReasonDuplicate || len(duplicate.Warnings) == 0 {
		t.Fatal("duplicates not explicitly excluded")
	}
	stale := readReview("stale-partial-review.json")
	if stale.Sources[1].Status != "stale" || stale.Records[0].Reviewable || stale.Warnings[0].Code != "deadline_exceeded" {
		t.Fatal("stale/partial scenario missing")
	}
	unavailable := readReview("unavailable-herdr-review.json")
	u := unavailable.Records[0]
	if unavailable.Sources[1].Status != "unavailable" || u.CWD != nil || u.Worktree.State != "unknown" || len(u.Worktree.Warnings) == 0 || u.Identity != nil || u.Reviewable {
		t.Fatal("inaccessible/unavailable scenario missing")
	}
	if u.ID != ExcludedID("process", "101", "os-start-101") {
		t.Fatal("excluded display ID mismatch")
	}
	if ExcludedID("herdr", "a/b", "c") == ExcludedID("herdr", "a", "b/c") {
		t.Fatal("display ID ambiguous encoding")
	}
	dirty := readReview("dirty-review.json").Records[0]
	if dirty.Worktree.State != "dirty" || dirty.ResumeReference != nil {
		t.Fatal("dirty/missing-resume scenario missing")
	}
	empty := readReview("empty-review.json")
	if len(empty.Records) != 0 || empty.Total != 0 || empty.ReviewableTotal != 0 || empty.Sources[0].Status != "fresh" {
		t.Fatal("empty fixture not a healthy empty collection")
	}
	var process ProcessInventory
	json.Unmarshal(fixture(t, "process-inventory.json"), &process)
	if process.Processes[2].ParentPID != process.Processes[1].PID || process.Processes[1].ParentPID != process.Processes[0].PID || *process.Processes[3].TTY == *process.Processes[0].TTY || process.Processes[4].ParentPID != 999 {
		t.Fatal("wrapper/separate-TTY/missing-parent scenario missing")
	}
	json.Unmarshal(fixture(t, "partial-process-inventory.json"), &process)
	if process.Source.Status != "partial" || process.Processes[4].Identity != nil || len(process.Warnings) == 0 {
		t.Fatal("disappeared/partial process scenario missing")
	}
	var herdr HerdrInventory
	json.Unmarshal(fixture(t, "unknown-herdr-inventory.json"), &herdr)
	if herdr.Occupants[0].Correlation != "unknown" || herdr.Occupants[0].Revision != nil || herdr.Occupants[0].Process != nil || herdr.ClientVersion == nil || herdr.ServerVersion == nil || *herdr.ClientVersion == *herdr.ServerVersion {
		t.Fatal("unknown correlation/version scenario missing")
	}
	var plan ReviewPlan
	json.Unmarshal(fixture(t, "example-plan.json"), &plan)
	if plan.ExecutionSupported || len(plan.Selections) != 4 {
		t.Fatal("plan fixture missing selection outcomes")
	}
	for n, status := range []string{"reviewable", "missing", "duplicate", "excluded"} {
		s := plan.Selections[n]
		if s.Status != status || !s.PreserveShell || s.RecoveryAvailable {
			t.Fatal("plan result missing", status)
		}
	}
}
