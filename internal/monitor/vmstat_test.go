package monitor

import (
	"testing"
	"time"
)

const sampleOutput = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               48133.
Pages active:                            637177.
Pages inactive:                          644813.
Pages speculative:                         1046.
Pages throttled:                              0.
Pages wired down:                        658818.
Pages purgeable:                           4334.
"Translation faults":               20568008641.
Pages copy-on-write:                  458012929.
Pages zero filled:                  10549132400.
Pages reactivated:                   5549978266.
Pages purged:                         625998046.
File-backed pages:                       323344.
Anonymous pages:                         959692.
Pages stored in compressor:             7140608.
Pages occupied by compressor:           1113038.
Decompressions:                      6952802378.
Compressions:                        7844988409.
Pageins:                              253698702.
Pageouts:                               4844092.
Swapins:                              100147151.
Swapouts:                             106435881.
`

func TestParseVmStat(t *testing.T) {
	ts := time.Now()
	s, err := ParseVmStat(sampleOutput, ts)
	if err != nil {
		t.Fatalf("ParseVmStat: %v", err)
	}

	tests := []struct {
		name string
		got  uint64
		want uint64
	}{
		{"PageSize", s.PageSize, 16384},
		{"PagesFree", s.PagesFree, 48133},
		{"PagesActive", s.PagesActive, 637177},
		{"PagesInactive", s.PagesInactive, 644813},
		{"PagesWired", s.PagesWired, 658818},
		{"PagesCompressor", s.PagesCompressor, 7140608},
		{"Compressions", s.Compressions, 7844988409},
		{"Decompressions", s.Decompressions, 6952802378},
		{"Swapins", s.Swapins, 100147151},
		{"Swapouts", s.Swapouts, 106435881},
		{"CompressorOccupied", s.CompressorOccupied, 1113038},
		{"FileBackedPages", s.FileBackedPages, 323344},
		{"AnonymousPages", s.AnonymousPages, 959692},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestComputeRates(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(5 * time.Second)

	prev := VmStatSnapshot{
		Timestamp:    t0,
		Compressions: 1000,
		Swapins:      100,
	}
	curr := VmStatSnapshot{
		Timestamp:    t1,
		Compressions: 3500,
		Swapins:      200,
	}

	r := ComputeRates(prev, curr)

	if r.CompressionRate != 500.0 {
		t.Errorf("CompressionRate = %f, want 500.0", r.CompressionRate)
	}
	if r.SwapInRate != 20.0 {
		t.Errorf("SwapInRate = %f, want 20.0", r.SwapInRate)
	}
}
