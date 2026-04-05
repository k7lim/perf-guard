package forensic

import (
	"testing"
)

const sampleIOreg = `+-o disk0s1  <class IOMedia, id 0x10000028f, registered, matched, active, busy 0 (0 ms), retain 7>
  | {
  |   "Content Hint" = "Apple_APFS"
  |   "Bytes (Read)" = 45821034496
  |   "Bytes (Write)" = 23847102464
  | }
+-o disk0s2  <class IOMedia, id 0x1000002a1, registered, matched, active, busy 0 (0 ms), retain 5>
  | {
  |   "Bytes (Read)" = 1024000
  |   "Bytes (Write)" = 512000
  | }
+-o disk0s3  <class IOMedia, id 0x1000002b3, registered, matched, active, busy 0 (0 ms), retain 3>
  | {
  |   "Content Hint" = "Apple_Boot"
  | }
`

func TestParseIOreg_MultipleDevices(t *testing.T) {
	stats := parseIOreg(sampleIOreg)
	wantRead := int64(45821034496 + 1024000)
	wantWrite := int64(23847102464 + 512000)

	if stats.ReadBytes != wantRead {
		t.Errorf("ReadBytes = %d, want %d", stats.ReadBytes, wantRead)
	}
	if stats.WriteBytes != wantWrite {
		t.Errorf("WriteBytes = %d, want %d", stats.WriteBytes, wantWrite)
	}
	if stats.Source != "ioreg" {
		t.Errorf("Source = %q, want %q", stats.Source, "ioreg")
	}
}

func TestParseIOreg_Empty(t *testing.T) {
	stats := parseIOreg("")
	if stats.ReadBytes != 0 || stats.WriteBytes != 0 {
		t.Errorf("expected zero stats for empty input, got %+v", stats)
	}
}

func TestParseIOreg_MissingFields(t *testing.T) {
	// Device with only read, no write
	input := `+-o disk0s1  <class IOMedia>
  | {
  |   "Bytes (Read)" = 999
  | }
`
	stats := parseIOreg(input)
	if stats.ReadBytes != 999 {
		t.Errorf("ReadBytes = %d, want 999", stats.ReadBytes)
	}
	if stats.WriteBytes != 0 {
		t.Errorf("WriteBytes = %d, want 0", stats.WriteBytes)
	}
}

func TestCaptureIOStats_FSUsage(t *testing.T) {
	_, err := CaptureIOStats(true)
	if err == nil {
		t.Error("expected error for fsUsage=true")
	}
}
