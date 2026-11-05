package main

import (
	"fmt"
	"github.com/k7lim/perf-guard/internal/process"
	"github.com/spf13/cobra"
)

func sessionsCmd() *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{Use: "sessions", Short: "Inventory terminal agents separately from app servers and desktop helpers", RunE: func(cmd *cobra.Command, args []string) error {
		if limit < 1 || offset < 0 {
			return fmt.Errorf("limit must be positive and offset nonnegative")
		}
		entries, err := process.AgentProcesses()
		if err != nil {
			return err
		}
		counts := map[string]int{}
		var rss uint64
		for _, p := range entries {
			counts[p.Kind]++
			rss += p.RSSBytes
		}
		start := min(offset, len(entries))
		end := start + min(limit, len(entries)-start)
		printer.Print("sessions", map[string]any{"items": entries[start:end], "total": len(entries), "limit": limit, "offset": offset, "counts_by_kind": counts, "rss_bytes": rss, "note": "Age is not inactivity. RSS excludes compressed/swapped memory. No processes are terminated."}, func() {
			fmt.Printf("Agent processes: %d; terminal sessions: %d; resident memory: %.1f MiB\n", len(entries), counts["terminal"], float64(rss)/(1024*1024))
			for _, p := range entries[start:end] {
				fmt.Printf("PID %d  %s  %s  age %.1f days  %.0f MiB resident\n", p.PID, p.Agent, p.Kind, float64(p.AgeSeconds)/86400, float64(p.RSSBytes)/(1024*1024))
			}
		})
		return nil
	}}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum inventory entries")
	cmd.Flags().IntVar(&offset, "offset", 0, "Inventory offset")
	return cmd
}
