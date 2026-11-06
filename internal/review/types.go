// Package review defines the additive, versioned read-only M1 boundary.
// Providers expose collection only; no process or Herdr control methods exist.
package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

const SchemaVersion = 1
const DefaultCollectionTimeout = 5 * time.Second
const DefaultFreshness = 30 * time.Second
const DefaultLimit = 50
const MaxLimit = 200

type Clock interface{ Now() time.Time }
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type ProcessSource interface {
	Collect(context.Context) (ProcessInventory, error)
}
type HerdrSource interface {
	Collect(context.Context, HerdrTarget) (HerdrInventory, error)
}
type WorktreeSource interface {
	Inspect(context.Context, string) WorktreeObservation
}

type ProcessIdentity struct {
	PID     int    `json:"pid"`
	StartID string `json:"start_id"`
}

type HerdrTarget struct {
	Endpoint  string `json:"endpoint"`
	SessionID string `json:"session_id"`
}

type OccupantIdentity struct {
	Endpoint    string          `json:"endpoint"`
	ServerID    string          `json:"server_id"`
	SessionID   string          `json:"session_id"`
	WorkspaceID string          `json:"workspace_id"`
	TabID       string          `json:"tab_id"`
	PaneID      string          `json:"pane_id"`
	Revision    string          `json:"revision"`
	Process     ProcessIdentity `json:"process"`
}

// ReviewID binds every scope and occupant component, including actual process
// start identity. JSON array encoding is unambiguous even for separator strings.
// Any occupant replacement produces a different ID; never use pane ID alone.
func ReviewID(i OccupantIdentity) string {
	b, _ := json.Marshal([]any{i.Endpoint, i.ServerID, i.SessionID, i.WorkspaceID, i.TabID, i.PaneID, i.Revision, i.Process.PID, i.Process.StartID})
	h := sha256.Sum256(b)
	return "review-v1-" + hex.EncodeToString(h[:])
}

type Warning struct {
	Code    string `json:"code"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

type SourceStatus struct {
	Source      string     `json:"source"`
	Status      string     `json:"status"` // fresh, stale, unavailable, partial
	CollectedAt *time.Time `json:"collected_at"`
	Provenance  string     `json:"provenance"`
}

type MemoryObservation struct {
	ResidentRSSBytes       *uint64 `json:"resident_rss_bytes"`
	SummedRSSIsEstimate    bool    `json:"summed_rss_is_estimate"`
	PhysicalFootprintBytes *uint64 `json:"physical_footprint_bytes"`
	CompressedBytes        *uint64 `json:"compressed_bytes"`
	SwapBytes              *uint64 `json:"swap_bytes"`
	Provenance             string  `json:"provenance"`
}

type ProcessRecord struct {
	PID          int               `json:"pid"`
	Identity     *ProcessIdentity  `json:"identity"`
	ParentPID    int               `json:"parent_pid"`
	Owner        *ProcessIdentity  `json:"owner"`
	Agent        string            `json:"agent"`
	Kind         string            `json:"kind"` // terminal, helper, desktop_helper, app_server, background
	TTY          *string           `json:"tty"`
	CWD          *string           `json:"cwd"`
	AgeSeconds   *float64          `json:"age_seconds"`
	Memory       MemoryObservation `json:"memory"`
	ObservedAt   time.Time         `json:"observed_at"`
	FieldSources map[string]string `json:"field_sources"`
	Warnings     []Warning         `json:"warnings"`
}

type ProcessGroup struct {
	Root    ProcessIdentity   `json:"root"`
	Members []ProcessIdentity `json:"members"`
	Memory  MemoryObservation `json:"memory"`
}

type ProcessInventory struct {
	SchemaVersion int             `json:"schema_version"`
	Source        SourceStatus    `json:"source"`
	Processes     []ProcessRecord `json:"processes"`
	Groups        []ProcessGroup  `json:"groups"`
	Warnings      []Warning       `json:"warnings"`
}

// ActivityObservation is a time-bearing fact, not an inferred idle timer.
// User/conversation activity may use only corresponding explicit evidence.
type ActivityObservation struct {
	At         time.Time `json:"at"`
	Provenance string    `json:"provenance"`
}

type UserActivityObservation ActivityObservation
type ConversationActivityObservation ActivityObservation

type LifecycleObservation struct {
	State      string     `json:"state"` // idle, done, working, blocked, unknown
	ObservedAt *time.Time `json:"observed_at"`
	Provenance string     `json:"provenance"`
}

type HerdrOccupant struct {
	ServerID             string                           `json:"server_id"`
	SessionID            string                           `json:"session_id"`
	WorkspaceID          string                           `json:"workspace_id"`
	TabID                string                           `json:"tab_id"`
	PaneID               string                           `json:"pane_id"`
	Revision             *string                          `json:"revision"`
	Process              *ProcessIdentity                 `json:"process"`
	Correlation          string                           `json:"correlation"` // positive, unknown, ambiguous
	Lifecycle            LifecycleObservation             `json:"lifecycle"`
	UserActivity         *UserActivityObservation         `json:"user_activity"`
	ConversationActivity *ConversationActivityObservation `json:"conversation_activity"`
	ResumeReference      *string                          `json:"resume_reference"`
	Warnings             []Warning                        `json:"warnings"`
}

type HerdrInventory struct {
	SchemaVersion int             `json:"schema_version"`
	Target        HerdrTarget     `json:"target"`
	Source        SourceStatus    `json:"source"`
	ClientVersion *string         `json:"client_version"`
	ServerVersion *string         `json:"server_version"`
	Capabilities  []string        `json:"capabilities"`
	Occupants     []HerdrOccupant `json:"occupants"`
	Warnings      []Warning       `json:"warnings"`
}

type WorktreeObservation struct {
	State      string     `json:"state"` // clean, dirty, unknown, not_repository
	ObservedAt *time.Time `json:"observed_at"`
	Provenance string     `json:"provenance"`
	Warnings   []Warning  `json:"warnings"`
}

type ChildWorkObservation struct {
	State      string     `json:"state"` // working, none_observed, unknown
	Provenance string     `json:"provenance"`
	ObservedAt *time.Time `json:"observed_at"`
}

type ReviewRecord struct {
	ID                   string                           `json:"id"`
	Identity             *OccupantIdentity                `json:"identity"`
	Process              *ProcessIdentity                 `json:"process"`
	Agent                string                           `json:"agent"`
	Project              *string                          `json:"project"`
	CWD                  *string                          `json:"cwd"`
	AgeSeconds           *float64                         `json:"age_seconds"`
	ProcessObservedAt    *time.Time                       `json:"process_observed_at"`
	ProcessProvenance    string                           `json:"process_provenance"`
	Memory               MemoryObservation                `json:"memory"`
	Lifecycle            LifecycleObservation             `json:"lifecycle"`
	UserActivity         *UserActivityObservation         `json:"user_activity"`
	ConversationActivity *ConversationActivityObservation `json:"conversation_activity"`
	ChildWork            ChildWorkObservation             `json:"child_work"`
	Worktree             WorktreeObservation              `json:"worktree"`
	ResumeReference      *string                          `json:"resume_reference"`
	Reviewable           bool                             `json:"reviewable"`
	Reasons              []string                         `json:"reasons"`
	Warnings             []Warning                        `json:"warnings"`
}

type ReviewResponse struct {
	SchemaVersion   int            `json:"schema_version"`
	CollectedAt     time.Time      `json:"collected_at"`
	Target          HerdrTarget    `json:"target"`
	Sources         []SourceStatus `json:"sources"`
	Records         []ReviewRecord `json:"records"`
	Total           int            `json:"total"`
	ReviewableTotal int            `json:"reviewable_total"`
	Limit           int            `json:"limit"`
	Offset          int            `json:"offset"`
	Warnings        []Warning      `json:"warnings"`
}

type PlanSelection struct {
	RequestedID       string        `json:"requested_id"`
	Status            string        `json:"status"` // reviewable, excluded, missing, duplicate
	Record            *ReviewRecord `json:"record"`
	Reasons           []string      `json:"reasons"`
	RecoveryAvailable bool          `json:"recovery_available"`
	PreserveShell     bool          `json:"preserve_shell"`
}

type ReviewPlan struct {
	SchemaVersion      int             `json:"schema_version"`
	CollectedAt        time.Time       `json:"collected_at"`
	Target             HerdrTarget     `json:"target"`
	ExecutionSupported bool            `json:"execution_supported"`
	Selections         []PlanSelection `json:"selections"`
	Sources            []SourceStatus  `json:"sources"`
	Warnings           []Warning       `json:"warnings"`
}

// Reason codes are owned by the shared policy (#8), rendered verbatim by UI.
const (
	ReasonIdle         = "fresh_idle"
	ReasonDone         = "fresh_done"
	ReasonWorking      = "working"
	ReasonBlocked      = "blocked"
	ReasonUnknown      = "unknown_state"
	ReasonStale        = "stale_source"
	ReasonUncorrelated = "uncorrelated"
	ReasonCurrent      = "current_controller"
	ReasonIdentity     = "identity_unavailable"
	ReasonDuplicate    = "duplicate_identity"
	ReasonChildWork    = "child_working"
	ReasonMissing      = "selection_missing_or_changed"
	ReasonRepeated     = "duplicate_selection"
)

// ExcludedID is a stable display-only identifier for uncorrelated observations.
// It can never authorize a reviewable selection. Components are scope fields for
// Herdr records, or PID/start identity (if known) for process-only records.
func ExcludedID(kind string, components ...string) string {
	fields := append([]string{kind}, components...)
	b, _ := json.Marshal(fields)
	h := sha256.Sum256(b)
	return "excluded-v1-" + hex.EncodeToString(h[:])
}
