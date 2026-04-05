package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SuspendedProcess records a process we suspended.
type SuspendedProcess struct {
	PID         int       `json:"pid"`
	Name        string    `json:"name"`
	Group       string    `json:"group"`
	SuspendedAt time.Time `json:"suspended_at"`
}

// State tracks all processes we've suspended and VM status.
type State struct {
	Suspended  []SuspendedProcess `json:"suspended"`
	VMStopped  bool               `json:"vm_stopped"`
	LastAction time.Time          `json:"last_action"`
}

// StatePath returns the default state file location.
func StatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "perf-guard", "state.json")
}

// Load reads state from disk. Returns empty state if file doesn't exist.
func Load() (State, error) {
	return LoadFrom(StatePath())
}

// LoadFrom reads state from a specific path.
func LoadFrom(path string) (State, error) {
	var s State
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

// Save writes state to disk.
func (s *State) Save() error {
	return s.SaveTo(StatePath())
}

// SaveTo writes state to a specific path.
func (s *State) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// AddSuspended records a newly suspended process.
func (s *State) AddSuspended(pid int, name, group string) {
	s.Suspended = append(s.Suspended, SuspendedProcess{
		PID:         pid,
		Name:        name,
		Group:       group,
		SuspendedAt: time.Now(),
	})
	s.LastAction = time.Now()
}

// Clear removes all suspended process records.
func (s *State) Clear() {
	s.Suspended = nil
	s.VMStopped = false
}

// RemovePID removes a specific PID from the suspended list.
func (s *State) RemovePID(pid int) {
	filtered := s.Suspended[:0]
	for _, sp := range s.Suspended {
		if sp.PID != pid {
			filtered = append(filtered, sp)
		}
	}
	s.Suspended = filtered
}
