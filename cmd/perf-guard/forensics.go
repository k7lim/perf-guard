package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/k7lim/perf-guard/internal/forensic"
	"github.com/spf13/cobra"
)

// ForensicsListResult wraps the list response with pagination metadata.
type ForensicsListResult struct {
	Items  []forensic.SnapshotMeta `json:"items"`
	Total  int                     `json:"total"`
	Offset int                     `json:"offset"`
	Limit  int                     `json:"limit"`
}

// ForensicsReport holds aggregate analysis across stored snapshots.
type ForensicsReport struct {
	TotalSpikes           int              `json:"total_spikes"`
	MostFrequentProcesses []FrequencyEntry `json:"most_frequent_processes"`
	MostFrequentURLs      []FrequencyEntry `json:"most_frequent_urls"`
	SpikesByHour          [24]int          `json:"spikes_by_hour"`
	AvgCompressionAtSpike float64          `json:"avg_compression_at_spike"`
}

// FrequencyEntry holds a name and its occurrence count.
type FrequencyEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func forensicsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forensics",
		Short: "Query forensic snapshots captured during memory pressure spikes",
	}
	cmd.AddCommand(forensicsListCmd())
	cmd.AddCommand(forensicsShowCmd())
	cmd.AddCommand(forensicsReportCmd())
	return cmd
}

func forensicsListCmd() *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List forensic snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			metas, total, err := forensic.ListSnapshots(limit, offset)
			if err != nil {
				printer.PrintError("forensics list", err)
				return nil
			}

			result := ForensicsListResult{
				Items:  metas,
				Total:  total,
				Offset: offset,
				Limit:  limit,
			}

			printer.Print("forensics list", result, func() {
				fmt.Printf("TIMESTAMP                    LEVEL      COMP_RATE  PROCS  TABS\n")
				for _, m := range metas {
					fmt.Printf("%-28s %-10s %9.0f  %5d  %4d\n",
						m.Timestamp.Format(time.RFC3339), m.Level, m.CompRate, m.ProcessCount, m.TabCount)
				}
				fmt.Printf("\nShowing %d of %d snapshots (offset %d)\n", len(metas), total, offset)
			})
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum snapshots to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "Number of snapshots to skip")
	return cmd
}

func forensicsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <timestamp-prefix>",
		Short: "Show full forensic snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			snap, err := forensic.LoadSnapshot(args[0])
			if err != nil {
				printer.PrintError("forensics show", err)
				return nil
			}

			printer.Print("forensics show", snap, func() {
				fmt.Printf("Timestamp:   %s\n", snap.Timestamp.Format(time.RFC3339))
				fmt.Printf("Level:       %s\n", snap.Level)
				fmt.Printf("Compression: %.0f pages/s\n", snap.CompRate)
				fmt.Printf("Decompress:  %.0f pages/s\n", snap.DecompRate)
				fmt.Printf("Swap In:     %.0f pages/s\n", snap.SwapInRate)
				fmt.Printf("Swap Out:    %.0f pages/s\n", snap.SwapOutRate)
				fmt.Println()
				fmt.Printf("Processes (%d):\n", len(snap.Processes))
				for i, p := range snap.Processes {
					if i >= 15 {
						fmt.Printf("  ... and %d more\n", len(snap.Processes)-15)
						break
					}
					fmt.Printf("  %3d. PID %-6d %s  RSS=%.0f MB  CPU=%.1f%%\n",
						i+1, p.PID, truncName(p.Name, 40),
						float64(p.RSSBytes)/(1024*1024), p.CPUPct)
				}
				if len(snap.Tabs) > 0 {
					fmt.Printf("\nTabs (%d):\n", len(snap.Tabs))
					for i, tab := range snap.Tabs {
						if i >= 10 {
							fmt.Printf("  ... and %d more\n", len(snap.Tabs)-10)
							break
						}
						fmt.Printf("  %s  %s\n", tab.Type, tab.URL)
					}
				}
				fmt.Printf("\nIO: read=%d bytes  write=%d bytes  source=%s\n",
					snap.IO.ReadBytes, snap.IO.WriteBytes, snap.IO.Source)
			})
			return nil
		},
	}
}

func forensicsReportCmd() *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Aggregate analysis across stored snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load all snapshots, then filter by date
			metas, total, err := forensic.ListSnapshots(0, 0)
			if err != nil {
				printer.PrintError("forensics report", err)
				return nil
			}
			if total == 0 {
				printer.Print("forensics report", ForensicsReport{}, func() {
					fmt.Println("No forensic snapshots found.")
				})
				return nil
			}

			cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
			procCounts := make(map[string]int)
			urlCounts := make(map[string]int)
			var spikesByHour [24]int
			var compSum float64
			var matchCount int

			for _, m := range metas {
				if m.Timestamp.Before(cutoff) {
					continue
				}
				matchCount++
				spikesByHour[m.Timestamp.Hour()]++
				compSum += m.CompRate

				// Load full snapshot for process/tab details
				snap, err := forensic.LoadSnapshot(m.Timestamp.Format("2006-01-02T150405"))
				if err != nil {
					continue
				}
				for i, p := range snap.Processes {
					if i >= 5 {
						break
					}
					procCounts[p.Name]++
				}
				for _, tab := range snap.Tabs {
					if tab.Type == "page" && tab.URL != "" {
						urlCounts[tab.URL]++
					}
				}
			}

			var avgComp float64
			if matchCount > 0 {
				avgComp = compSum / float64(matchCount)
			}

			report := ForensicsReport{
				TotalSpikes:           matchCount,
				MostFrequentProcesses: topEntries(procCounts, 10),
				MostFrequentURLs:      topEntries(urlCounts, 10),
				SpikesByHour:          spikesByHour,
				AvgCompressionAtSpike: avgComp,
			}

			printer.Print("forensics report", report, func() {
				fmt.Printf("Forensic Report (last %d days)\n", days)
				fmt.Printf("Total spikes: %d\n", report.TotalSpikes)
				fmt.Printf("Avg compression at spike: %.0f pages/s\n\n", report.AvgCompressionAtSpike)

				if len(report.MostFrequentProcesses) > 0 {
					fmt.Println("Most frequent processes:")
					for _, e := range report.MostFrequentProcesses {
						fmt.Printf("  %3d  %s\n", e.Count, truncName(e.Name, 60))
					}
					fmt.Println()
				}
				if len(report.MostFrequentURLs) > 0 {
					fmt.Println("Most frequent URLs:")
					for _, e := range report.MostFrequentURLs {
						fmt.Printf("  %3d  %s\n", e.Count, e.Name)
					}
					fmt.Println()
				}
				fmt.Println("Spikes by hour:")
				for h, count := range report.SpikesByHour {
					if count > 0 {
						fmt.Printf("  %02d:00  %s (%d)\n", h, strings.Repeat("*", count), count)
					}
				}
			})
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "Number of days to analyze")
	return cmd
}

func topEntries(counts map[string]int, n int) []FrequencyEntry {
	var entries []FrequencyEntry
	for name, count := range counts {
		entries = append(entries, FrequencyEntry{Name: name, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})
	if n > 0 && n < len(entries) {
		entries = entries[:n]
	}
	return entries
}

func truncName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-3] + "..."
}
