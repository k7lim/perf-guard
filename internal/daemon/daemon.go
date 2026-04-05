package daemon

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/k7lim/perf-guard/internal/config"
	"github.com/k7lim/perf-guard/internal/forensic"
	"github.com/k7lim/perf-guard/internal/livestatus"
	"github.com/k7lim/perf-guard/internal/monitor"
	"github.com/k7lim/perf-guard/internal/pressure"
	"github.com/k7lim/perf-guard/internal/process"
	"github.com/k7lim/perf-guard/internal/state"
	"github.com/k7lim/perf-guard/internal/vm"
	"golang.org/x/sys/unix"
)

const hysteresisCount = 3 // consecutive readings below threshold to downgrade

// Daemon is the main monitor loop.
type Daemon struct {
	cfg           config.Config
	mgr           *process.Manager
	thresholds    pressure.Thresholds
	currentLevel  pressure.Level
	belowCount    int // consecutive readings below current level
	lastSnapshot  *monitor.VmStatSnapshot
	state         state.State
	logger        *log.Logger
	spikeDetector *forensic.SpikeDetector
	lastSpikeMu   sync.Mutex
	lastSpikeTime time.Time
}

// New creates a new Daemon with the given config.
func New(cfg config.Config) *Daemon {
	d := &Daemon{
		cfg:        cfg,
		mgr:        process.NewManager(cfg.Protect),
		thresholds: cfg.PressureThresholds(),
		logger:     log.New(os.Stderr, "perf-guard: ", log.LstdFlags),
	}
	if cfg.Forensic.Enabled {
		d.spikeDetector = forensic.NewSpikeDetector(
			cfg.Forensic.WindowSize,
			time.Duration(cfg.Forensic.Cooldown)*time.Second,
		)
	}
	return d
}

// Run starts the daemon loop. Blocks until interrupted.
func (d *Daemon) Run() error {
	// Best-effort mlockall
	if err := unix.Mlockall(unix.MCL_CURRENT | unix.MCL_FUTURE); err != nil {
		d.logger.Printf("warning: mlockall failed (need elevated privileges?): %v", err)
	}

	// Load any existing state (in case we crashed with suspended processes)
	st, _ := state.Load()
	d.state = st

	// Set up log file
	if d.cfg.LogPath != "" {
		f, err := os.OpenFile(d.cfg.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			d.logger.Printf("warning: cannot open log file %s: %v", d.cfg.LogPath, err)
		} else {
			defer f.Close()
			d.logger = log.New(f, "perf-guard: ", log.LstdFlags)
		}
	}

	d.logger.Println("daemon starting")

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Take initial snapshot
	snap, err := monitor.TakeSnapshot()
	if err != nil {
		return fmt.Errorf("initial snapshot: %w", err)
	}
	d.lastSnapshot = &snap

	for {
		interval := d.pollInterval()

		select {
		case <-sigCh:
			d.logger.Println("shutting down")
			return nil
		case <-time.After(interval):
		}

		snap, err := monitor.TakeSnapshot()
		if err != nil {
			d.logger.Printf("snapshot error: %v", err)
			continue
		}

		reading := monitor.ComputeRates(*d.lastSnapshot, snap)
		d.lastSnapshot = &snap

		if d.spikeDetector != nil && d.spikeDetector.Record(reading.CompressionRate) {
			go d.captureForensic(reading)
		}

		helperSubs := d.helperSubstrings()
		helpers, _ := d.mgr.CountHelpers(helperSubs)

		newLevel := d.thresholds.Evaluate(reading.CompressionRate, helpers)

		// Apply hysteresis: only downgrade after consecutive readings below current level
		effectiveLevel := d.applyHysteresis(newLevel)

		if effectiveLevel != d.currentLevel {
			d.logger.Printf("level change: %s → %s (rate=%.0f pages/s, helpers=%d)",
				d.currentLevel, effectiveLevel, reading.CompressionRate, helpers)
			d.act(effectiveLevel, d.currentLevel)
			d.currentLevel = effectiveLevel
		}

		d.writeLiveStatus(reading, helpers)
	}
}

func (d *Daemon) applyHysteresis(newLevel pressure.Level) pressure.Level {
	if newLevel >= d.currentLevel {
		// Escalation is immediate
		d.belowCount = 0
		return newLevel
	}
	// Downgrade requires consecutive readings
	d.belowCount++
	if d.belowCount >= hysteresisCount {
		d.belowCount = 0
		return newLevel
	}
	return d.currentLevel // hold current level
}

func (d *Daemon) act(newLevel, oldLevel pressure.Level) {
	switch {
	case newLevel >= pressure.Warning && oldLevel < pressure.Warning:
		d.logger.Println("action: pressure rising to warning level")

	case newLevel >= pressure.High && oldLevel < pressure.High:
		d.logger.Println("action: stopping Lima VM")
		vc := vm.NewController()
		if err := vc.Stop(); err != nil {
			d.logger.Printf("warning: stopping VM: %v", err)
		} else {
			d.state.VMStopped = true
			d.state.Save()
		}

	case newLevel >= pressure.Critical && oldLevel < pressure.Critical:
		d.logger.Println("action: suspending helper processes")
		d.suspendHelpers()

	case newLevel < pressure.Critical && oldLevel >= pressure.Critical:
		d.logger.Println("action: resuming helper processes")
		d.resumeHelpers()

	case newLevel < pressure.High && oldLevel >= pressure.High:
		// VM stays stopped until manual restore — too expensive to auto-restart
		d.logger.Println("pressure below high — VM remains stopped (use 'perf-guard restore' to restart)")
	}
}

func (d *Daemon) suspendHelpers() {
	type namedGroup struct {
		name string
		gc   config.GroupConfig
	}
	var groups []namedGroup
	for name, gc := range d.cfg.Groups {
		if gc.Action == "suspend" {
			groups = append(groups, namedGroup{name, gc})
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].gc.Priority < groups[j].gc.Priority
	})

	for _, g := range groups {
		procs, err := d.mgr.FindBySubstring(g.gc.Match)
		if err != nil {
			d.logger.Printf("warning: finding %s: %v", g.name, err)
			continue
		}
		for _, p := range procs {
			if err := d.mgr.Suspend(p.PID, p.Name); err != nil {
				d.logger.Printf("warning: suspend PID %d: %v", p.PID, err)
				continue
			}
			d.state.AddSuspended(p.PID, p.Name, g.name)
			time.Sleep(10 * time.Millisecond) // per-PID, not process group
		}
		d.logger.Printf("suspended %d %s processes", len(procs), g.name)
	}
	d.state.Save()
}

func (d *Daemon) resumeHelpers() {
	resumed := 0
	for _, sp := range d.state.Suspended {
		if !process.IsRunning(sp.PID, sp.Name) {
			continue
		}
		if err := d.mgr.Resume(sp.PID); err != nil {
			d.logger.Printf("warning: resume PID %d: %v", sp.PID, err)
			continue
		}
		resumed++
	}
	d.state.Suspended = nil
	d.state.Save()
	d.logger.Printf("resumed %d processes", resumed)
}

func (d *Daemon) pollInterval() time.Duration {
	switch d.currentLevel {
	case pressure.Critical:
		return time.Duration(d.cfg.Polling.Critical) * time.Second
	case pressure.High:
		return time.Duration(d.cfg.Polling.Critical) * time.Second // fast polling when high too
	case pressure.Warning:
		return time.Duration(d.cfg.Polling.Warning) * time.Second
	default:
		return time.Duration(d.cfg.Polling.Normal) * time.Second
	}
}

func (d *Daemon) helperSubstrings() []string {
	var subs []string
	for _, gc := range d.cfg.Groups {
		if gc.Action == "suspend" {
			subs = append(subs, gc.Match)
		}
	}
	return subs
}

func (d *Daemon) captureForensic(reading monitor.PressureReading) {
	snap, err := forensic.CaptureSnapshot(
		d.currentLevel.String(),
		reading,
		d.cfg.Forensic.CDPPort,
		d.cfg.Forensic.FSUsage,
	)
	if err != nil {
		d.logger.Printf("forensic capture failed: %v", err)
		return
	}
	if err := forensic.SaveSnapshot(snap); err != nil {
		d.logger.Printf("forensic save failed: %v", err)
		return
	}
	if err := forensic.RotateSnapshots(d.cfg.Forensic.MaxKeep); err != nil {
		d.logger.Printf("forensic rotation failed: %v", err)
	}

	d.lastSpikeMu.Lock()
	d.lastSpikeTime = snap.Timestamp
	d.lastSpikeMu.Unlock()

	d.logger.Printf("forensic snapshot captured: %s", snap.Timestamp.Format(time.RFC3339))
}

func (d *Daemon) writeLiveStatus(reading monitor.PressureReading, helperCount int) {
	topProcs, _ := d.mgr.TopByRSS(5)

	var top []livestatus.TopProcess
	for _, p := range topProcs {
		top = append(top, livestatus.TopProcess{PID: p.PID, Name: p.Name, RSSBytes: p.RSS, CPUPct: p.CPU})
	}

	var susp []livestatus.SuspendedBrief
	for _, s := range d.state.Suspended {
		susp = append(susp, livestatus.SuspendedBrief{PID: s.PID, Name: s.Name})
	}

	status := &livestatus.LiveStatus{
		Level:           d.currentLevel.String(),
		CompressionRate: reading.CompressionRate,
		HelperCount:     helperCount,
		TopProcesses:    top,
		Suspended:       susp,
		VMStopped:       d.state.VMStopped,
	}

	d.lastSpikeMu.Lock()
	spikeTime := d.lastSpikeTime
	d.lastSpikeMu.Unlock()
	if !spikeTime.IsZero() {
		status.LastSpikeTime = spikeTime.Format(time.RFC3339)
	}

	home, _ := os.UserHomeDir()
	forensicsDir := filepath.Join(home, ".config", "perf-guard", "forensics")
	if entries, err := os.ReadDir(forensicsDir); err == nil {
		status.ForensicCount = len(entries)
	}

	if err := livestatus.Write(status); err != nil {
		d.logger.Printf("write live status: %v", err)
	}
}
