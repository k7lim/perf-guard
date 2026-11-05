package livestatus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// LiveStatus mirrors contracts/live-status.json.
type LiveStatus struct {
	MonitorOnly     bool             `json:"monitor_only"`
	DiscoveryError  string           `json:"discovery_error,omitempty"`
	SchemaVersion   int              `json:"schema_version"`
	Level           string           `json:"level"`
	CompressionRate float64          `json:"compression_rate"`
	HelperCount     int              `json:"helper_count"`
	TopProcesses    []TopProcess     `json:"top_processes"`
	LastSpikeTime   string           `json:"last_spike_time,omitempty"`
	ForensicCount   int              `json:"forensic_count"`
	Suspended       []SuspendedBrief `json:"suspended"`
	VMStopped       bool             `json:"vm_stopped"`
	UpdatedAt       string           `json:"updated_at"`
}

type TopProcess struct {
	PID      int     `json:"pid"`
	Name     string  `json:"name"`
	RSSBytes uint64  `json:"rss_bytes"`
	CPUPct   float64 `json:"cpu_pct"`
}

type SuspendedBrief struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

// Path returns the default live-status file path.
func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "perf-guard", "live-status.json")
}

// Write atomically writes the live status to disk.
func Write(status *LiveStatus) error {
	return WriteTo(status, Path())
}

// WriteTo atomically writes the live status to a specific path.
func WriteTo(status *LiveStatus, path string) error {
	status.SchemaVersion = 1
	status.UpdatedAt = time.Now().Format(time.RFC3339)

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "live-status-*.json")
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

	return os.Rename(tmpPath, path)
}

// Read reads the live status from the default path.
func Read() (*LiveStatus, error) {
	return ReadFrom(Path())
}

// ReadFrom reads the live status from a specific path.
func ReadFrom(path string) (*LiveStatus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var status LiveStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
