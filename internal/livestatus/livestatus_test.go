package livestatus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live-status.json")

	status := &LiveStatus{
		Level:           "warning",
		CompressionRate: 842.5,
		HelperCount:     127,
		TopProcesses: []TopProcess{
			{PID: 1234, Name: "Brave Browser Helper", RSSBytes: 1288000000, CPUPct: 34.2},
		},
		LastSpikeTime: "2026-04-04T23:47:12-07:00",
		ForensicCount: 7,
		Suspended:     []SuspendedBrief{{PID: 5678, Name: "firefox"}},
		VMStopped:     true,
	}

	if err := WriteTo(status, path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got.Level != "warning" {
		t.Errorf("Level = %q, want %q", got.Level, "warning")
	}
	if got.CompressionRate != 842.5 {
		t.Errorf("CompressionRate = %f, want 842.5", got.CompressionRate)
	}
	if got.HelperCount != 127 {
		t.Errorf("HelperCount = %d, want 127", got.HelperCount)
	}
	if len(got.TopProcesses) != 1 || got.TopProcesses[0].PID != 1234 {
		t.Errorf("TopProcesses mismatch: %+v", got.TopProcesses)
	}
	if got.ForensicCount != 7 {
		t.Errorf("ForensicCount = %d, want 7", got.ForensicCount)
	}
	if len(got.Suspended) != 1 || got.Suspended[0].PID != 5678 {
		t.Errorf("Suspended mismatch: %+v", got.Suspended)
	}
	if !got.VMStopped {
		t.Error("VMStopped = false, want true")
	}
}

func TestSchemaVersionAlways1(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live-status.json")

	status := &LiveStatus{SchemaVersion: 99, Level: "normal"}
	if err := WriteTo(status, path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", got.SchemaVersion)
	}
}

func TestUpdatedAtAutoSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live-status.json")

	status := &LiveStatus{Level: "normal"}
	if err := WriteTo(status, path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.UpdatedAt == "" {
		t.Error("UpdatedAt is empty, expected auto-set")
	}
}

func TestReadMissingFile(t *testing.T) {
	_, err := ReadFrom("/nonexistent/path/live-status.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestForwardCompatibility(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live-status.json")

	// Write JSON with an unknown field
	data := []byte(`{
		"schema_version": 1,
		"level": "normal",
		"compression_rate": 0,
		"helper_count": 0,
		"top_processes": [],
		"forensic_count": 0,
		"suspended": [],
		"vm_stopped": false,
		"updated_at": "2026-04-04T00:00:00Z",
		"unknown_future_field": "should be ignored"
	}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("Read with unknown field: %v", err)
	}
	if got.Level != "normal" {
		t.Errorf("Level = %q, want %q", got.Level, "normal")
	}

	// Re-marshal should not include unknown field
	remarshaled, _ := json.Marshal(got)
	var m map[string]any
	json.Unmarshal(remarshaled, &m)
	if _, ok := m["unknown_future_field"]; ok {
		t.Error("unknown field should not survive round-trip through struct")
	}
}
