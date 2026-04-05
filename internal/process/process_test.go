package process

import (
	"testing"
)

func TestParsePSAll(t *testing.T) {
	output := `  PID   RSS  %CPU COMM
 1234 1257812  34.2 /Applications/Brave Browser.app/Contents/Frameworks/Brave Browser Helper (Renderer)
  567 524288  12.1 /usr/local/bin/node
    1  15000   0.0 /sbin/launchd
`
	results := parsePSAll(output)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// First entry
	if results[0].PID != 1234 {
		t.Errorf("PID = %d, want 1234", results[0].PID)
	}
	if results[0].RSS != 1257812*1024 {
		t.Errorf("RSS = %d, want %d", results[0].RSS, 1257812*1024)
	}
	if results[0].CPU != 34.2 {
		t.Errorf("CPU = %f, want 34.2", results[0].CPU)
	}
	// Process name with spaces and parentheses
	want := "/Applications/Brave Browser.app/Contents/Frameworks/Brave Browser Helper (Renderer)"
	if results[0].Name != want {
		t.Errorf("Name = %q, want %q", results[0].Name, want)
	}

	// Second entry
	if results[1].PID != 567 {
		t.Errorf("PID = %d, want 567", results[1].PID)
	}
}

func TestTopByRSS(t *testing.T) {
	m := NewManager([]string{"launchd"})

	// We can't mock ps easily, but we can test the parsePSAll + sorting logic
	// by testing the method contract
	t.Run("zero returns empty", func(t *testing.T) {
		got, err := m.TopByRSS(0)
		if err != nil {
			t.Fatalf("TopByRSS(0): %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d results, want 0", len(got))
		}
	})

	t.Run("positive n returns results", func(t *testing.T) {
		got, err := m.TopByRSS(5)
		if err != nil {
			t.Fatalf("TopByRSS(5): %v", err)
		}
		if len(got) > 5 {
			t.Errorf("got %d results, want <= 5", len(got))
		}
		// Verify sorted by RSS desc
		for i := 1; i < len(got); i++ {
			if got[i].RSS > got[i-1].RSS {
				t.Errorf("not sorted: index %d RSS=%d > index %d RSS=%d", i, got[i].RSS, i-1, got[i-1].RSS)
			}
		}
		// Verify no protected processes
		for _, p := range got {
			if p.Name == "launchd" || contains(p.Name, "launchd") {
				t.Errorf("protected process %q in results", p.Name)
			}
		}
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
