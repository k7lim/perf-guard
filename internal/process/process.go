package process

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Info holds metadata about a running process.
type Info struct {
	PID  int
	Name string
	RSS  uint64  // bytes
	CPU  float64 // percent
}

// Manager handles process discovery and signaling.
type Manager struct {
	Protect []string // process name substrings to never touch
}

// NewManager creates a Manager with the given protection list.
func NewManager(protect []string) *Manager {
	return &Manager{Protect: protect}
}

// FindBySubstring returns processes whose command name contains the given substring.
func (m *Manager) FindBySubstring(substr string) ([]Info, error) {
	if err := ValidatePattern(substr); err != nil {
		return nil, err
	}
	out, err := exec.Command("ps", "-eo", "pid,rss,%cpu,comm").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	all := parsePSAll(string(out))
	var results []Info
	for _, p := range all {
		if strings.Contains(p.Name, substr) {
			results = append(results, p)
		}
	}
	return results, nil
}

// TopByRSS returns the top N processes sorted by RSS descending.
// Protected processes are excluded from results.
func (m *Manager) TopByRSS(n int) ([]Info, error) {
	if n <= 0 {
		return []Info{}, nil
	}
	out, err := exec.Command("ps", "-eo", "pid,rss,%cpu,comm").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	all := parsePSAll(string(out))

	// Filter out protected processes
	var filtered []Info
	for _, p := range all {
		protected := false
		for _, prot := range m.Protect {
			if strings.Contains(p.Name, prot) {
				protected = true
				break
			}
		}
		if !protected {
			filtered = append(filtered, p)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].RSS > filtered[j].RSS
	})

	if n > len(filtered) {
		n = len(filtered)
	}
	return filtered[:n], nil
}

// CountHelpers returns the total count of browser helper processes across all given substrings.
func (m *Manager) CountHelpers(substrings []string) (int, error) {
	out, err := exec.Command("ps", "-eo", "comm").Output()
	if err != nil {
		return 0, fmt.Errorf("ps: %w", err)
	}
	count := 0
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		lines = lines[1:] // skip ps header
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, sub := range substrings {
			if strings.Contains(line, sub) {
				count++
				break
			}
		}
	}
	return count, nil
}

// Suspend sends SIGSTOP to a process after safety checks.
func (m *Manager) Suspend(pid int, name string) error {
	if err := m.safetyCheck(pid, name); err != nil {
		return err
	}
	return unix.Kill(pid, unix.SIGSTOP)
}

// Resume sends SIGCONT to a process.
func (m *Manager) Resume(pid int) error {
	if pid <= 1 {
		return fmt.Errorf("refusing to signal PID %d", pid)
	}
	return unix.Kill(pid, unix.SIGCONT)
}

// IsRunning checks if a process exists and has the expected name.
func IsRunning(pid int, expectedName string) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return false
	}
	actual := strings.TrimSpace(string(out))
	return strings.Contains(actual, expectedName)
}

func (m *Manager) safetyCheck(pid int, name string) error {
	if pid <= 1 {
		return fmt.Errorf("refusing to signal PID %d", pid)
	}
	if pid == os.Getpid() {
		return fmt.Errorf("refusing to signal own PID")
	}
	for _, p := range m.Protect {
		if strings.Contains(name, p) {
			return fmt.Errorf("process %q is protected (matches %q)", name, p)
		}
	}
	return nil
}

// ValidatePattern rejects patterns with control characters or excessive length.
// Defends against hallucinated agent inputs per Agent DX input hardening.
func ValidatePattern(s string) error {
	if s == "" {
		return fmt.Errorf("pattern must not be empty")
	}
	if len(s) > 256 {
		return fmt.Errorf("pattern too long (%d chars, max 256)", len(s))
	}
	for i, c := range s {
		if c < 0x20 && c != '\t' {
			return fmt.Errorf("pattern contains control character at position %d (0x%02x)", i, c)
		}
	}
	return nil
}

func parsePSAll(output string) []Info {
	var results []Info
	lines := strings.Split(output, "\n")
	for _, line := range lines[1:] { // skip header
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		rss, _ := strconv.ParseUint(fields[1], 10, 64)
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		name := strings.Join(fields[3:], " ")

		results = append(results, Info{
			PID:  pid,
			Name: name,
			RSS:  rss * 1024, // ps reports RSS in KB
			CPU:  cpu,
		})
	}
	return results
}
