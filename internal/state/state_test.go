package state

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	orig := State{VMStopped: true}
	orig.AddSuspended(100, "chrome", "browser")
	orig.AddSuspended(200, "code", "editor")

	if err := orig.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if loaded.VMStopped != orig.VMStopped {
		t.Errorf("VMStopped = %v, want %v", loaded.VMStopped, orig.VMStopped)
	}
	if len(loaded.Suspended) != 2 {
		t.Fatalf("Suspended len = %d, want 2", len(loaded.Suspended))
	}
	if loaded.Suspended[0].PID != 100 || loaded.Suspended[1].PID != 200 {
		t.Errorf("PIDs = [%d,%d], want [100,200]", loaded.Suspended[0].PID, loaded.Suspended[1].PID)
	}
}

func TestLoadFromMissingFile(t *testing.T) {
	s, err := LoadFrom(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(s.Suspended) != 0 || s.VMStopped {
		t.Errorf("expected zero state, got %+v", s)
	}
}

func TestAddSuspended(t *testing.T) {
	var s State
	s.AddSuspended(42, "firefox", "browser")

	if len(s.Suspended) != 1 {
		t.Fatalf("len = %d, want 1", len(s.Suspended))
	}
	sp := s.Suspended[0]
	if sp.PID != 42 || sp.Name != "firefox" || sp.Group != "browser" {
		t.Errorf("got %+v", sp)
	}
	if sp.SuspendedAt.IsZero() {
		t.Error("SuspendedAt should be set")
	}
	if s.LastAction.IsZero() {
		t.Error("LastAction should be set")
	}
}

func TestRemovePID(t *testing.T) {
	tests := []struct {
		name      string
		initial   []int
		remove    int
		wantPIDs  []int
	}{
		{"remove middle", []int{1, 2, 3}, 2, []int{1, 3}},
		{"remove first", []int{1, 2, 3}, 1, []int{2, 3}},
		{"remove last", []int{1, 2, 3}, 3, []int{1, 2}},
		{"remove absent", []int{1, 2}, 99, []int{1, 2}},
		{"remove from empty", nil, 1, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s State
			for _, pid := range tt.initial {
				s.AddSuspended(pid, "proc", "group")
			}
			s.RemovePID(tt.remove)

			got := make([]int, len(s.Suspended))
			for i, sp := range s.Suspended {
				got[i] = sp.PID
			}
			if len(got) != len(tt.wantPIDs) {
				t.Fatalf("PIDs = %v, want %v", got, tt.wantPIDs)
			}
			for i := range got {
				if got[i] != tt.wantPIDs[i] {
					t.Fatalf("PIDs = %v, want %v", got, tt.wantPIDs)
				}
			}
		})
	}
}

func TestClear(t *testing.T) {
	var s State
	s.AddSuspended(1, "a", "g")
	s.AddSuspended(2, "b", "g")
	s.VMStopped = true

	s.Clear()

	if len(s.Suspended) != 0 {
		t.Errorf("Suspended len = %d after Clear", len(s.Suspended))
	}
	if s.VMStopped {
		t.Error("VMStopped should be false after Clear")
	}
}

func TestJSONFieldNames(t *testing.T) {
	var s State
	s.AddSuspended(10, "proc", "grp")
	s.VMStopped = true

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, key := range []string{"suspended", "vm_stopped", "last_action"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}

	var procs []map[string]json.RawMessage
	if err := json.Unmarshal(raw["suspended"], &procs); err != nil {
		t.Fatalf("Unmarshal suspended: %v", err)
	}
	for _, key := range []string{"pid", "name", "group", "suspended_at"} {
		if _, ok := procs[0][key]; !ok {
			t.Errorf("missing process key %q", key)
		}
	}
}
