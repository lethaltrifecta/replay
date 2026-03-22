package commands

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/drift"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Manage and inspect behavioral drift",
	Long:  `Detect, calculate, and monitor behavioral drift between agent traces and baselines.`,
}

var driftCheckCmd = &cobra.Command{
	Use:   "check <baseline-trace-id> <candidate-trace-id>",
	Short: "Calculate drift between two specific traces",
	Args:  cobra.ExactArgs(2),
	RunE:  runDriftCheck,
}

var driftListCmd = &cobra.Command{
	Use:   "list",
	Short: "List calculated drift results",
	RunE:  runDriftList,
}

var driftWatchCmd = &cobra.Command{
	Use:   "watch <baseline-trace-id>",
	Short: "Monitor new traces and calculate drift against baseline in real-time",
	Args:  cobra.ExactArgs(1),
	RunE:  runDriftWatch,
}

func init() {
	driftCmd.AddCommand(driftCheckCmd)
	driftCmd.AddCommand(driftListCmd)
	driftCmd.AddCommand(driftWatchCmd)

	driftListCmd.Flags().String("baseline", "", "Filter by baseline trace ID")
	driftListCmd.Flags().Int("limit", 20, "Maximum number of results to show")

	driftWatchCmd.Flags().Int("interval", 5, "Polling interval in seconds")
	driftWatchCmd.Flags().String("model", "", "Filter candidate traces by model")
	driftWatchCmd.Flags().String("provider", "", "Filter candidate traces by provider")
}

func runDriftCheck(cmd *cobra.Command, args []string) error {
	baselineTraceID := args[0]
	candidateTraceID := args[1]

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()

	// 1. Get Baseline
	baselineSpans, err := store.GetReplayTraceSpans(ctx, baselineTraceID)
	if err != nil {
		return fmt.Errorf("failed to get baseline spans: %w", err)
	}
	if len(baselineSpans) == 0 {
		return fmt.Errorf("baseline trace %s not found", baselineTraceID)
	}

	baselineTools, err := store.GetToolCapturesByTrace(ctx, baselineTraceID)
	if err != nil {
		return fmt.Errorf("failed to get baseline tool captures: %w", err)
	}

	baselineFP, err := drift.Extract(baselineTraceID, baselineSpans, baselineTools)
	if err != nil {
		return fmt.Errorf("failed to extract baseline fingerprint: %w", err)
	}

	// 2. Get Candidate
	candidateSpans, err := store.GetReplayTraceSpans(ctx, candidateTraceID)
	if err != nil {
		return fmt.Errorf("failed to get candidate spans: %w", err)
	}
	if len(candidateSpans) == 0 {
		return fmt.Errorf("candidate trace %s not found", candidateTraceID)
	}

	candidateTools, err := store.GetToolCapturesByTrace(ctx, candidateTraceID)
	if err != nil {
		return fmt.Errorf("failed to get candidate tool captures: %w", err)
	}

	candidateFP, err := drift.Extract(candidateTraceID, candidateSpans, candidateTools)
	if err != nil {
		return fmt.Errorf("failed to extract candidate fingerprint: %w", err)
	}

	cfg := drift.DefaultConfig()
	report := drift.Compare(baselineFP, candidateFP, cfg)

	// Persistence
	var reason string
	if r, ok := report.Details["reason"].(string); ok {
		reason = r
	}
	var divStep int
	if ds, ok := report.Details["divergence_step"].(int); ok {
		divStep = ds
	} else if ds, ok := report.Details["divergence_step"].(float64); ok {
		divStep = int(ds)
	}

	res := &storage.DriftResult{
		TraceID:         candidateTraceID,
		BaselineTraceID: baselineTraceID,
		DriftScore:      report.Score,
		Verdict:         report.Verdict,
		Details: storage.DriftDetails{
			Reason:         reason,
			DivergenceStep: divStep,
		},
	}

	if err := store.CreateDriftResult(ctx, res); err != nil {
		return fmt.Errorf("failed to store drift result: %w", err)
	}

	// Print summary
	cmd.Println("Drift Check Result")
	cmd.Println("==================")
	cmd.Printf("Trace:    %s\n", candidateTraceID)
	cmd.Printf("Baseline: %s\n", baselineTraceID)
	cmd.Printf("Score:    %.3f\n", report.Score)
	cmd.Printf("Verdict:  %s\n", verdictDisplay(report.Verdict))

	return nil
}

func runDriftList(cmd *cobra.Command, args []string) error {
	baselineFlag, _ := cmd.Flags().GetString("baseline")
	limit, _ := cmd.Flags().GetInt("limit")

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()

	var results []*storage.DriftResult
	if baselineFlag != "" {
		results, err = store.GetDriftResultsByBaseline(ctx, baselineFlag, limit)
		if err != nil {
			return fmt.Errorf("failed to get drift results: %w", err)
		}
	} else {
		results, err = store.ListDriftResults(ctx, limit, 0)
		if err != nil {
			return fmt.Errorf("failed to list drift results: %w", err)
		}
	}

	if len(results) == 0 {
		cmd.Println("No drift results found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TRACE ID\tBASELINE\tSCORE\tVERDICT\tCHECKED AT")
	for _, res := range results {
		fmt.Fprintf(w, "%s\t%s\t%.3f\t%s\t%s\n",
			res.TraceID,
			res.BaselineTraceID,
			res.DriftScore,
			verdictDisplay(res.Verdict),
			res.CreatedAt.Format(time.RFC3339),
		)
	}
	return w.Flush()
}

func runDriftWatch(cmd *cobra.Command, args []string) error {
	baselineTraceID := args[0]
	interval, _ := cmd.Flags().GetInt("interval")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()

	// Verify baseline exists and extract its fingerprint
	baselineSpans, err := store.GetReplayTraceSpans(ctx, baselineTraceID)
	if err != nil {
		return err
	}
	if len(baselineSpans) == 0 {
		return fmt.Errorf("baseline trace %s not found", baselineTraceID)
	}
	baselineTools, err := store.GetToolCapturesByTrace(ctx, baselineTraceID)
	if err != nil {
		return fmt.Errorf("failed to load baseline tool captures: %w", err)
	}
	baselineFP, err := drift.Extract(baselineTraceID, baselineSpans, baselineTools)
	if err != nil {
		return err
	}

	cmd.Printf("Watching for new traces against baseline %s...\n", baselineTraceID)
	cmd.Printf("Polling every %d seconds. Press Ctrl+C to stop.\n\n", interval)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// Use the newest trace we've already seen as the watermark
	var highWater time.Time
	seedRows, err := store.ListReplayTraces(ctx, storage.TraceFilters{Limit: 1})
	if err == nil && len(seedRows) > 0 {
		highWater = seedRows[0].CreatedAt
	}

	cfg := drift.DefaultConfig()
	filters := storage.TraceFilters{
		SortAsc: false,
	}
	if model != "" {
		filters.Model = &model
	}
	if provider != "" {
		filters.Provider = &provider
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			res, err := pollAndCheck(ctx, store, baselineTraceID, baselineFP, cfg, filters, highWater)
			if err != nil {
				cmd.PrintErrf("poll failed: %v\n", err)
				continue
			}
			if !res.highWater.IsZero() {
				highWater = res.highWater
			}
			if res.overflow {
				cmd.PrintErrln("processed drift-watch backlog across multiple pages")
			}
		}
	}
}

type pollResult struct {
	checked   int
	highWater time.Time
	overflow  bool
}

func pollAndCheck(ctx context.Context, store storage.Storage, baselineTraceID string, baselineFP *drift.Fingerprint, cfg drift.CompareConfig, filters storage.TraceFilters, lastSeen time.Time) (pollResult, error) {
	const pageSize = 100

	// Deduplicate by trace_id, preserving first-seen order across the full backlog window.
	seen := make(map[string]bool)
	var uniqueTraceIDs []string

	var (
		checkedCount int
		highWater    time.Time
		hadFailures  bool
		pageCount    int
	)

	for offset := 0; ; offset += pageSize {
		pageFilters := filters
		pageFilters.StartTime = &lastSeen
		pageFilters.Limit = pageSize
		pageFilters.Offset = offset

		spans, err := store.ListReplayTraces(ctx, pageFilters)
		if err != nil {
			return pollResult{}, err
		}
		if len(spans) == 0 {
			break
		}
		pageCount++

		pageHighWater := spans[0].CreatedAt
		if filters.SortAsc {
			pageHighWater = spans[len(spans)-1].CreatedAt
		}
		if pageHighWater.After(highWater) {
			highWater = pageHighWater
		}

		for _, span := range spans {
			if span.TraceID == baselineTraceID {
				continue // Skip comparing baseline to itself
			}
			if seen[span.TraceID] {
				continue
			}

			exists, err := store.HasDriftResultForBaseline(ctx, span.TraceID, baselineTraceID)
			if err != nil {
				return pollResult{}, err
			}
			if !exists {
				uniqueTraceIDs = append(uniqueTraceIDs, span.TraceID)
			}
			seen[span.TraceID] = true
		}

		if len(spans) < pageSize {
			break
		}
	}

	for _, traceID := range uniqueTraceIDs {
		report, err := checkOneTrace(ctx, store, baselineTraceID, baselineFP, cfg, traceID)
		if err != nil {
			hadFailures = true
			continue
		}
		fmt.Printf("  %s  score=%.3f  verdict=%s\n", traceID, report.Score, verdictDisplay(report.Verdict))
		checkedCount++
	}

	if hadFailures {
		highWater = lastSeen
	}

	return pollResult{checked: checkedCount, highWater: highWater, overflow: pageCount > 1}, nil
}

// checkOneTrace fetches spans/tools for a single trace, compares against the baseline,
// and persists the drift result. Returns the report on success.
func checkOneTrace(ctx context.Context, store storage.Storage, baselineTraceID string, baselineFP *drift.Fingerprint, cfg drift.CompareConfig, traceID string) (*drift.DriftReport, error) {
	candidateSpans, err := store.GetReplayTraceSpans(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get spans: %w", err)
	}
	if len(candidateSpans) == 0 {
		return nil, fmt.Errorf("no spans found")
	}

	candidateTools, err := store.GetToolCapturesByTrace(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool captures: %w", err)
	}

	candidateFP, err := drift.Extract(traceID, candidateSpans, candidateTools)
	if err != nil {
		return nil, fmt.Errorf("failed to extract fingerprint: %w", err)
	}

	report := drift.Compare(baselineFP, candidateFP, cfg)

	var reason string
	if r, ok := report.Details["reason"].(string); ok {
		reason = r
	}
	var divStep int
	if ds, ok := report.Details["divergence_step"].(int); ok {
		divStep = ds
	} else if ds, ok := report.Details["divergence_step"].(float64); ok {
		divStep = int(ds)
	}

	result := &storage.DriftResult{
		TraceID:         traceID,
		BaselineTraceID: baselineTraceID,
		DriftScore:      report.Score,
		Verdict:         report.Verdict,
		Details: storage.DriftDetails{
			Reason:         reason,
			DivergenceStep: divStep,
		},
	}
	if err := store.CreateDriftResult(ctx, result); err != nil {
		return nil, fmt.Errorf("failed to store result: %w", err)
	}

	return report, nil
}
