package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/k7lim/perf-guard/internal/config"
	"github.com/k7lim/perf-guard/internal/daemon"
	"github.com/k7lim/perf-guard/internal/livestatus"
	"github.com/k7lim/perf-guard/internal/monitor"
	"github.com/k7lim/perf-guard/internal/output"
	"github.com/k7lim/perf-guard/internal/process"
	"github.com/k7lim/perf-guard/internal/state"
	"github.com/k7lim/perf-guard/internal/vm"
	"github.com/spf13/cobra"
)

var (
	outputFlag  string
	fieldsFlag  []string
	dryRunFlag  bool
	printer     *output.Printer
	commandName string
)

func main() {
	root := &cobra.Command{
		Use:           "perf-guard",
		Short:         "macOS memory pressure monitor and process manager",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			commandName = strings.TrimPrefix(cmd.CommandPath(), "perf-guard ")
			format := output.ResolveFormat(outputFlag)
			printer = output.NewPrinter(format, fieldsFlag)
			return nil
		},
	}

	root.PersistentFlags().StringVar(&outputFlag, "output", "", "Output format: json|text (auto-detects TTY when unset)")
	root.PersistentFlags().StringSliceVar(&fieldsFlag, "fields", nil, "Comma-separated fields to include in JSON output")
	root.PersistentFlags().BoolVar(&dryRunFlag, "dry-run", false, "Show planned actions without executing")

	root.AddCommand(statusCmd())
	root.AddCommand(focusCmd())
	root.AddCommand(restoreCmd())
	root.AddCommand(suspendCmd())
	root.AddCommand(daemonCmd())
	root.AddCommand(installCmd())
	root.AddCommand(sessionsCmd())
	root.AddCommand(forensicsCmd())
	root.AddCommand(describeCmd())

	if err := root.Execute(); err != nil {
		if printer != nil {
			printer.PrintError(commandName, err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// status
// ---------------------------------------------------------------------------

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current memory pressure and process stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			th := cfg.PressureThresholds()

			printer.Progress("Taking snapshots...")
			snap1, err := monitor.TakeSnapshot()
			if err != nil {
				return err
			}
			time.Sleep(2 * time.Second)
			snap2, err := monitor.TakeSnapshot()
			if err != nil {
				return err
			}

			reading := monitor.ComputeRates(snap1, snap2)
			mgr := process.NewManager(cfg.Protect)
			helpers, err := mgr.CountHelpers(helperSubstrings(cfg))
			if err != nil {
				return fmt.Errorf("process discovery unavailable: %w", err)
			}
			level := th.Evaluate(reading.CompressionRate, helpers)

			st, _ := state.Load()
			var suspended []output.SuspendedInfo
			for _, sp := range st.Suspended {
				suspended = append(suspended, output.SuspendedInfo{
					PID: sp.PID, Name: sp.Name, Group: sp.Group,
					SuspendedAt: sp.SuspendedAt.Format(time.RFC3339),
				})
			}
			if suspended == nil {
				suspended = []output.SuspendedInfo{}
			}

			result := output.StatusResult{
				Level:             level.String(),
				CompressionRate:   reading.CompressionRate,
				DecompressionRate: reading.DecompressionRate,
				SwapInRate:        reading.SwapInRate,
				SwapOutRate:       reading.SwapOutRate,
				PagesInCompressor: snap2.PagesCompressor,
				CompressorGB:      float64(snap2.CompressorOccupied*snap2.PageSize) / (1024 * 1024 * 1024),
				TotalCompressions: snap2.Compressions,
				TotalSwapins:      snap2.Swapins,
				TotalSwapouts:     snap2.Swapouts,
				BrowserHelpers:    helpers,
				Thresholds: output.ThresholdsInfo{
					Warning: th.WarningRate, High: th.HighRate,
					Critical: th.CriticalRate, MaxHelpers: th.MaxHelpers,
				},
				Suspended: suspended,
				VMStopped: st.VMStopped,
			}
			if live, err := livestatus.Read(); err == nil {
				if updated, err := time.Parse(time.RFC3339, live.UpdatedAt); err == nil {
					age := time.Since(updated).Seconds()
					result.Daemon = &output.DaemonInfo{MonitorOnly: live.MonitorOnly, UpdatedAt: live.UpdatedAt, HeartbeatAgeSeconds: age, Stale: age > 90, DiscoveryError: live.DiscoveryError}
				}
			}

			printer.Print("status", result, func() {
				fmt.Println()
				fmt.Printf("Pressure Level:    %s\n", strings.ToUpper(level.String()))
				fmt.Println()
				fmt.Printf("Compression rate:  %.0f pages/s\n", reading.CompressionRate)
				fmt.Printf("Decompress rate:   %.0f pages/s\n", reading.DecompressionRate)
				fmt.Printf("Swap in rate:      %.0f pages/s\n", reading.SwapInRate)
				fmt.Printf("Swap out rate:     %.0f pages/s\n", reading.SwapOutRate)
				fmt.Println()
				fmt.Printf("Pages stored in compressor: %d (occupies %.1f GiB RAM)\n",
					snap2.PagesCompressor, result.CompressorGB)
				fmt.Printf("Total compressions:  %d\n", snap2.Compressions)
				fmt.Printf("Total swapins:       %d\n", snap2.Swapins)
				fmt.Printf("Total swapouts:      %d\n", snap2.Swapouts)
				fmt.Println()
				fmt.Printf("Browser helpers:     %d\n", helpers)
				fmt.Println()
				fmt.Printf("Thresholds: warning=%.0f  high=%.0f  critical=%.0f pages/s  max_helpers=%d\n",
					th.WarningRate, th.HighRate, th.CriticalRate, th.MaxHelpers)
				if len(st.Suspended) > 0 {
					fmt.Println()
					fmt.Printf("Suspended processes: %d\n", len(st.Suspended))
					for _, sp := range st.Suspended {
						fmt.Printf("  PID %d  %s  (%s)  since %s\n",
							sp.PID, sp.Name, sp.Group, sp.SuspendedAt.Format("15:04:05"))
					}
				}
				if st.VMStopped {
					fmt.Println("Lima VM: stopped by perf-guard")
				}
			})
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// focus
// ---------------------------------------------------------------------------

func focusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "focus",
		Short: "Suspend all non-essential processes (one-click focus mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			mgr := process.NewManager(cfg.Protect)
			st, _ := state.Load()

			groups := sortedGroups(cfg)
			var actions []output.ActionResult

			for _, g := range groups {
				if g.gc.Action == "vm-stop" {
					vc := vm.NewController()
					if vc.HasRunning() {
						if dryRunFlag {
							actions = append(actions, output.ActionResult{
								Action: "vm-stop", Group: g.name, Status: "planned",
							})
						} else {
							printer.Progress("Stopping Lima VM...")
							if err := vc.Stop(); err != nil {
								actions = append(actions, output.ActionResult{
									Action: "vm-stop", Group: g.name, Status: "error", Error: err.Error(),
								})
							} else {
								st.VMStopped = true
								actions = append(actions, output.ActionResult{
									Action: "vm-stop", Group: g.name, Status: "ok",
								})
								printer.Progress("  Lima VM stopped")
							}
						}
					}
					continue
				}

				procs, err := mgr.FindBySubstring(g.gc.Match)
				if err != nil {
					log.Printf("warning: finding %s: %v", g.name, err)
					continue
				}
				if len(procs) == 0 {
					continue
				}

				printer.Progress(fmt.Sprintf("Suspending %d %s processes...", len(procs), g.name))
				for _, p := range procs {
					if dryRunFlag {
						actions = append(actions, output.ActionResult{
							Action: "suspend", Group: g.name, PID: p.PID, Name: p.Name, Status: "planned",
						})
					} else {
						if err := mgr.Suspend(p.PID, p.Name); err != nil {
							actions = append(actions, output.ActionResult{
								Action: "suspend", Group: g.name, PID: p.PID, Name: p.Name,
								Status: "error", Error: err.Error(),
							})
						} else {
							st.AddSuspended(p.PID, p.Name, g.name)
							actions = append(actions, output.ActionResult{
								Action: "suspend", Group: g.name, PID: p.PID, Name: p.Name, Status: "ok",
							})
						}
						time.Sleep(10 * time.Millisecond)
					}
				}
				printer.Progress(fmt.Sprintf("  %d processed", len(procs)))
			}

			if !dryRunFlag {
				if err := st.Save(); err != nil {
					return fmt.Errorf("saving state: %w", err)
				}
			}

			result := output.MutationResult{
				DryRun:  dryRunFlag,
				Actions: actions,
				Summary: summarizeActions(actions, dryRunFlag),
			}

			printer.Print("focus", result, func() {
				if dryRunFlag {
					fmt.Printf("[dry-run] %s\n", result.Summary)
				} else {
					fmt.Println("Focus mode active. Run 'perf-guard restore' to resume.")
				}
			})
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// restore
// ---------------------------------------------------------------------------

func restoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore",
		Short: "Resume all processes suspended by perf-guard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			mgr := process.NewManager(cfg.Protect)
			st, _ := state.Load()

			var actions []output.ActionResult

			for _, sp := range st.Suspended {
				if !process.IsRunning(sp.PID, sp.Name) {
					actions = append(actions, output.ActionResult{
						Action: "resume", Group: sp.Group, PID: sp.PID, Name: sp.Name, Status: "skipped",
						Error: "process no longer running or name changed",
					})
					continue
				}
				if dryRunFlag {
					actions = append(actions, output.ActionResult{
						Action: "resume", Group: sp.Group, PID: sp.PID, Name: sp.Name, Status: "planned",
					})
				} else {
					if err := mgr.Resume(sp.PID); err != nil {
						actions = append(actions, output.ActionResult{
							Action: "resume", Group: sp.Group, PID: sp.PID, Name: sp.Name,
							Status: "error", Error: err.Error(),
						})
					} else {
						actions = append(actions, output.ActionResult{
							Action: "resume", Group: sp.Group, PID: sp.PID, Name: sp.Name, Status: "ok",
						})
					}
				}
			}

			if st.VMStopped {
				if dryRunFlag {
					actions = append(actions, output.ActionResult{
						Action: "vm-start", Group: "lima", Status: "planned",
					})
				} else {
					printer.Progress("Starting Lima VM...")
					vc := vm.NewController()
					if err := vc.Start(); err != nil {
						actions = append(actions, output.ActionResult{
							Action: "vm-start", Group: "lima", Status: "error", Error: err.Error(),
						})
					} else {
						actions = append(actions, output.ActionResult{
							Action: "vm-start", Group: "lima", Status: "ok",
						})
						printer.Progress("  Lima VM started")
					}
				}
			}

			if !dryRunFlag {
				st.Clear()
				if err := st.Save(); err != nil {
					return fmt.Errorf("saving state: %w", err)
				}
			}

			result := output.MutationResult{
				DryRun:  dryRunFlag,
				Actions: actions,
				Summary: summarizeActions(actions, dryRunFlag),
			}

			printer.Print("restore", result, func() {
				if dryRunFlag {
					fmt.Printf("[dry-run] %s\n", result.Summary)
				} else {
					fmt.Printf("%s\n", result.Summary)
				}
			})
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// suspend (vm | browsers)
// ---------------------------------------------------------------------------

func suspendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suspend",
		Short: "Suspend a specific group (vm, browsers)",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "vm",
		Short: "Stop Lima VM",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _ := state.Load()
			vc := vm.NewController()
			var actions []output.ActionResult

			if !vc.HasRunning() {
				actions = append(actions, output.ActionResult{
					Action: "vm-stop", Group: "lima", Status: "skipped", Error: "no running VMs",
				})
			} else if dryRunFlag {
				actions = append(actions, output.ActionResult{
					Action: "vm-stop", Group: "lima", Status: "planned",
				})
			} else {
				printer.Progress("Stopping Lima VM...")
				if err := vc.Stop(); err != nil {
					actions = append(actions, output.ActionResult{
						Action: "vm-stop", Group: "lima", Status: "error", Error: err.Error(),
					})
				} else {
					st.VMStopped = true
					st.Save()
					actions = append(actions, output.ActionResult{
						Action: "vm-stop", Group: "lima", Status: "ok",
					})
					printer.Progress("Lima VM stopped")
				}
			}

			result := output.MutationResult{
				DryRun: dryRunFlag, Actions: actions,
				Summary: summarizeActions(actions, dryRunFlag),
			}
			printer.Print("suspend vm", result, func() {
				if dryRunFlag {
					fmt.Printf("[dry-run] %s\n", result.Summary)
				} else {
					fmt.Println(result.Summary)
				}
			})
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "browsers",
		Short: "SIGSTOP all browser helper processes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			mgr := process.NewManager(cfg.Protect)
			st, _ := state.Load()

			var actions []output.ActionResult
			browserGroups := []string{"brave", "firefox"}
			for _, gname := range browserGroups {
				gc, ok := cfg.Groups[gname]
				if !ok {
					continue
				}
				procs, err := mgr.FindBySubstring(gc.Match)
				if err != nil {
					log.Printf("warning: %v", err)
					continue
				}
				if len(procs) == 0 {
					continue
				}
				printer.Progress(fmt.Sprintf("Suspending %d %s helpers...", len(procs), gname))
				for _, p := range procs {
					if dryRunFlag {
						actions = append(actions, output.ActionResult{
							Action: "suspend", Group: gname, PID: p.PID, Name: p.Name, Status: "planned",
						})
					} else {
						if err := mgr.Suspend(p.PID, p.Name); err != nil {
							actions = append(actions, output.ActionResult{
								Action: "suspend", Group: gname, PID: p.PID, Name: p.Name,
								Status: "error", Error: err.Error(),
							})
						} else {
							st.AddSuspended(p.PID, p.Name, gname)
							actions = append(actions, output.ActionResult{
								Action: "suspend", Group: gname, PID: p.PID, Name: p.Name, Status: "ok",
							})
						}
						time.Sleep(10 * time.Millisecond)
					}
				}
			}

			if !dryRunFlag {
				st.Save()
			}

			result := output.MutationResult{
				DryRun: dryRunFlag, Actions: actions,
				Summary: summarizeActions(actions, dryRunFlag),
			}
			printer.Print("suspend browsers", result, func() {
				if dryRunFlag {
					fmt.Printf("[dry-run] %s\n", result.Summary)
				} else {
					fmt.Println("Browser helpers suspended. Run 'perf-guard restore' to resume.")
				}
			})
			return nil
		},
	})

	return cmd
}

// ---------------------------------------------------------------------------
// daemon
// ---------------------------------------------------------------------------

func daemonCmd() *cobra.Command {
	var monitorOnly bool
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a background monitor daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("monitor-only") {
				cfg.MonitorOnly = monitorOnly
			}
			if dryRunFlag {
				printer.Print("daemon", map[string]any{"dry_run": true, "monitor_only": cfg.MonitorOnly, "log_path": cfg.LogPath}, func() {
					fmt.Printf("[dry-run] Start daemon (monitor_only=%t), log: %s\n", cfg.MonitorOnly, cfg.LogPath)
				})
				return nil
			}
			return daemon.New(cfg).Run()
		},
	}
	cmd.Flags().BoolVar(&monitorOnly, "monitor-only", false, "Monitor, notify and capture diagnostics without managing processes or VMs")
	return cmd
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type namedGroup struct {
	name string
	gc   config.GroupConfig
}

func sortedGroups(cfg config.Config) []namedGroup {
	var groups []namedGroup
	for name, gc := range cfg.Groups {
		groups = append(groups, namedGroup{name, gc})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].gc.Priority < groups[j].gc.Priority
	})
	return groups
}

func helperSubstrings(cfg config.Config) []string {
	var subs []string
	for _, gc := range cfg.Groups {
		if gc.Action == "suspend" {
			subs = append(subs, gc.Match)
		}
	}
	return subs
}

func summarizeActions(actions []output.ActionResult, dryRun bool) string {
	planned, ok, skipped, errored := 0, 0, 0, 0
	for _, a := range actions {
		switch a.Status {
		case "planned":
			planned++
		case "ok":
			ok++
		case "skipped":
			skipped++
		case "error":
			errored++
		}
	}
	if dryRun {
		return fmt.Sprintf("Would perform %d actions", planned)
	}
	parts := []string{fmt.Sprintf("%d succeeded", ok)}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if errored > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", errored))
	}
	return strings.Join(parts, ", ")
}
