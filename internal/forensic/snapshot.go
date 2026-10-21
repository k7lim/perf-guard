package forensic

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/k7lim/perf-guard/internal/monitor"
)

// Snapshot holds a complete forensic capture from a memory pressure spike.
type Snapshot struct {
	SchemaVersion int                    `json:"schema_version"`
	Timestamp     time.Time              `json:"timestamp"`
	Level         string                 `json:"level"`
	CompRate      float64                `json:"compression_rate"`
	DecompRate    float64                `json:"decompression_rate"`
	SwapInRate    float64                `json:"swap_in_rate"`
	SwapOutRate   float64                `json:"swap_out_rate"`
	Processes     []ProcessSnapshot      `json:"processes"`
	Tabs          []TabInfo              `json:"tabs,omitempty"`
	TabsError     string                 `json:"tabs_error,omitempty"`
	IO            IOStats                `json:"io"`
	VmStat        monitor.VmStatSnapshot `json:"vmstat"`
}

// SnapshotMeta holds lightweight metadata for listing snapshots.
type SnapshotMeta struct {
	Timestamp    time.Time `json:"timestamp"`
	Level        string    `json:"level"`
	CompRate     float64   `json:"compression_rate"`
	ProcessCount int       `json:"process_count"`
	TabCount     int       `json:"tab_count"`
	TabsError    string    `json:"tabs_error,omitempty"`
}

// ForensicsDir returns the default forensics directory path.
func ForensicsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "perf-guard", "forensics")
	}
	return filepath.Join(home, ".config", "perf-guard", "forensics")
}

// CaptureSnapshot assembles all forensic data sources into a single snapshot.
func CaptureSnapshot(level string, reading monitor.PressureReading, cdpPort int, fsUsage bool) (*Snapshot, error) {
	snap := &Snapshot{
		SchemaVersion: 1,
		Timestamp:     time.Now(),
		Level:         level,
		CompRate:      reading.CompressionRate,
		DecompRate:    reading.DecompressionRate,
		SwapInRate:    reading.SwapInRate,
		SwapOutRate:   reading.SwapOutRate,
		VmStat:        reading.Snapshot,
	}

	procs, err := CaptureProcesses(30)
	if err != nil {
		return nil, fmt.Errorf("process capture: %w", err)
	}
	snap.Processes = procs

	tabs, err := CaptureTabs(cdpPort)
	if err != nil {
		snap.TabsError = err.Error()
		log.Printf("forensic: tabs unavailable: %v", err)
	} else {
		snap.Tabs = tabs
	}

	io, err := CaptureIOStats(fsUsage)
	if err != nil {
		log.Printf("forensic: IO stats unavailable: %v", err)
		snap.IO = IOStats{Source: "error"}
	} else {
		snap.IO = io
	}

	return snap, nil
}

// SaveSnapshot atomically writes a snapshot to the forensics directory.
func SaveSnapshot(snap *Snapshot) error {
	return SaveSnapshotTo(snap, ForensicsDir())
}

// SaveSnapshotTo atomically writes a snapshot to a specific directory.
func SaveSnapshotTo(snap *Snapshot, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := snap.Timestamp.Format("2006-01-02T150405") + ".json"
	finalPath := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "snapshot-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, finalPath)
}

// RotateSnapshots keeps at most maxKeep snapshots, deleting the oldest.
func RotateSnapshots(maxKeep int) error {
	return RotateSnapshotsIn(ForensicsDir(), maxKeep)
}

// RotateSnapshotsIn keeps at most maxKeep snapshots in the given directory.
func RotateSnapshotsIn(dir string, maxKeep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var jsonFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), "snapshot-") {
			jsonFiles = append(jsonFiles, e.Name())
		}
	}
	sort.Strings(jsonFiles) // lexicographic = chronological for timestamp filenames

	if len(jsonFiles) <= maxKeep {
		return nil
	}

	toDelete := jsonFiles[:len(jsonFiles)-maxKeep]
	var errs []string
	for _, f := range toDelete {
		if err := os.Remove(filepath.Join(dir, f)); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("rotation: failed to delete %d/%d files: %s", len(errs), len(toDelete), strings.Join(errs, "; "))
	}
	return nil
}

// ListSnapshots returns snapshot metadata, newest first, with pagination.
func ListSnapshots(limit, offset int) ([]SnapshotMeta, int, error) {
	return ListSnapshotsFrom(ForensicsDir(), limit, offset)
}

// ListSnapshotsFrom returns snapshot metadata from a specific directory.
func ListSnapshotsFrom(dir string, limit, offset int) ([]SnapshotMeta, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SnapshotMeta{}, 0, nil
		}
		return nil, 0, err
	}

	var jsonFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), "snapshot-") {
			jsonFiles = append(jsonFiles, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(jsonFiles))) // newest first

	total := len(jsonFiles)

	// Apply pagination
	if offset >= len(jsonFiles) {
		return []SnapshotMeta{}, total, nil
	}
	jsonFiles = jsonFiles[offset:]
	if limit > 0 && limit < len(jsonFiles) {
		jsonFiles = jsonFiles[:limit]
	}

	var metas []SnapshotMeta
	var skipped int
	for _, f := range jsonFiles {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			skipped++
			log.Printf("forensic: skipping unreadable snapshot %s: %v", f, err)
			continue
		}
		var snap Snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			skipped++
			log.Printf("forensic: skipping corrupt snapshot %s: %v", f, err)
			continue
		}
		metas = append(metas, SnapshotMeta{
			Timestamp:    snap.Timestamp,
			Level:        snap.Level,
			CompRate:     snap.CompRate,
			ProcessCount: len(snap.Processes),
			TabCount:     len(snap.Tabs),
			TabsError:    snap.TabsError,
		})
	}
	return metas, total, nil
}

// LoadSnapshot finds and loads a snapshot by timestamp prefix.
func LoadSnapshot(prefix string) (*Snapshot, error) {
	return LoadSnapshotFrom(ForensicsDir(), prefix)
}

// LoadSnapshotFrom finds and loads a snapshot from a specific directory.
func LoadSnapshotFrom(dir, prefix string) (*Snapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			nameWithoutExt := strings.TrimSuffix(e.Name(), ".json")
			if strings.HasPrefix(nameWithoutExt, prefix) {
				matches = append(matches, e.Name())
			}
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no snapshot matching prefix %q", prefix)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("ambiguous prefix %q matches %d snapshots", prefix, len(matches))
	}

	data, err := os.ReadFile(filepath.Join(dir, matches[0]))
	if err != nil {
		return nil, err
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}
