package forensic

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// IOStats holds disk IO statistics from ioreg.
type IOStats struct {
	ReadBytes  int64  `json:"read_bytes"`
	WriteBytes int64  `json:"write_bytes"`
	Source     string `json:"source"`
}

var (
	readBytesRe  = regexp.MustCompile(`"Bytes \(Read\)"\s*=\s*(\d+)`)
	writeBytesRe = regexp.MustCompile(`"Bytes \(Write\)"\s*=\s*(\d+)`)
)

// CaptureIOStats runs ioreg and sums read/write bytes across all disk devices.
func CaptureIOStats(fsUsage bool) (IOStats, error) {
	if fsUsage {
		return IOStats{}, fmt.Errorf("fs_usage requires elevated privileges; not implemented yet")
	}

	out, err := exec.Command("/usr/sbin/ioreg", "-c", "IOMedia", "-r").Output()
	if err != nil {
		return IOStats{Source: ""}, fmt.Errorf("ioreg: %w", err)
	}

	return parseIOreg(string(out)), nil
}

func parseIOreg(output string) IOStats {
	var readTotal, writeTotal int64

	for _, line := range strings.Split(output, "\n") {
		if m := readBytesRe.FindStringSubmatch(line); len(m) == 2 {
			v, _ := strconv.ParseInt(m[1], 10, 64)
			readTotal += v
		}
		if m := writeBytesRe.FindStringSubmatch(line); len(m) == 2 {
			v, _ := strconv.ParseInt(m[1], 10, 64)
			writeTotal += v
		}
	}

	return IOStats{
		ReadBytes:  readTotal,
		WriteBytes: writeTotal,
		Source:     "ioreg",
	}
}
