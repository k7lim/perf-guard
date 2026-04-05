package monitor

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// VmStatSnapshot holds raw counters from a single vm_stat invocation.
type VmStatSnapshot struct {
	Timestamp          time.Time
	PageSize           uint64
	PagesFree          uint64
	PagesActive        uint64
	PagesInactive      uint64
	PagesWired         uint64
	PagesCompressor    uint64 // "Pages stored in compressor"
	Compressions       uint64
	Decompressions     uint64
	Swapins            uint64
	Swapouts           uint64
	PagesSpeculative   uint64
	PagesThrottled     uint64
	PagesPurgeable     uint64
	PagesReactivated   uint64
	PagesCopyOnWrite   uint64
	PagesZeroFilled    uint64
	PagesPurged        uint64
	FileBackedPages    uint64
	AnonymousPages     uint64
	CompressorOccupied uint64 // "Pages occupied by compressor"
	Pageins            uint64
	Pageouts           uint64
}

// PressureReading holds derived rates between two snapshots.
type PressureReading struct {
	CompressionRate   float64 // pages/sec
	DecompressionRate float64 // pages/sec
	SwapInRate        float64 // pages/sec
	SwapOutRate       float64 // pages/sec
	Interval          time.Duration
	Snapshot          VmStatSnapshot // the latest snapshot for absolute values
}

// ComputeRates calculates per-second rates between two snapshots.
func ComputeRates(prev, curr VmStatSnapshot) PressureReading {
	dt := curr.Timestamp.Sub(prev.Timestamp).Seconds()
	if dt <= 0 {
		dt = 1 // avoid division by zero
	}
	return PressureReading{
		CompressionRate:   float64(curr.Compressions-prev.Compressions) / dt,
		DecompressionRate: float64(curr.Decompressions-prev.Decompressions) / dt,
		SwapInRate:        float64(curr.Swapins-prev.Swapins) / dt,
		SwapOutRate:       float64(curr.Swapouts-prev.Swapouts) / dt,
		Interval:          curr.Timestamp.Sub(prev.Timestamp),
		Snapshot:          curr,
	}
}

// TakeSnapshot runs /usr/bin/vm_stat and parses the output.
func TakeSnapshot() (VmStatSnapshot, error) {
	out, err := exec.Command("/usr/bin/vm_stat").Output()
	if err != nil {
		return VmStatSnapshot{}, fmt.Errorf("vm_stat: %w", err)
	}
	return ParseVmStat(string(out), time.Now())
}

// ParseVmStat parses vm_stat output into a VmStatSnapshot.
// Exported for testing.
func ParseVmStat(output string, ts time.Time) (VmStatSnapshot, error) {
	s := VmStatSnapshot{Timestamp: ts}

	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		return s, fmt.Errorf("vm_stat: unexpected output")
	}

	// First line: "Mach Virtual Memory Statistics: (page size of 16384 bytes)"
	if idx := strings.Index(lines[0], "page size of "); idx >= 0 {
		rest := lines[0][idx+len("page size of "):]
		rest = strings.TrimSuffix(rest, ")")
		rest = strings.TrimSpace(rest)
		rest = strings.TrimSuffix(rest, " bytes")
		if v, err := strconv.ParseUint(rest, 10, 64); err == nil {
			s.PageSize = v
		}
	}
	if s.PageSize == 0 {
		s.PageSize = 16384 // default for Apple Silicon
	}

	for _, line := range lines[1:] {
		key, val, ok := parseLine(line)
		if !ok {
			continue
		}
		switch key {
		case "Pages free":
			s.PagesFree = val
		case "Pages active":
			s.PagesActive = val
		case "Pages inactive":
			s.PagesInactive = val
		case "Pages speculative":
			s.PagesSpeculative = val
		case "Pages throttled":
			s.PagesThrottled = val
		case "Pages wired down":
			s.PagesWired = val
		case "Pages purgeable":
			s.PagesPurgeable = val
		case "Pages copy-on-write":
			s.PagesCopyOnWrite = val
		case "Pages zero filled":
			s.PagesZeroFilled = val
		case "Pages reactivated":
			s.PagesReactivated = val
		case "Pages purged":
			s.PagesPurged = val
		case "File-backed pages":
			s.FileBackedPages = val
		case "Anonymous pages":
			s.AnonymousPages = val
		case "Pages stored in compressor":
			s.PagesCompressor = val
		case "Pages occupied by compressor":
			s.CompressorOccupied = val
		case "Decompressions":
			s.Decompressions = val
		case "Compressions":
			s.Compressions = val
		case "Pageins":
			s.Pageins = val
		case "Pageouts":
			s.Pageouts = val
		case "Swapins":
			s.Swapins = val
		case "Swapouts":
			s.Swapouts = val
		}
	}

	return s, nil
}

// parseLine extracts "Key: value." from a vm_stat output line.
// Returns the key (with quotes stripped), numeric value, and success bool.
func parseLine(line string) (string, uint64, bool) {
	idx := strings.LastIndex(line, ":")
	if idx < 0 {
		return "", 0, false
	}
	key := strings.Trim(strings.TrimSpace(line[:idx]), "\"")
	valStr := strings.TrimSpace(line[idx+1:])
	valStr = strings.TrimSuffix(valStr, ".")
	val, err := strconv.ParseUint(valStr, 10, 64)
	if err != nil {
		return "", 0, false
	}
	return key, val, true
}
