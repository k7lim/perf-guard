package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/k7lim/perf-guard/internal/herdr"
	"github.com/k7lim/perf-guard/internal/process"
	"github.com/k7lim/perf-guard/internal/review"
	"github.com/spf13/cobra"
)

// newReviewService is the sole production wiring. It accepts only read seams;
// neither Manager nor Herdr control adapters are part of this path.
var newReviewService = func() *review.Service {
	return &review.Service{Processes: process.NewReviewSource(), Herdr: herdr.NewSource(process.ReadIdentity), Worktrees: review.GitWorktreeSource{}, ControllerPID: os.Getpid()}
}

func reviewCmd() *cobra.Command {
	var endpoint, session string
	var limit, offset int
	var ids []string
	target := func() (review.HerdrTarget, error) {
		t := review.HerdrTarget{Endpoint: endpoint, SessionID: session}
		if endpoint != "" || session != "" {
			resolved, err := herdr.ResolveTarget(t, nil)
			if err != nil {
				return t, fmt.Errorf("invalid Herdr target: supply an absolute Unix endpoint and session together")
			}
			return resolved, nil
		}
		return t, nil
	}
	runList := func(cmd *cobra.Command, args []string) error {
		if outputFlag != "" && outputFlag != "json" && outputFlag != "text" {
			return fmt.Errorf("output must be json or text")
		}
		if err := review.ValidatePagination(limit, offset); err != nil {
			return err
		}
		t, err := target()
		if err != nil {
			return err
		}
		r, err := newReviewService().List(cmd.Context(), review.ListOptions{Target: t, Limit: limit, Offset: offset})
		if err != nil {
			return err
		}
		printer.Print("review", r, func() {
			fmt.Printf("Sessions: %d; reviewable: %d; target: %s (%s)\n", r.Total, r.ReviewableTotal, r.Target.Endpoint, r.Target.SessionID)
			for _, v := range r.Records {
				fmt.Printf("%s  %s  %s  %s\n", v.ID, v.Agent, v.Lifecycle.State, strings.Join(v.Reasons, ", "))
			}
			fmt.Println("Reviewable means suitable for assessment. Age does not authorize exit.")
		})
		return nil
	}
	cmd := &cobra.Command{Use: "review", Short: "Review scoped terminal-agent observations without process control", Args: cobra.NoArgs, RunE: runList}
	cmd.PersistentFlags().StringVar(&endpoint, "herdr-endpoint", "", "Explicit absolute Unix Herdr endpoint (or inherited PERF_GUARD_HERDR_ENDPOINT)")
	cmd.PersistentFlags().StringVar(&session, "herdr-session", "", "Explicit Herdr session scope (or inherited PERF_GUARD_HERDR_SESSION)")
	cmd.Flags().IntVar(&limit, "limit", review.DefaultLimit, "Maximum records, 1..200")
	cmd.Flags().IntVar(&offset, "offset", 0, "Record offset")
	plan := &cobra.Command{Use: "plan", Short: "Inspect a fresh identity-bound dry-run plan; execution is unsupported", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if outputFlag != "" && outputFlag != "json" && outputFlag != "text" {
			return fmt.Errorf("output must be json or text")
		}
		if err := review.ValidateSelections(ids); err != nil {
			return err
		}
		t, err := target()
		if err != nil {
			return err
		}
		p, err := newReviewService().Plan(cmd.Context(), t, ids)
		if err != nil {
			return err
		}
		printer.Print("review plan", p, func() {
			fmt.Println("Dry-run plan: execution_supported=false; preserve shell; no sessions are exited.")
			for _, v := range p.Selections {
				fmt.Printf("%s  %s  %s\n", v.RequestedID, v.Status, strings.Join(v.Reasons, ", "))
			}
		})
		return nil
	}}
	plan.Flags().StringArrayVar(&ids, "session", nil, "Exact review ID to inspect (repeatable; never a pane ID)")
	cmd.AddCommand(plan)
	return cmd
}

func reviewCommandSchema() commandSchema {
	return commandSchema{Description: "Read-only scoped terminal-agent review; no execution capability", Flags: map[string]flagInfo{
		"--limit": {Type: "int", Default: "50", Description: "Maximum records, 1..200"}, "--offset": {Type: "int", Default: "0", Description: "Nonnegative record offset"},
		"--herdr-endpoint": {Type: "string", Description: "Explicit absolute local Unix endpoint"}, "--herdr-session": {Type: "string", Description: "Explicit session scope"}, "--output": {Type: "string", Values: []string{"json", "text"}, Description: "Output format"},
	}, OutputSchema: map[string]string{"schema_version": "1", "records": "ReviewRecord[]; contracts/review-v1/ReviewResponse.schema.json", "sources": "SourceStatus[]; fresh/partial/stale/unavailable", "total": "int (before pagination)", "reviewable_total": "int (before pagination)", "limit": "int", "offset": "int", "warnings": "Warning[]"}, Subcommands: map[string]commandSchema{"plan": {Description: "Fresh identity-bound dry-run plan; no apply or exit mode", Flags: map[string]flagInfo{"--session": {Type: "string[]", Description: "Repeat exact review ID; required"}}, OutputSchema: map[string]string{"execution_supported": "false (always)", "selections": "PlanSelection[] in request order", "schema_version": "1"}, Notes: []string{"Missing, changed, excluded and repeated IDs return per-selection results with success; no process control is invoked"}}}, Notes: []string{"Five-second overall collection deadline; explicit/inherited target only; age is not user activity", "RSS is resident memory; group sums are estimates", "Read-only sensor; no --dry-run flag is required"}}
}
