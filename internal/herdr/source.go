package herdr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/k7lim/perf-guard/internal/review"
)

// IdentityResolver must read actual OS start identity, never now-elapsed. #8 can
// inject process.ReadIdentity. Only foreground agent PIDs are presented to it.
type IdentityResolver func(context.Context, int) (review.ProcessIdentity, error)

// Source implements the frozen review.HerdrSource. Optional seams are for bounded
// reads and deterministic tests; their zero values use safe production defaults.
type Source struct {
	Transport       Transport
	ResolveIdentity IdentityResolver
	Clock           review.Clock
	ClientVersion   func(context.Context) (string, error)
	LookupEnv       func(string) string
}

func NewSource(resolve IdentityResolver) *Source { return &Source{ResolveIdentity: resolve} }

var _ review.HerdrSource = (*Source)(nil)

func localClientVersion(ctx context.Context) (string, error) {
	b, err := exec.CommandContext(ctx, "herdr", "--version").Output()
	if err != nil {
		return "", errors.New("Herdr client version unavailable")
	}
	fields := strings.Fields(string(b))
	if len(fields) != 2 || fields[0] != "herdr" {
		return "", errors.New("invalid Herdr client version")
	}
	return fields[1], nil
}

// ResolveTarget uses only explicit arguments or this tool's explicitly inherited
// PERF_GUARD_HERDR_ENDPOINT + PERF_GUARD_HERDR_SESSION pair. It never discovers a
// default server. A partial explicit target is rejected, not completed by env.
func ResolveTarget(target review.HerdrTarget, lookup func(string) string) (review.HerdrTarget, error) {
	if lookup == nil {
		lookup = os.Getenv
	}
	if target.Endpoint == "" && target.SessionID == "" {
		target = review.HerdrTarget{Endpoint: lookup("PERF_GUARD_HERDR_ENDPOINT"), SessionID: lookup("PERF_GUARD_HERDR_SESSION")}
	}
	if target.Endpoint == "" || target.SessionID == "" || strings.IndexFunc(target.SessionID, unicode.IsControl) >= 0 || target.SessionID == "unavailable" {
		return review.HerdrTarget{Endpoint: "unavailable", SessionID: "unavailable"}, errors.New("explicit Herdr endpoint and session required")
	}
	canonical, err := NormalizeEndpoint(target.Endpoint)
	if err != nil {
		return target, err
	}
	target.Endpoint = canonical
	return target, nil
}

type nativeSession struct {
	Source string `json:"source"`
	Agent  string `json:"agent"`
	Kind   string `json:"kind"`
	Value  string `json:"value"`
}
type pane struct {
	PaneID      string         `json:"pane_id"`
	WorkspaceID string         `json:"workspace_id"`
	TabID       string         `json:"tab_id"`
	TerminalID  string         `json:"terminal_id"`
	Agent       *string        `json:"agent"`
	State       string         `json:"agent_status"`
	Session     *nativeSession `json:"agent_session"`
	// Deliberately do not decode cwd, titles, output revision, argv or transcript.
}
type snapshot struct {
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
	Panes    []pane `json:"panes"`
	Agents   []pane `json:"agents"`
}
type processInfo struct {
	PaneID    string `json:"pane_id"`
	Processes []struct {
		PID  int    `json:"pid"`
		Name string `json:"name"`
	} `json:"foreground_processes"`
}
type observed struct {
	identity    *review.ProcessIdentity
	fingerprint string
	err         error
	supported   bool
}

func warning(code, message string) review.Warning {
	return review.Warning{Code: code, Source: "herdr", Message: message}
}
func state(s string) string {
	switch s {
	case "working", "blocked", "idle", "done", "unknown":
		return s
	}
	return "unknown"
}
func agentName(p pane) string {
	if p.Agent != nil {
		return *p.Agent
	}
	if p.Session != nil {
		return p.Session.Agent
	}
	return ""
}
func isAgent(p pane) bool {
	return p.Agent != nil && *p.Agent != "" || p.Session != nil && p.Session.Agent != ""
}
func samePane(a, b pane) bool {
	// Lifecycle may change while occupant metadata remains the same.
	a.State = ""
	b.State = ""
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func digest(parts ...string) string {
	b, _ := json.Marshal(parts)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func scopedPaneKey(p pane) string { return digest(p.WorkspaceID, p.TabID, p.PaneID) }
func validPane(p pane) bool {
	return p.PaneID != "" && p.WorkspaceID != "" && p.TabID != "" && p.TerminalID != ""
}
func readSnapshot(ctx context.Context, t Transport, endpoint string) (snapshot, error) {
	raw, err := t.Read(ctx, endpoint, "session.snapshot", "")
	if err != nil {
		return snapshot{}, err
	}
	var r struct {
		Type     string   `json:"type"`
		Snapshot snapshot `json:"snapshot"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Type != "session_snapshot" || r.Snapshot.Panes == nil || r.Snapshot.Version == "" || r.Snapshot.Protocol != 22 {
		return snapshot{}, errors.New("unsupported Herdr snapshot")
	}
	return r.Snapshot, nil
}

func (s *Source) observe(ctx context.Context, t Transport, endpoint string, p pane) observed {
	raw, err := t.Read(ctx, endpoint, "pane.process_info", p.PaneID)
	if err != nil {
		return observed{err: err}
	}
	var r struct {
		Type string      `json:"type"`
		Info processInfo `json:"process_info"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Type != "pane_process_info" || r.Info.PaneID != p.PaneID {
		return observed{err: errors.New("invalid process correlation response")}
	}
	candidates := map[int]bool{}
	for _, v := range r.Info.Processes {
		name := filepath.Base(v.Name)
		if v.PID > 0 && name == agentName(p) && (name == "codex" || name == "claude") {
			candidates[v.PID] = true
		}
	}
	if len(candidates) != 1 {
		return observed{err: errors.New("agent process correlation unknown or ambiguous"), supported: true}
	}
	if s.ResolveIdentity == nil {
		return observed{err: errors.New("OS identity resolver unavailable"), supported: true}
	}
	var pid int
	for v := range candidates {
		pid = v
	}
	id, err := s.ResolveIdentity(ctx, pid)
	if err != nil || id.PID != pid || id.StartID == "" {
		return observed{err: errors.New("actual OS process start identity unavailable"), supported: true}
	}
	return observed{identity: &id, fingerprint: digest(strconv.Itoa(id.PID), id.StartID, p.TerminalID, agentName(p)), supported: true}
}

func (s *Source) Collect(ctx context.Context, target review.HerdrTarget) (review.HerdrInventory, error) {
	ctx, cancel := context.WithTimeout(ctx, review.DefaultCollectionTimeout)
	defer cancel()
	clock := s.Clock
	if clock == nil {
		clock = review.SystemClock{}
	}
	t := s.Transport
	if t == nil {
		t = SocketTransport{}
	}
	out := review.HerdrInventory{SchemaVersion: review.SchemaVersion, Target: target, Source: review.SourceStatus{Source: "herdr", Status: "unavailable", Provenance: "herdr-json-api-read-only"}, Capabilities: []string{}, Occupants: []review.HerdrOccupant{}, Warnings: []review.Warning{}}
	resolved, err := ResolveTarget(target, s.LookupEnv)
	out.Target = resolved
	if err != nil {
		out.Warnings = append(out.Warnings, warning("source_unavailable", "Explicit Herdr endpoint/session unavailable"))
		return out, nil
	}
	version := s.ClientVersion
	if version == nil {
		version = localClientVersion
	}
	if v, e := version(ctx); e == nil && v != "" {
		out.ClientVersion = &v
	} else {
		out.Warnings = append(out.Warnings, warning("client_version_unavailable", "Herdr client version unavailable"))
	}
	raw, err := t.Read(ctx, resolved.Endpoint, "ping", "")
	if err != nil {
		out.Warnings = append(out.Warnings, readWarning(ctx, "Herdr endpoint unavailable"))
		return out, nil
	}
	var pong struct {
		Type         string                     `json:"type"`
		Version      string                     `json:"version"`
		Protocol     int                        `json:"protocol"`
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	if json.Unmarshal(raw, &pong) != nil || pong.Type != "pong" || pong.Version == "" {
		out.Warnings = append(out.Warnings, warning("unsupported_capability", "Invalid Herdr protocol handshake"))
		return out, nil
	}
	out.ServerVersion = &pong.Version
	out.Capabilities = append(out.Capabilities, "ping", "protocol:"+strconv.Itoa(pong.Protocol))
	if pong.Protocol != 22 {
		out.Warnings = append(out.Warnings, warning("unsupported_capability", "Herdr protocol is not the characterized protocol 22"))
		return out, nil
	}
	for k, v := range pong.Capabilities {
		if string(v) == "true" {
			out.Capabilities = append(out.Capabilities, "server:"+k)
		}
	}
	first, err := readSnapshot(ctx, t, resolved.Endpoint)
	if err != nil {
		out.Warnings = append(out.Warnings, readWarning(ctx, "Herdr snapshot unavailable or incompatible"))
		return out, nil
	}
	if first.Version != pong.Version {
		out.Warnings = append(out.Warnings, warning("source_unavailable", "Herdr server changed during collection"))
		return out, nil
	}
	out.Capabilities = append(out.Capabilities, "session.snapshot")
	now := clock.Now()
	out.Source.CollectedAt = &now
	out.Source.Status = "fresh"
	reportedAgents := 0
	for _, p := range first.Panes {
		if isAgent(p) {
			reportedAgents++
		}
	}
	if first.Agents == nil || len(first.Agents) > reportedAgents {
		out.Source.Status = "partial"
		out.Warnings = append(out.Warnings, warning("partial_discovery", "Herdr agent inventory is incomplete or lacks matching pane metadata"))
	}
	serverID := "endpoint-sha256-" + digest(resolved.Endpoint)
	initial := map[string]observed{}
	duplicates := map[string]int{}
	paneIDs := map[string]int{}
	panes := []pane{}
	for _, p := range first.Panes {
		if isAgent(p) {
			if p.PaneID == "" || p.WorkspaceID == "" || p.TabID == "" {
				out.Source.Status = "partial"
				out.Warnings = append(out.Warnings, warning("partial_discovery", "Herdr response omitted required pane scope"))
				continue
			}
			panes = append(panes, p)
			duplicates[scopedPaneKey(p)]++
			paneIDs[p.PaneID]++
		}
	}
	sort.SliceStable(panes, func(i, j int) bool { return scopedPaneKey(panes[i]) < scopedPaneKey(panes[j]) })
	for _, p := range panes {
		if ctx.Err() != nil {
			break
		}
		if validPane(p) && duplicates[scopedPaneKey(p)] == 1 && paneIDs[p.PaneID] == 1 {
			initial[scopedPaneKey(p)] = s.observe(ctx, t, resolved.Endpoint, p)
		}
	}
	last, finalErr := readSnapshot(ctx, t, resolved.Endpoint)
	finalPanes := map[string][]pane{}
	if finalErr == nil && last.Version == first.Version {
		if last.Agents == nil {
			out.Source.Status = "partial"
			out.Warnings = append(out.Warnings, warning("partial_discovery", "Herdr final agent inventory is incomplete"))
		}
		for _, p := range last.Panes {
			finalPanes[scopedPaneKey(p)] = append(finalPanes[scopedPaneKey(p)], p)
		}
	} else {
		out.Source.Status = "partial"
		out.Warnings = append(out.Warnings, readWarning(ctx, "Herdr final identity snapshot unavailable"))
	}
	processReadSupported := false
	for _, a := range initial {
		if a.supported {
			processReadSupported = true
		}
	}
	for _, p := range panes {
		o := review.HerdrOccupant{ServerID: serverID, SessionID: resolved.SessionID, WorkspaceID: p.WorkspaceID, TabID: p.TabID, PaneID: p.PaneID, Correlation: "unknown", Lifecycle: review.LifecycleObservation{State: state(p.State), ObservedAt: &now, Provenance: "herdr-session.snapshot"}, Warnings: []review.Warning{}}
		key := scopedPaneKey(p)
		f := finalPanes[key]
		if len(f) == 1 && samePane(p, f[0]) {
			o.Lifecycle.State = state(f[0].State)
		}
		switch {
		case !validPane(p):
			o.Warnings = append(o.Warnings, warning("partial_discovery", "Herdr pane scope is incomplete"))
		case duplicates[key] > 1 || paneIDs[p.PaneID] > 1 || len(f) > 1:
			o.Correlation = "ambiguous"
			o.Warnings = append(o.Warnings, warning("duplicate_identity", "Duplicate scoped Herdr panes"))
		case finalErr != nil || len(f) != 1 || !samePane(p, f[0]):
			o.Warnings = append(o.Warnings, warning("identity_changed", "Herdr occupant changed or could not be revalidated"))
		default:
			a := initial[key]
			if a.err == nil && a.identity != nil {
				b := s.observe(ctx, t, resolved.Endpoint, f[0])
				processReadSupported = true
				if b.err == nil && b.identity != nil && a.fingerprint == b.fingerprint {
					o.Process = b.identity
					rev := "os-occupant-sha256-" + digest(resolved.Endpoint, serverID, resolved.SessionID, p.WorkspaceID, p.TabID, p.PaneID, b.fingerprint)
					o.Revision = &rev
					o.Correlation = "positive"
				} else {
					o.Warnings = append(o.Warnings, warning("identity_changed", "Agent process identity changed or could not be revalidated"))
				}
			} else {
				o.Warnings = append(o.Warnings, warning("uncorrelated", "No unique verified foreground terminal-agent identity"))
				if !a.supported {
					o.Warnings = append(o.Warnings, warning("unsupported_capability", "Herdr process identity read unavailable"))
				}
			}
		}
		if len(f) == 1 && samePane(p, f[0]) && f[0].Session != nil {
			r := f[0].Session
			if r.Source != "" && (r.Kind == "id" || r.Kind == "path") && r.Value != "" {
				ref := r.Value
				o.ResumeReference = &ref
			}
		}
		if len(o.Warnings) > 0 {
			out.Source.Status = "partial"
		}
		out.Occupants = append(out.Occupants, o)
	}
	if processReadSupported {
		out.Capabilities = append(out.Capabilities, "pane.process_info")
	}
	if ctx.Err() != nil {
		out.Source.Status = "partial"
		out.Warnings = append(out.Warnings, warning("deadline_exceeded", "Herdr collection deadline exceeded"))
	}
	sort.Strings(out.Capabilities)
	sort.Slice(out.Warnings, func(i, j int) bool { return out.Warnings[i].Code < out.Warnings[j].Code })
	return out, nil
}
func readWarning(ctx context.Context, message string) review.Warning {
	if ctx.Err() != nil {
		return warning("deadline_exceeded", message)
	}
	return warning("source_unavailable", message)
}
