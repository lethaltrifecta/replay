package commands

import (
	"context"
	"fmt"
	"text/tabwriter"

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

func init() {
	driftCmd.AddCommand(driftBaselineCmd)
	driftCmd.AddCommand(driftCheckCmd)
	driftBaselineCmd.AddCommand(driftBaselineSetCmd)
	driftBaselineCmd.AddCommand(driftBaselineListCmd)
	driftBaselineCmd.AddCommand(driftBaselineRemoveCmd)

	driftBaselineSetCmd.Flags().String("name", "", "Human-readable name for the baseline")
	driftBaselineSetCmd.Flags().String("description", "", "Description of the baseline")

	driftCheckCmd.Flags().String("baseline", "", "Baseline trace ID to compare against (defaults to most recent)")
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
	var baselineTraceID string

	if baselineFlag != "" {
		// Verify the explicit baseline exists
		_, err := store.GetBaseline(ctx, baselineFlag)
		if err != nil {
			return fmt.Errorf("baseline %q not found: %w", baselineFlag, err)
		}
		baselineTraceID = baselineFlag
	} else {
		// Auto-detect: use the most recently created baseline
		baselines, err := store.ListBaselines(ctx)
		if err != nil {
			return fmt.Errorf("failed to list baselines: %w", err)
		}
		if len(baselines) == 0 {
			return fmt.Errorf("no baselines found; set one with: cmdr drift baseline set <trace-id>")
		}
		// ListBaselines returns ordered by created_at DESC, so first is most recent
		baselineTraceID = baselines[0].TraceID
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
