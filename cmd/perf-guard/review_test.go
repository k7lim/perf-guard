package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/k7lim/perf-guard/internal/output"
	"github.com/k7lim/perf-guard/internal/review"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type cliClock struct{ at time.Time }

func (c cliClock) Now() time.Time { return c.at }

type cliProcesses struct {
	p    review.ProcessInventory
	fail bool
}

func (p cliProcesses) Collect(context.Context) (review.ProcessInventory, error) {
	if p.fail {
		return p.p, errors.New("fixture denied")
	}
	return p.p, nil
}

type cliHerdr struct{ h review.HerdrInventory }

func (h cliHerdr) Collect(context.Context, review.HerdrTarget) (review.HerdrInventory, error) {
	return h.h, nil
}
func cliFixtureService(t *testing.T) *review.Service {
	b, e := os.ReadFile("../../contracts/review-v1/fixtures/healthy-review.json")
	if e != nil {
		t.Fatal(e)
	}
	var r review.ReviewResponse
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	p := review.ProcessInventory{SchemaVersion: 1, Source: r.Sources[0], Processes: []review.ProcessRecord{}, Groups: []review.ProcessGroup{}, Warnings: []review.Warning{}}
	h := review.HerdrInventory{SchemaVersion: 1, Target: r.Target, Source: r.Sources[1], Occupants: []review.HerdrOccupant{}, Warnings: []review.Warning{}}
	for _, v := range r.Records {
		i := v.Identity
		p.Processes = append(p.Processes, review.ProcessRecord{PID: v.Process.PID, Identity: v.Process, Owner: v.Process, Kind: "terminal", Agent: v.Agent, ObservedAt: *v.ProcessObservedAt, Memory: v.Memory, CWD: v.CWD, AgeSeconds: v.AgeSeconds, FieldSources: map[string]string{"identity": v.ProcessProvenance}, Warnings: []review.Warning{}})
		h.Occupants = append(h.Occupants, review.HerdrOccupant{ServerID: i.ServerID, SessionID: i.SessionID, WorkspaceID: i.WorkspaceID, TabID: i.TabID, PaneID: i.PaneID, Revision: &i.Revision, Process: v.Process, Correlation: "positive", Lifecycle: v.Lifecycle, Warnings: []review.Warning{}})
	}
	return &review.Service{Processes: cliProcesses{p: p, fail: os.Getenv("PERF_GUARD_CLI_TEST_FAILURE") == "1"}, Herdr: cliHerdr{h}, Clock: cliClock{r.CollectedAt}, ControllerIdentity: &review.ProcessIdentity{PID: 99999, StartID: "other-controller"}}
}
func TestReviewCLIChild(t *testing.T) {
	if os.Getenv("PERF_GUARD_CLI_TEST_CHILD") != "1" {
		return
	}
	newReviewService = func() *review.Service { return cliFixtureService(t) }
	for i, v := range os.Args {
		if v == "--" {
			os.Args = append([]string{"perf-guard"}, os.Args[i+1:]...)
			break
		}
	}
	main()
	os.Exit(0)
}
func runCLI(t *testing.T, fail bool, args ...string) (output.Response, int, []byte) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestReviewCLIChild$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "PERF_GUARD_CLI_TEST_CHILD=1", "OUTPUT_FORMAT=json")
	if fail {
		cmd.Env = append(cmd.Env, "PERF_GUARD_CLI_TEST_FAILURE=1")
	}
	b, e := cmd.CombinedOutput()
	code := 0
	if e != nil {
		var ex *exec.ExitError
		if !errors.As(e, &ex) {
			t.Fatal(e)
		}
		code = ex.ExitCode()
	}
	var r output.Response
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatalf("not a JSON envelope: %v: %s", e, b)
	}
	return r, code, b
}
func validateCLIData(t *testing.T, r output.Response, kind string) {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	s, e := c.Compile(filepath.Join("../../contracts/review-v1", kind+".schema.json"))
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(r.Data)
	d, e := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Validate(d); e != nil {
		t.Fatal(e)
	}
}
func TestReviewCLIEnvelopePlanAndErrors(t *testing.T) {
	r, code, _ := runCLI(t, false, "review", "--output", "json", "--limit", "1", "--offset", "1")
	if code != 0 || !r.Success || r.Command != "review" {
		t.Fatal(r, code)
	}
	validateCLIData(t, r, "ReviewResponse")
	data := r.Data.(map[string]any)
	if data["total"] != float64(4) || len(data["records"].([]any)) != 1 {
		t.Fatal(data)
	}
	r, code, _ = runCLI(t, false, "review", "plan", "--session", "missing", "--session", "missing", "--output", "json")
	if code != 0 || !r.Success || r.Command != "review plan" {
		t.Fatal(r, code)
	}
	validateCLIData(t, r, "ReviewPlan")
	data = r.Data.(map[string]any)
	if data["execution_supported"] != false || data["selections"].([]any)[1].(map[string]any)["status"] != "duplicate" {
		t.Fatal(data)
	}
	for _, args := range [][]string{{"review", "--limit", "0"}, {"review", "--limit", "201"}, {"review", "--offset", "-1"}, {"review", "--limit", "invalid"}, {"review", "--wat"}, {"review", "plan"}, {"review", "plan", "--session", "bad\nID"}, {"review", "--herdr-endpoint", "relative", "--herdr-session", "scope"}, {"review", "--herdr-endpoint", "/absolute"}, {"review", "--output", "bogus"}, {"review", "unexpected"}, {"review", "plan", "--apply"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r, code, _ := runCLI(t, false, args...)
			if code != 1 || r.Success || r.Error == "" || r.Data != nil || !strings.HasPrefix(r.Command, "review") {
				t.Fatal("error contract", r, code)
			}
		})
	}
	r, code, _ = runCLI(t, true, "review", "--output", "json")
	if code != 1 || r.Success || r.Data != nil || !strings.Contains(r.Error, "process discovery") {
		t.Fatal("required failure", r, code)
	}
}
func TestReviewHelpAndDescribe(t *testing.T) {
	r := newRootCommand()
	cmd, _, e := r.Find([]string{"review", "plan"})
	if e != nil || cmd.Name() != "plan" || cmd.Flags().Lookup("session") == nil {
		t.Fatal("plan help", e)
	}
	s := reviewCommandSchema()
	if s.Mutates || s.Flags["--limit"].Default != "50" || s.Subcommands["plan"].OutputSchema["execution_supported"] != "false (always)" {
		t.Fatal(s)
	}
	root := newRootCommand()
	var b bytes.Buffer
	root.SetOut(&b)
	root.SetArgs([]string{"review", "--help"})
	if e = root.Execute(); e != nil || !strings.Contains(b.String(), "--herdr-endpoint") {
		t.Fatal("help", e, b.String())
	}
}
