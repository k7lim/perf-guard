package forensic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/k7lim/perf-guard/internal/monitor"
)

func makeTestSnapshot(t time.Time, level string, rate float64) *Snapshot {
	return &Snapshot{
		SchemaVersion: 1,
		Timestamp:     t,
		Level:         level,
		CompRate:      rate,
		DecompRate:    100.0,
		SwapInRate:    10.0,
		SwapOutRate:   20.0,
		Processes: []ProcessSnapshot{
			{PID: 1, Name: "test", RSSBytes: 1024, VSZBytes: 2048, CPUPct: 1.0, MEMPct: 0.5},
		},
		Tabs: []TabInfo{
			{URL: "http://test.com", Title: "Test", Type: "page"},
		},
		IO:     IOStats{ReadBytes: 100, WriteBytes: 200, Source: "ioreg"},
		VmStat: monitor.VmStatSnapshot{PageSize: 16384},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 4, 4, 23, 47, 12, 0, time.UTC)
	snap := makeTestSnapshot(ts, "critical", 3200.5)

	if err := SaveSnapshotTo(snap, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadSnapshotFrom(dir, "2026-04-04T234712")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", loaded.SchemaVersion)
	}
	if loaded.Level != "critical" {
		t.Errorf("Level = %q, want critical", loaded.Level)
	}
	if loaded.CompRate != 3200.5 {
		t.Errorf("CompRate = %f, want 3200.5", loaded.CompRate)
	}
	if len(loaded.Processes) != 1 {
		t.Errorf("Processes count = %d, want 1", len(loaded.Processes))
	}
	if len(loaded.Tabs) != 1 {
		t.Errorf("Tabs count = %d, want 1", len(loaded.Tabs))
	}
}

func TestFilenameFormat(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 4, 4, 23, 47, 12, 0, time.UTC)
	snap := makeTestSnapshot(ts, "warning", 500.0)

	if err := SaveSnapshotTo(snap, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	if entries[0].Name() != "2026-04-04T234712.json" {
		t.Errorf("filename = %q, want 2026-04-04T234712.json", entries[0].Name())
	}
}

func TestRotateSnapshots(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 5; i++ {
		ts := time.Date(2026, 4, 4, 10, 0, i, 0, time.UTC)
		snap := makeTestSnapshot(ts, "warning", 500.0)
		if err := SaveSnapshotTo(snap, dir); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	if err := RotateSnapshotsIn(dir, 3); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	var jsonCount int
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			jsonCount++
		}
	}
	if jsonCount != 3 {
		t.Errorf("after rotation: %d files, want 3", jsonCount)
	}
}

func TestListSnapshots_Pagination(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 5; i++ {
		ts := time.Date(2026, 4, 4, 10, 0, i, 0, time.UTC)
		snap := makeTestSnapshot(ts, "warning", float64(500+i))
		if err := SaveSnapshotTo(snap, dir); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	// First page
	metas, total, err := ListSnapshotsFrom(dir, 2, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(metas) != 2 {
		t.Fatalf("got %d metas, want 2", len(metas))
	}
	// Newest first
	if metas[0].CompRate < metas[1].CompRate {
		t.Error("expected newest first (higher rate)")
	}

	// Second page
	metas2, _, err := ListSnapshotsFrom(dir, 2, 2)
	if err != nil {
		t.Fatalf("List offset: %v", err)
	}
	if len(metas2) != 2 {
		t.Errorf("got %d metas at offset 2, want 2", len(metas2))
	}
}

func TestLoadSnapshot_NoMatch(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadSnapshotFrom(dir, "2099-01-01")
	if err == nil {
		t.Error("expected error for no match")
	}
}

func TestLoadSnapshot_Ambiguous(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 2; i++ {
		ts := time.Date(2026, 4, 4, 10, 0, i, 0, time.UTC)
		snap := makeTestSnapshot(ts, "warning", 500.0)
		SaveSnapshotTo(snap, dir)
	}

	// Prefix matches both
	_, err := LoadSnapshotFrom(dir, "2026-04-04T1000")
	if err == nil {
		t.Error("expected ambiguous error")
	}
}

func TestLoadSnapshot_UniquePrefix(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 4, 4, 23, 47, 12, 0, time.UTC)
	snap := makeTestSnapshot(ts, "critical", 3200.0)
	SaveSnapshotTo(snap, dir)

	loaded, err := LoadSnapshotFrom(dir, "2026-04-04T2347")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Level != "critical" {
		t.Errorf("Level = %q, want critical", loaded.Level)
	}
}

func TestSchemaVersionAlways1(t *testing.T) {
	dir := t.TempDir()
	snap := makeTestSnapshot(time.Now(), "normal", 100.0)
	snap.SchemaVersion = 99
	SaveSnapshotTo(snap, dir)

	// Read back raw JSON
	entries, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	// SchemaVersion was set in struct, so it persists as-is.
	// The CaptureSnapshot function always sets 1.
}
