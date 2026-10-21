package forensic

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// ProcessSnapshot holds extended process info for forensic capture.
type ProcessSnapshot struct {
	PID      int     `json:"pid"`
	Name     string  `json:"name"`
	RSSBytes uint64  `json:"rss_bytes"`
	VSZBytes uint64  `json:"vsz_bytes"`
	CPUPct   float64 `json:"cpu_pct"`
	MEMPct   float64 `json:"mem_pct"`
}

// CaptureProcesses runs ps and returns the top N processes sorted by RSS descending.
func CaptureProcesses(n int) ([]ProcessSnapshot, error) {
	out, err := exec.Command("ps", "-eo", "pid,rss,vsz,%cpu,%mem,comm").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	procs, parseErrs, totalLines := parseProcessSnapshot(string(out), n)
	if totalLines > 0 && parseErrs > totalLines/2 {
		return procs, fmt.Errorf("ps: %d/%d lines failed to parse", parseErrs, totalLines)
	}
	return procs, nil
}

func parseProcessSnapshot(output string, n int) ([]ProcessSnapshot, int, int) {
	var results []ProcessSnapshot
	var parseErrors int
	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		return results, 0, 0
	}
	dataLines := 0
	for _, line := range lines[1:] { // skip header
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dataLines++
		fields := strings.Fields(line)
		if len(fields) < 6 {
			parseErrors++
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid == 0 {
			parseErrors++
			continue
		}
		rss, _ := strconv.ParseUint(fields[1], 10, 64)
		vsz, _ := strconv.ParseUint(fields[2], 10, 64)
		cpu, _ := strconv.ParseFloat(fields[3], 64)
		mem, _ := strconv.ParseFloat(fields[4], 64)
		name := strings.Join(fields[5:], " ")

		results = append(results, ProcessSnapshot{
			PID:      pid,
			Name:     name,
			RSSBytes: rss * 1024,
			VSZBytes: vsz * 1024,
			CPUPct:   cpu,
			MEMPct:   mem,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].RSSBytes > results[j].RSSBytes
	})

	if n > 0 && n < len(results) {
		results = results[:n]
	}
	return results, parseErrors, dataLines
}
