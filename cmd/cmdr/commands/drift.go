package commands

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/config"
	"github.com/lethaltrifecta/replay/pkg/drift"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Drift detection commands",
	Long:  `Manage baselines and view drift detection results`,
}

var driftBaselineCmd = &cobra.Command{
	Use:   "baseline",
	Short: "Manage baselines",
	Long:  `Set, list, and remove baseline traces for drift comparison`,
}

var driftBaselineSetCmd = &cobra.Command{
	Use:   "set <trace-id>",
	Short: "Mark a trace as a baseline",
	Args:  cobra.ExactArgs(1),
	RunE:  runDriftBaselineSet,
}

var driftBaselineListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all baselines",
	RunE:  runDriftBaselineList,
}

var driftBaselineRemoveCmd = &cobra.Command{
	Use:   "remove <trace-id>",
	Short: "Remove a baseline",
	Args:  cobra.ExactArgs(1),
	RunE:  runDriftBaselineRemove,
}

var driftCheckCmd = &cobra.Command{
	Use:   "check <trace-id>",
	Short: "Compare a trace against a baseline for drift",
	Long:  `Extracts behavioral fingerprints from both traces and computes a drift score.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDriftCheck,
}

var driftStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show drift scores for recent traces",
	Long:  `Displays a table of recent drift check results. Optionally filter by baseline.`,
	RunE:  runDriftStatus,
}

var driftWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Continuously watch for new traces and check drift",
	Long:  `Polls for new replay traces and automatically compares them against the baseline. Press Ctrl+C to stop.`,
	RunE:  runDriftWatch,
}

func init() {
	driftCmd.AddCommand(driftBaselineCmd)
	driftCmd.AddCommand(driftCheckCmd)
	driftCmd.AddCommand(driftStatusCmd)
	driftCmd.AddCommand(driftWatchCmd)
	driftBaselineCmd.AddCommand(driftBaselineSetCmd)
	driftBaselineCmd.AddCommand(driftBaselineListCmd)
	driftBaselineCmd.AddCommand(driftBaselineRemoveCmd)

	driftBaselineSetCmd.Flags().String("name", "", "Human-readable name for the baseline")
	driftBaselineSetCmd.Flags().String("description", "", "Description of the baseline")

	driftCheckCmd.Flags().String("baseline", "", "Baseline trace ID to compare against (defaults to most recent)")

	driftStatusCmd.Flags().Int("limit", 20, "Maximum number of results to show")
	driftStatusCmd.Flags().String("baseline", "", "Filter results by baseline trace ID")

	driftWatchCmd.Flags().String("baseline", "", "Baseline trace ID (defaults to most recent)")
	driftWatchCmd.Flags().Duration("interval", 30*time.Second, "Polling interval")
}

func connectDB() (*storage.PostgresStorage, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	ctx := context.Background()
	store, err := storage.NewPostgresStorage(ctx, cfg.PostgresURL, cfg.PostgresMaxConn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := store.Migrate(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return store, nil
}

func runDriftBaselineSet(cmd *cobra.Command, args []string) error {
	traceID := args[0]

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")

	baseline := &storage.Baseline{
		TraceID: traceID,
	}
	if name != "" {
		baseline.Name = &name
	}
	if description != "" {
		baseline.Description = &description
	}

	ctx := context.Background()
	if err := store.MarkTraceAsBaseline(ctx, baseline); err != nil {
		return fmt.Errorf("failed to set baseline: %w", err)
	}

	cmd.Printf("Baseline set for trace %s\n", traceID)
	if name != "" {
		cmd.Printf("  Name: %s\n", name)
	}
	if description != "" {
		cmd.Printf("  Description: %s\n", description)
	}

	return nil
}

func runDriftBaselineList(cmd *cobra.Command, args []string) error {
	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	baselines, err := store.ListBaselines(ctx)
	if err != nil {
		return fmt.Errorf("failed to list baselines: %w", err)
	}

	if len(baselines) == 0 {
		cmd.Println("No baselines found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TRACE ID\tNAME\tDESCRIPTION\tCREATED AT")
	for _, b := range baselines {
		name := ""
		if b.Name != nil {
			name = *b.Name
		}
		desc := ""
		if b.Description != nil {
			desc = *b.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			b.TraceID, name, desc, b.CreatedAt.Format("2006-01-02 15:04:05"),
		)
	}
	w.Flush()

	return nil
}

func runDriftBaselineRemove(cmd *cobra.Command, args []string) error {
	traceID := args[0]

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.UnmarkBaseline(ctx, traceID); err != nil {
		return fmt.Errorf("failed to remove baseline: %w", err)
	}

	cmd.Printf("Baseline removed for trace %s\n", traceID)
	return nil
}

func runDriftCheck(cmd *cobra.Command, args []string) error {
	candidateTraceID := args[0]

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()

	// Resolve baseline trace ID
	baselineFlag, _ := cmd.Flags().GetString("baseline")
	baselineTraceID, err := resolveBaseline(ctx, store, baselineFlag)
	if err != nil {
		return err
	}

	// Fetch data for both traces
	baselineSpans, err := store.GetReplayTraceSpans(ctx, baselineTraceID)
	if err != nil {
		return fmt.Errorf("failed to get baseline spans: %w", err)
	}
	if len(baselineSpans) == 0 {
		return fmt.Errorf("no replay spans found for baseline trace %s", baselineTraceID)
	}

	baselineTools, err := store.GetToolCapturesByTrace(ctx, baselineTraceID)
	if err != nil {
		return fmt.Errorf("failed to get baseline tool captures: %w", err)
	}

	candidateSpans, err := store.GetReplayTraceSpans(ctx, candidateTraceID)
	if err != nil {
		return fmt.Errorf("failed to get candidate spans: %w", err)
	}
	if len(candidateSpans) == 0 {
		return fmt.Errorf("no replay spans found for candidate trace %s", candidateTraceID)
	}

	candidateTools, err := store.GetToolCapturesByTrace(ctx, candidateTraceID)
	if err != nil {
		return fmt.Errorf("failed to get candidate tool captures: %w", err)
	}

	// Extract fingerprints and compare
	baselineFP, err := drift.Extract(baselineTraceID, baselineSpans, baselineTools)
	if err != nil {
		return fmt.Errorf("failed to extract baseline fingerprint: %w", err)
	}

	candidateFP, err := drift.Extract(candidateTraceID, candidateSpans, candidateTools)
	if err != nil {
		return fmt.Errorf("failed to extract candidate fingerprint: %w", err)
	}

	cfg := drift.DefaultConfig()
	report := drift.Compare(baselineFP, candidateFP, cfg)

	// Persist result
	result := &storage.DriftResult{
		TraceID:         candidateTraceID,
		BaselineTraceID: baselineTraceID,
		DriftScore:      report.Score,
		Verdict:         report.Verdict,
		Details:         storage.JSONB(report.Details),
	}
	if err := store.CreateDriftResult(ctx, result); err != nil {
		return fmt.Errorf("failed to store drift result: %w", err)
	}

	// Print summary
	cmd.Println("Drift Check Result")
	cmd.Println("==================")
	cmd.Printf("Trace:    %s\n", candidateTraceID)
	cmd.Printf("Baseline: %s\n", baselineTraceID)
	cmd.Printf("Score:    %.3f\n", report.Score)
	cmd.Printf("Verdict:  %s\n\n", verdictDisplay(report.Verdict))

	cmd.Println("Dimension Breakdown:")
	cmd.Printf("  Tool Order:     %.3f\n", report.Details["tool_order_score"])
	cmd.Printf("  Tool Frequency: %.3f\n", report.Details["tool_frequency_score"])
	cmd.Printf("  Risk Shift:     %.3f\n", report.Details["risk_shift_score"])
	cmd.Printf("  Token Delta:    %.3f\n", report.Details["token_delta_score"])

	if modelChanged, ok := report.Details["model_changed"].(bool); ok && modelChanged {
		cmd.Println("\n  WARNING: Model changed between baseline and candidate")
	}
	if providerChanged, ok := report.Details["provider_changed"].(bool); ok && providerChanged {
		cmd.Println("  WARNING: Provider changed between baseline and candidate")
	}

	return nil
}

func runDriftStatus(cmd *cobra.Command, args []string) error {
	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	baselineFlag, _ := cmd.Flags().GetString("baseline")
	limit, _ := cmd.Flags().GetInt("limit")

	var results []*storage.DriftResult
	if baselineFlag != "" {
		results, err = store.GetDriftResultsByBaseline(ctx, baselineFlag)
		if err != nil {
			return fmt.Errorf("failed to get drift results: %w", err)
		}
		// Apply --limit to baseline-filtered results too
		if limit > 0 && len(results) > limit {
			results = results[:limit]
		}
	} else {
		if limit <= 0 {
			limit = 20
		}
		results, err = store.ListDriftResults(ctx, limit)
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
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%.3f\t%s\t%s\n",
			r.TraceID,
			r.BaselineTraceID,
			r.DriftScore,
			verdictDisplay(r.Verdict),
			r.CreatedAt.Format("2006-01-02 15:04:05"),
		)
	}
	w.Flush()

	return nil
}

func resolveBaseline(ctx context.Context, store storage.Storage, baselineFlag string) (string, error) {
	if baselineFlag != "" {
		_, err := store.GetBaseline(ctx, baselineFlag)
		if err != nil {
			return "", fmt.Errorf("baseline %q not found: %w", baselineFlag, err)
		}
		return baselineFlag, nil
	}

	baselines, err := store.ListBaselines(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list baselines: %w", err)
	}
	if len(baselines) == 0 {
		return "", fmt.Errorf("no baselines found; set one with: cmdr drift baseline set <trace-id>")
	}
	return baselines[0].TraceID, nil
}

// maxPollRows is a safety cap on the number of rows fetched per poll cycle.
// The time window is the primary bound; this prevents unbounded queries after
// long gaps or on first run.
const maxPollRows = 10000

// maxRetries is the number of times a trace is retried before being abandoned.
const maxRetries = 3

func runDriftWatch(cmd *cobra.Command, args []string) error {
	baselineFlag, _ := cmd.Flags().GetString("baseline")
	interval, _ := cmd.Flags().GetDuration("interval")

	if interval <= 0 {
		return fmt.Errorf("--interval must be positive, got %s", interval)
	}

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	baselineTraceID, err := resolveBaseline(ctx, store, baselineFlag)
	if err != nil {
		return err
	}

	// Pre-compute baseline fingerprint once — it's immutable
	baselineFP, err := extractBaselineFP(ctx, store, baselineTraceID)
	if err != nil {
		return err
	}

	cmd.Printf("Watching for new traces (baseline: %s, interval: %s)...\n", baselineTraceID, interval)

	cfg := drift.DefaultConfig()
	lastPoll := time.Now()
	retryTraces := make(map[string]int) // trace_id -> attempt count

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cmd.Println("Stopped watching.")
			return nil
		case <-ticker.C:
			result := pollAndCheck(ctx, cmd, store, baselineTraceID, baselineFP, cfg, lastPoll, retryTraces)
			if result.err != nil {
				if ctx.Err() != nil {
					cmd.Println("Stopped watching.")
					return nil
				}
				cmd.PrintErrf("Poll error: %v\n", result.err)
				// Don't advance cursor on poll-level failure
				continue
			}
			if result.checked > 0 {
				cmd.Printf("Checked %d new trace(s)\n", result.checked)
			}
			if result.overflow {
				cmd.PrintErrf("Warning: poll returned %d rows (limit). Some traces may be delayed. Consider decreasing --interval.\n", maxPollRows)
			}
			// Advance cursor based on what was returned, not wall-clock time.
			// This eliminates clock-skew between Go process and DB.
			if !result.highWater.IsZero() {
				lastPoll = result.highWater
			}
		}
	}
}

// extractBaselineFP fetches baseline spans/tools and returns the fingerprint.
func extractBaselineFP(ctx context.Context, store storage.Storage, baselineTraceID string) (*drift.Fingerprint, error) {
	baselineSpans, err := store.GetReplayTraceSpans(ctx, baselineTraceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get baseline spans: %w", err)
	}
	if len(baselineSpans) == 0 {
		return nil, fmt.Errorf("no replay spans found for baseline trace %s", baselineTraceID)
	}

	baselineTools, err := store.GetToolCapturesByTrace(ctx, baselineTraceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get baseline tool captures: %w", err)
	}

	fp, err := drift.Extract(baselineTraceID, baselineSpans, baselineTools)
	if err != nil {
		return nil, fmt.Errorf("failed to extract baseline fingerprint: %w", err)
	}
	return fp, nil
}

// pollResult holds the outcome of a single poll cycle.
type pollResult struct {
	checked   int
	highWater time.Time // newest created_at seen; used as next poll cursor
	overflow  bool      // true if the query hit maxPollRows
	err       error     // non-nil for poll-level failures (not per-trace)
}

func pollAndCheck(ctx context.Context, cmd *cobra.Command, store storage.Storage, baselineTraceID string, baselineFP *drift.Fingerprint, cfg drift.CompareConfig, since time.Time, retryTraces map[string]int) pollResult {
	filters := storage.TraceFilters{
		StartTime: &since,
		Limit:     maxPollRows,
	}
	spans, err := store.ListReplayTraces(ctx, filters)
	if err != nil {
		return pollResult{err: fmt.Errorf("failed to list replay traces: %w", err)}
	}

	overflow := len(spans) >= maxPollRows

	// Compute the cursor for the next poll.
	// ListReplayTraces returns DESC order, so spans[0] is newest and
	// spans[len-1] is oldest. On overflow, hold the cursor at the oldest
	// returned row so the next poll re-enters the truncated window.
	// On a normal (non-overflow) poll, advance to the newest row.
	var highWater time.Time
	if len(spans) > 0 {
		if overflow {
			highWater = spans[len(spans)-1].CreatedAt
		} else {
			highWater = spans[0].CreatedAt
		}
	}

	// Deduplicate by trace_id, preserving first-seen order
	seen := make(map[string]bool)
	var candidates []string
	for _, span := range spans {
		if seen[span.TraceID] {
			continue
		}
		seen[span.TraceID] = true

		if span.TraceID == baselineTraceID {
			continue
		}

		// Baseline-aware dedup: only skip if checked against THIS baseline
		checked, err := store.HasDriftResultForBaseline(ctx, span.TraceID, baselineTraceID)
		if err != nil {
			return pollResult{highWater: highWater, err: fmt.Errorf("failed to check drift result for %s: %w", span.TraceID, err)}
		}
		if checked {
			continue
		}

		candidates = append(candidates, span.TraceID)
	}

	// Add retry traces (from previous failed polls). They may be outside the
	// current time window, so they won't appear in the query results.
	// Re-check existence to avoid duplicates (e.g. concurrent drift check or
	// a previous retry that partially succeeded).
	for traceID := range retryTraces {
		if seen[traceID] {
			continue
		}
		checked, err := store.HasDriftResultForBaseline(ctx, traceID, baselineTraceID)
		if err != nil {
			return pollResult{highWater: highWater, err: fmt.Errorf("failed to check drift result for retry %s: %w", traceID, err)}
		}
		if checked {
			delete(retryTraces, traceID)
			continue
		}
		candidates = append(candidates, traceID)
	}

	if len(candidates) == 0 {
		return pollResult{highWater: highWater, overflow: overflow}
	}

	checkedCount := 0
	for _, traceID := range candidates {
		if ctx.Err() != nil {
			return pollResult{checked: checkedCount, highWater: highWater, overflow: overflow, err: ctx.Err()}
		}

		report, err := checkOneTrace(ctx, store, baselineTraceID, baselineFP, cfg, traceID)
		if err != nil {
			retryTraces[traceID]++
			if retryTraces[traceID] >= maxRetries {
				cmd.PrintErrf("  Abandon %s after %d failures: %v\n", traceID, maxRetries, err)
				delete(retryTraces, traceID)
			} else {
				cmd.PrintErrf("  Retry %s (attempt %d/%d): %v\n", traceID, retryTraces[traceID], maxRetries, err)
			}
			continue
		}

		delete(retryTraces, traceID)
		cmd.Printf("  %s  score=%.3f  verdict=%s\n", traceID, report.Score, verdictDisplay(report.Verdict))
		checkedCount++
	}

	return pollResult{checked: checkedCount, highWater: highWater, overflow: overflow}
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

	result := &storage.DriftResult{
		TraceID:         traceID,
		BaselineTraceID: baselineTraceID,
		DriftScore:      report.Score,
		Verdict:         report.Verdict,
		Details:         storage.JSONB(report.Details),
	}
	if err := store.CreateDriftResult(ctx, result); err != nil {
		return nil, fmt.Errorf("failed to store result: %w", err)
	}

	return report, nil
}

func verdictDisplay(verdict string) string {
	switch verdict {
	case storage.DriftVerdictPass:
		return "PASS"
	case storage.DriftVerdictWarn:
		return "WARN"
	case storage.DriftVerdictFail:
		return "FAIL"
	default:
		return verdict
	}
}
