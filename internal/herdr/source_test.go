package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/k7lim/perf-guard/internal/review"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type wireFixture struct {
	SchemaVersion int                        `json:"schema_version"`
	ClientVersion string                     `json:"client_version"`
	Ping          json.RawMessage            `json:"ping"`
	Snapshot      json.RawMessage            `json:"snapshot"`
	ProcessInfo   map[string]json.RawMessage `json:"process_info"`
}

func fixture(t *testing.T, name string) wireFixture {
	t.Helper()
	b, e := os.ReadFile("testdata/" + name)
	if e != nil {
		t.Fatal(e)
	}
	var f wireFixture
	if e = json.Unmarshal(b, &f); e != nil {
		t.Fatal(e)
	}
	if f.SchemaVersion != 1 {
		t.Fatal("fixture version")
	}
	return f
}

type recordingTransport struct {
	f            wireFixture
	calls        []string
	forbidden    []string
	snapshots    int
	mutateFinal  func(*snapshot)
	failMethod   string
	blockMethod  string
	process      func(string, int) json.RawMessage
	processCalls map[string]int
}

func (r *recordingTransport) Read(ctx context.Context, endpoint, method, paneID string) (json.RawMessage, error) {
	if !allowedRead(method) {
		r.forbidden = append(r.forbidden, method)
		return nil, errors.New("forbidden")
	}
	r.calls = append(r.calls, method)
	if r.blockMethod == method {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if r.failMethod == method {
		return nil, errors.New("unsupported method")
	}
	switch method {
	case "ping":
		return r.f.Ping, nil
	case "session.snapshot":
		r.snapshots++
		raw := r.f.Snapshot
		if r.snapshots > 1 && r.mutateFinal != nil {
			var v struct {
				Type     string   `json:"type"`
				Snapshot snapshot `json:"snapshot"`
			}
			json.Unmarshal(raw, &v)
			r.mutateFinal(&v.Snapshot)
			raw, _ = json.Marshal(v)
		}
		return raw, nil
	case "pane.process_info":
		if r.processCalls == nil {
			r.processCalls = map[string]int{}
		}
		r.processCalls[paneID]++
		if r.process != nil {
			return r.process(paneID, r.processCalls[paneID]), nil
		}
		return r.f.ProcessInfo[paneID], nil
	}
	return nil, errors.New("unexpected read")
}
func setup(t *testing.T) (*Source, *recordingTransport) {
	t.Helper()
	f := fixture(t, "protocol22-server090-client095.json")
	r := &recordingTransport{f: f}
	s := NewSource(func(_ context.Context, pid int) (review.ProcessIdentity, error) {
		return review.ProcessIdentity{PID: pid, StartID: "os-start-" + string(rune(pid))}, nil
	})
	s.Transport = r
	s.ClientVersion = func(context.Context) (string, error) { return f.ClientVersion, nil }
	s.LookupEnv = func(string) string { return "" }
	s.Clock = fixedClock{time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)}
	return s, r
}

var explicitTarget = review.HerdrTarget{Endpoint: "unix:///example/herdr.sock", SessionID: "session-example"}

func collect(t *testing.T, s *Source) review.HerdrInventory {
	t.Helper()
	v, e := s.Collect(context.Background(), explicitTarget)
	if e != nil {
		t.Fatal(e)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	schema, err := c.Compile(filepath.Join("../../contracts/review-v1", "HerdrInventory.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(document); err != nil {
		t.Fatal(err)
	}
	return v
}
func hasCode(ws []review.Warning, code string) bool {
	for _, w := range ws {
		if w.Code == code {
			return true
		}
	}
	return false
}

func TestReadOnlyNormalizationAndStableOccupants(t *testing.T) {
	s, r := setup(t)
	v := collect(t, s)
	if *v.ClientVersion != "0.9.5" || *v.ServerVersion != "0.9.0" || v.Source.Status != "fresh" || len(v.Occupants) != 5 {
		t.Fatalf("versions/collection: %+v", v)
	}
	states := map[string]bool{}
	var revision string
	for _, o := range v.Occupants {
		states[o.Lifecycle.State] = true
		if o.Correlation != "positive" || o.Process == nil || o.Revision == nil {
			t.Fatal("not correlated", o)
		}
		if o.ServerID == "" || o.SessionID != explicitTarget.SessionID {
			t.Fatal("lost scope")
		}
		if o.UserActivity != nil || o.ConversationActivity != nil {
			t.Fatal("invented activity")
		}
		if o.Process.PID == 101 {
			revision = *o.Revision
		}
		if o.Process.PID == 102 && (o.ResumeReference == nil || *o.ResumeReference != "example-resume-102") {
			t.Fatal("native resume context lost")
		}
	}
	for _, st := range []string{"idle", "done", "working", "blocked", "unknown"} {
		if !states[st] {
			t.Fatal("state lost", st)
		}
	}
	if len(r.forbidden) != 0 {
		t.Fatal(r.forbidden)
	}
	for _, c := range r.calls {
		if !allowedRead(c) {
			t.Fatal(c)
		}
	}
	// Output revisions change frequently and are deliberately not decoded.
	var snap map[string]any
	json.Unmarshal(r.f.Snapshot, &snap)
	for _, p := range snap["snapshot"].(map[string]any)["panes"].([]any) {
		p.(map[string]any)["revision"] = 999999
	}
	r.f.Snapshot, _ = json.Marshal(snap)
	again := collect(t, s)
	for _, o := range again.Occupants {
		if o.Process.PID == 101 && *o.Revision != revision {
			t.Fatal("output redraw changed occupant identity")
		}
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "argv") || strings.Contains(string(raw), "cmdline") {
		t.Fatal("arguments retained")
	}
}
func TestChangedIdentityAndUnknownCorrelation(t *testing.T) {
	cases := []struct {
		name      string
		configure func(*Source, *recordingTransport)
		code      string
	}{
		{"no-resolver", func(s *Source, r *recordingTransport) { s.ResolveIdentity = nil }, "uncorrelated"},
		{"resolver-fails", func(s *Source, r *recordingTransport) {
			s.ResolveIdentity = func(context.Context, int) (review.ProcessIdentity, error) {
				return review.ProcessIdentity{}, errors.New("denied")
			}
		}, "uncorrelated"},
		{"wrapper-only", func(s *Source, r *recordingTransport) {
			r.process = func(id string, n int) json.RawMessage {
				b, _ := json.Marshal(map[string]any{"type": "pane_process_info", "process_info": map[string]any{"pane_id": id, "foreground_processes": []any{map[string]any{"pid": 101, "name": "nono"}}}})
				return b
			}
		}, "uncorrelated"},
		{"ambiguous-agents", func(s *Source, r *recordingTransport) {
			r.process = func(id string, n int) json.RawMessage {
				b, _ := json.Marshal(map[string]any{"type": "pane_process_info", "process_info": map[string]any{"pane_id": id, "foreground_processes": []any{map[string]any{"pid": 101, "name": "codex"}, map[string]any{"pid": 102, "name": "codex"}}}})
				return b
			}
		}, "uncorrelated"},
		{"pid-reuse", func(s *Source, r *recordingTransport) {
			counts := map[int]int{}
			s.ResolveIdentity = func(_ context.Context, pid int) (review.ProcessIdentity, error) {
				counts[pid]++
				return review.ProcessIdentity{PID: pid, StartID: strings.Repeat("x", counts[pid])}, nil
			}
		}, "identity_changed"},
		{"changed-terminal", func(s *Source, r *recordingTransport) {
			r.mutateFinal = func(v *snapshot) {
				for i := range v.Panes {
					v.Panes[i].TerminalID = "new-terminal"
				}
			}
		}, "identity_changed"},
		{"changed-native-session", func(s *Source, r *recordingTransport) {
			r.mutateFinal = func(v *snapshot) {
				for i := range v.Panes {
					v.Panes[i].Session = &nativeSession{Source: "native", Agent: "codex", Kind: "id", Value: "replacement"}
				}
			}
		}, "identity_changed"},
		{"disappeared-pane", func(s *Source, r *recordingTransport) { r.mutateFinal = func(v *snapshot) { v.Panes = []pane{} } }, "identity_changed"},
		{"unsupported-process-read", func(s *Source, r *recordingTransport) { r.failMethod = "pane.process_info" }, "uncorrelated"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s, r := setup(t)
			tt.configure(s, r)
			v := collect(t, s)
			if v.Source.Status != "partial" {
				t.Fatal(v.Source.Status)
			}
			for _, o := range v.Occupants {
				if o.Correlation == "positive" || o.Process != nil || o.Revision != nil || !hasCode(o.Warnings, tt.code) {
					t.Fatal("invented positive correlation", o)
				}
			}
			if len(r.forbidden) != 0 {
				t.Fatal(r.forbidden)
			}
		})
	}
}
func TestScopedServersAndDuplicates(t *testing.T) {
	s, r := setup(t)
	a := collect(t, s)
	b, e := s.Collect(context.Background(), review.HerdrTarget{Endpoint: "unix:///example/other.sock", SessionID: explicitTarget.SessionID})
	if e != nil {
		t.Fatal(e)
	}
	if a.Occupants[0].ServerID == b.Occupants[0].ServerID || *a.Occupants[0].Revision == *b.Occupants[0].Revision {
		t.Fatal("scope collision")
	}
	var v struct {
		Type     string   `json:"type"`
		Snapshot snapshot `json:"snapshot"`
	}
	json.Unmarshal(r.f.Snapshot, &v)
	v.Snapshot.Panes = append(v.Snapshot.Panes, v.Snapshot.Panes[0])
	r.f.Snapshot, _ = json.Marshal(v)
	out := collect(t, s)
	n := 0
	for _, o := range out.Occupants {
		if o.PaneID == v.Snapshot.Panes[0].PaneID {
			n++
			if o.Correlation != "ambiguous" || o.Process != nil || !hasCode(o.Warnings, "duplicate_identity") {
				t.Fatal("duplicate not failed closed")
			}
		}
	}
	if n != 2 {
		t.Fatal("duplicate evidence discarded")
	}
}
func TestMissingIncompatibleEmptyAndCancellation(t *testing.T) {
	t.Run("missing-target", func(t *testing.T) {
		s, r := setup(t)
		v, e := s.Collect(context.Background(), review.HerdrTarget{})
		if e != nil || v.Source.Status != "unavailable" || len(r.calls) != 0 || v.Target.Endpoint != "unavailable" {
			t.Fatal(v, e)
		}
	})
	t.Run("unsupported-protocol", func(t *testing.T) {
		s, r := setup(t)
		r.f = fixture(t, "protocol21-unsupported.json")
		v := collect(t, s)
		if v.Source.Status != "unavailable" || !hasCode(v.Warnings, "unsupported_capability") || !reflect.DeepEqual(r.calls, []string{"ping"}) {
			t.Fatal(v, r.calls)
		}
	})
	t.Run("disconnected", func(t *testing.T) {
		s, r := setup(t)
		r.failMethod = "ping"
		v := collect(t, s)
		if v.Source.Status != "unavailable" || v.Source.CollectedAt != nil {
			t.Fatal(v)
		}
	})
	t.Run("empty", func(t *testing.T) {
		s, r := setup(t)
		r.f.Snapshot = json.RawMessage(`{"type":"session_snapshot","snapshot":{"version":"0.9.0","protocol":22,"panes":[],"agents":[]}}`)
		v := collect(t, s)
		if v.Source.Status != "fresh" || v.Occupants == nil || len(v.Occupants) != 0 {
			t.Fatal(v)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		s, r := setup(t)
		r.blockMethod = "pane.process_info"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		start := time.Now()
		v, e := s.Collect(ctx, explicitTarget)
		if e != nil || time.Since(start) > time.Second || v.Source.Status != "partial" || !hasCode(v.Warnings, "deadline_exceeded") {
			t.Fatal(v, e)
		}
	})
}
func TestExplicitAndInheritedTarget(t *testing.T) {
	lookup := func(k string) string {
		switch k {
		case "PERF_GUARD_HERDR_ENDPOINT":
			return "/example/inherited.sock"
		case "PERF_GUARD_HERDR_SESSION":
			return "caller-session"
		}
		return ""
	}
	v, e := ResolveTarget(review.HerdrTarget{}, lookup)
	if e != nil || v.Endpoint != "unix:///example/inherited.sock" || v.SessionID != "caller-session" {
		t.Fatal(v, e)
	}
	v, e = ResolveTarget(explicitTarget, lookup)
	if e != nil || v != explicitTarget {
		t.Fatal(v, e)
	}
	if _, e = ResolveTarget(review.HerdrTarget{Endpoint: "/example/partial.sock"}, lookup); e == nil {
		t.Fatal("partial target guessed")
	}
	for _, bad := range []string{"relative.sock", "http://example", "unix://host/example.sock", "unix:///example.sock?focus=1", "/example/\n.sock", "unix:///example%2fsock"} {
		if _, e := NormalizeEndpoint(bad); e == nil {
			t.Fatal("accepted", bad)
		}
	}
}

func TestMalformedPartialSnapshotsAndEphemeralPrivateFields(t *testing.T) {
	t.Run("missing-scope", func(t *testing.T) {
		s, r := setup(t)
		var v struct {
			Type     string   `json:"type"`
			Snapshot snapshot `json:"snapshot"`
		}
		json.Unmarshal(r.f.Snapshot, &v)
		v.Snapshot.Panes[0].WorkspaceID = ""
		r.f.Snapshot, _ = json.Marshal(v)
		out := collect(t, s)
		if out.Source.Status != "partial" || len(out.Occupants) != 4 || !hasCode(out.Warnings, "partial_discovery") {
			t.Fatal(out)
		}
	})
	t.Run("same-id-different-workspaces", func(t *testing.T) {
		s, r := setup(t)
		var v struct {
			Type     string   `json:"type"`
			Snapshot snapshot `json:"snapshot"`
		}
		json.Unmarshal(r.f.Snapshot, &v)
		v.Snapshot.Panes[1].PaneID = v.Snapshot.Panes[0].PaneID
		r.f.Snapshot, _ = json.Marshal(v)
		out := collect(t, s)
		for _, o := range out.Occupants {
			if o.PaneID == v.Snapshot.Panes[0].PaneID && o.Correlation != "ambiguous" {
				t.Fatal("pane-only query ambiguity ignored")
			}
		}
	})
	t.Run("failed-final-snapshot", func(t *testing.T) {
		s, r := setup(t)
		r.mutateFinal = func(v *snapshot) { v.Version = "changed-server" }
		out := collect(t, s)
		for _, o := range out.Occupants {
			if o.Correlation == "positive" {
				t.Fatal("server changed")
			}
		}
		if out.Source.Status != "partial" {
			t.Fatal(out)
		}
	})
	t.Run("private-arguments-discarded", func(t *testing.T) {
		s, r := setup(t)
		r.process = func(id string, n int) json.RawMessage {
			raw := r.f.ProcessInfo[id]
			var v map[string]any
			json.Unmarshal(raw, &v)
			processes := v["process_info"].(map[string]any)["foreground_processes"].([]any)
			processes[0].(map[string]any)["argv"] = []string{"SENSITIVE-ARGUMENT"}
			processes[0].(map[string]any)["cmdline"] = "SENSITIVE-COMMAND"
			processes[0].(map[string]any)["cwd"] = "/SENSITIVE-CWD"
			b, _ := json.Marshal(v)
			return b
		}
		out := collect(t, s)
		b, _ := json.Marshal(out)
		if strings.Contains(string(b), "SENSITIVE") {
			t.Fatal("private ephemeral wire fields retained")
		}
	})
}
