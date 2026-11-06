package main

import (
	"encoding/json"
	"os"

	"github.com/k7lim/perf-guard/internal/output"
	"github.com/spf13/cobra"
)

type schema struct {
	Name        string                   `json:"name"`
	Version     string                   `json:"version"`
	Description string                   `json:"description"`
	Commands    map[string]commandSchema `json:"commands"`
	EnvVars     map[string]string        `json:"env_vars"`
	ConfigPath  string                   `json:"config_path"`
	StatePath   string                   `json:"state_path"`
}

type commandSchema struct {
	Description     string                   `json:"description"`
	Flags           map[string]flagInfo      `json:"flags,omitempty"`
	OutputSchema    map[string]string        `json:"output_schema,omitempty"`
	AvailableFields []string                 `json:"available_fields,omitempty"`
	Mutates         bool                     `json:"mutates,omitempty"`
	Reversible      bool                     `json:"reversible,omitempty"`
	ReverseCommand  string                   `json:"reverse_command,omitempty"`
	Subcommands     map[string]commandSchema `json:"subcommands,omitempty"`
	Notes           []string                 `json:"notes,omitempty"`
}

type flagInfo struct {
	Type        string   `json:"type"`
	Values      []string `json:"values,omitempty"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description"`
}

func describeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "describe",
		Short: "Show command metadata and output schemas (always JSON)",
		Run: func(cmd *cobra.Command, args []string) {
			s := schema{
				Name:        "perf-guard",
				Version:     "0.2.0",
				Description: "macOS memory pressure monitor and process manager",
				EnvVars: map[string]string{
					"OUTPUT_FORMAT":             "json|text — override output format for all commands",
					"PERF_GUARD_HERDR_ENDPOINT": "Explicit inherited absolute local Unix endpoint for review",
					"PERF_GUARD_HERDR_SESSION":  "Explicit inherited session scope for review",
				},
				ConfigPath: "~/.config/perf-guard/config.toml",
				StatePath:  "~/.config/perf-guard/state.json",
				Commands: map[string]commandSchema{
					"review": reviewCommandSchema(),
					"status": {
						Description: "Show current memory pressure and process stats",
						Flags: map[string]flagInfo{
							"--output": {Type: "string", Values: []string{"json", "text"}, Default: "auto (TTY=text, pipe=json)", Description: "Output format"},
							"--fields": {Type: "string[]", Description: "Comma-separated fields to include in JSON output"},
						},
						AvailableFields: output.StatusFields,
						OutputSchema: map[string]string{
							"daemon":              "{monitor_only, updated_at, heartbeat_age_seconds, stale, discovery_error?} or null when no heartbeat is available",
							"level":               "string (normal|warning|high|critical)",
							"compression_rate":    "float64 (pages/sec)",
							"decompression_rate":  "float64 (pages/sec)",
							"swap_in_rate":        "float64 (pages/sec)",
							"swap_out_rate":       "float64 (pages/sec)",
							"pages_in_compressor": "uint64",
							"compressor_gb":       "float64 (physical GiB occupied by compressor)",
							"total_compressions":  "uint64",
							"total_swapins":       "uint64",
							"total_swapouts":      "uint64",
							"browser_helpers":     "int",
							"thresholds":          "{warning, high, critical, max_helpers}",
							"suspended":           "[{pid, name, group, suspended_at}]",
							"vm_stopped":          "bool",
						},
						Notes: []string{"Takes ~2s (two vm_stat snapshots required)"},
					},
					"focus": {
						Description: "Suspend all non-essential processes (one-click focus mode)",
						Flags: map[string]flagInfo{
							"--output":  {Type: "string", Values: []string{"json", "text"}, Description: "Output format"},
							"--dry-run": {Type: "bool", Default: "false", Description: "Show planned actions without executing"},
						},
						OutputSchema: map[string]string{
							"dry_run": "bool",
							"actions": "[{action, group, pid, name, status, error}]",
							"summary": "string",
						},
						Mutates:        true,
						Reversible:     true,
						ReverseCommand: "restore",
						Notes:          []string{"Always use --dry-run first to preview actions"},
					},
					"restore": {
						Description: "Resume all processes suspended by perf-guard",
						Flags: map[string]flagInfo{
							"--output":  {Type: "string", Values: []string{"json", "text"}, Description: "Output format"},
							"--dry-run": {Type: "bool", Default: "false", Description: "Show planned actions without executing"},
						},
						OutputSchema: map[string]string{
							"dry_run": "bool",
							"actions": "[{action, group, pid, name, status, error}]",
							"summary": "string",
						},
						Mutates: true,
						Notes:   []string{"Skips stale PIDs (process exited or name changed)"},
					},
					"suspend": {
						Description: "Suspend a specific group",
						Subcommands: map[string]commandSchema{
							"vm": {
								Description:    "Stop Lima VM via limactl",
								Mutates:        true,
								Reversible:     true,
								ReverseCommand: "restore",
							},
							"browsers": {
								Description:    "SIGSTOP all browser helper processes",
								Mutates:        true,
								Reversible:     true,
								ReverseCommand: "restore",
							},
						},
						Flags: map[string]flagInfo{
							"--output":  {Type: "string", Values: []string{"json", "text"}, Description: "Output format"},
							"--dry-run": {Type: "bool", Default: "false", Description: "Show planned actions without executing"},
						},
					},
					"forensics": {
						Description: "Forensic snapshot capture and query",
						Subcommands: map[string]commandSchema{
							"capture": {
								Description: "Capture a forensic snapshot right now (manual trigger)",
								Mutates:     true,
								Notes:       []string{"Takes ~2s (two vm_stat samples). Run when you see a freeze."},
							},
							"list": {
								Description: "List forensic snapshots with pagination",
								Flags: map[string]flagInfo{
									"--limit":  {Type: "int", Default: "20", Description: "Maximum snapshots to return"},
									"--offset": {Type: "int", Default: "0", Description: "Number of snapshots to skip"},
								},
								OutputSchema: map[string]string{
									"items":  "[{timestamp, level, compression_rate, process_count, tab_count}]",
									"total":  "int",
									"offset": "int",
									"limit":  "int",
								},
							},
							"show": {
								Description: "Show full forensic snapshot by timestamp prefix",
								OutputSchema: map[string]string{
									"schema_version":     "int",
									"timestamp":          "string (RFC3339)",
									"level":              "string",
									"compression_rate":   "float64",
									"decompression_rate": "float64",
									"swap_in_rate":       "float64",
									"swap_out_rate":      "float64",
									"processes":          "[{pid, name, rss_bytes, vsz_bytes, cpu_pct, mem_pct}]",
									"tabs":               "[{url, title, type}]",
									"io":                 "{read_bytes, write_bytes, source}",
									"vmstat":             "object",
								},
								Notes: []string{"Accepts timestamp prefix for fuzzy matching (e.g. '2026-04-04T23')"},
							},
							"report": {
								Description: "Aggregate analysis across stored snapshots",
								Flags: map[string]flagInfo{
									"--days": {Type: "int", Default: "7", Description: "Number of days to analyze"},
								},
								OutputSchema: map[string]string{
									"total_spikes":             "int",
									"most_frequent_processes":  "[{name, count}]",
									"most_frequent_urls":       "[{name, count}]",
									"spikes_by_hour":           "[24]int",
									"avg_compression_at_spike": "float64",
								},
							},
						},
					},
					"describe": {
						Description: "Show command metadata and output schemas (always JSON)",
					},
					"daemon": {
						Description: "Background monitoring, notifications and optional pressure actions",
						Flags: map[string]flagInfo{
							"--monitor-only": {Type: "bool", Description: "Disable automatic process and VM actions; keep monitoring, notifications and forensic capture"},
							"--dry-run":      {Type: "bool", Description: "Preview daemon mode and log path without starting"},
						},
						Notes: []string{"Use install for persistent operation. Writes to log file, not stdout."},
					},
					"sessions": {
						Description: "Read-only inventory separating terminal agent sessions from desktop helpers and app servers",
						Flags: map[string]flagInfo{
							"--limit":  {Type: "int", Default: "20", Description: "Maximum entries"},
							"--offset": {Type: "int", Default: "0", Description: "Inventory offset"},
						},
						OutputSchema: map[string]string{
							"items":          "[{pid, parent_pid, agent, kind, tty, age_seconds, rss_bytes}]",
							"counts_by_kind": "{terminal?, terminal_helper?, desktop_helper?, app_server?, background?}",
							"total":          "int", "limit": "int", "offset": "int", "rss_bytes": "uint64 (resident memory only)",
						},
						Notes: []string{"Age is not inactivity. No termination or reaper is performed."},
					},
					"install": {
						Description: "Install as LaunchAgent (starts on login)",
						Flags: map[string]flagInfo{
							"--dry-run":      {Type: "bool", Default: "false", Description: "Show install paths without writing"},
							"--monitor-only": {Type: "bool", Description: "Persist monitor-only operation across login and restarts"},
						},
						Mutates: true,
					},
				},
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(s)
		},
	}
}
