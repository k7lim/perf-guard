package forensic

import (
	"testing"
)

const samplePS = `  PID   RSS   VSZ  %CPU %MEM COMM
 1234 1257812 4394000  34.2  2.7 /Applications/Brave Browser.app/Contents/Frameworks/Brave Browser Helper (Renderer)
  567 524288 2000000  12.1  1.1 /usr/local/bin/node
    1  15000  400000   0.0  0.0 /sbin/launchd
  890 200000 1000000   5.5  0.4 /usr/bin/some process
    0  50000  300000   0.0  0.0 kernel_task
`

func TestParseProcessSnapshot(t *testing.T) {
	results, parseErrs, totalLines := parseProcessSnapshot(samplePS, 10)

	// kernel_task (PID 0) should be skipped (counted as parse error)
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4", len(results))
	}
	if parseErrs != 1 { // PID 0
		t.Errorf("parseErrs = %d, want 1", parseErrs)
	}
	if totalLines != 5 {
		t.Errorf("totalLines = %d, want 5", totalLines)
	}

	// Should be sorted by RSS descending
	if results[0].PID != 1234 {
		t.Errorf("first PID = %d, want 1234 (highest RSS)", results[0].PID)
	}
	if results[1].PID != 567 {
		t.Errorf("second PID = %d, want 567", results[1].PID)
	}

	// Check field conversions
	if results[0].RSSBytes != 1257812*1024 {
		t.Errorf("RSSBytes = %d, want %d", results[0].RSSBytes, 1257812*1024)
	}
	if results[0].VSZBytes != 4394000*1024 {
		t.Errorf("VSZBytes = %d, want %d", results[0].VSZBytes, 4394000*1024)
	}
	if results[0].CPUPct != 34.2 {
		t.Errorf("CPUPct = %f, want 34.2", results[0].CPUPct)
	}
	if results[0].MEMPct != 2.7 {
		t.Errorf("MEMPct = %f, want 2.7", results[0].MEMPct)
	}

	// Process name with spaces
	want := "/Applications/Brave Browser.app/Contents/Frameworks/Brave Browser Helper (Renderer)"
	if results[0].Name != want {
		t.Errorf("Name = %q, want %q", results[0].Name, want)
	}
}

func TestParseProcessSnapshot_Limit(t *testing.T) {
	results, _, _ := parseProcessSnapshot(samplePS, 2)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	// Top 2 by RSS
	if results[0].PID != 1234 || results[1].PID != 567 {
		t.Errorf("wrong top 2: PIDs %d, %d", results[0].PID, results[1].PID)
	}
}

func TestParseProcessSnapshot_Empty(t *testing.T) {
	results, _, _ := parseProcessSnapshot("", 10)
	if len(results) != 0 {
		t.Errorf("got %d results for empty input, want 0", len(results))
	}
}
