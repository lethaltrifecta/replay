package commands

import (
	"context"
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
	"github.com/lethaltrifecta/replay/pkg/config"
	"github.com/lethaltrifecta/replay/pkg/diff"
	"github.com/lethaltrifecta/replay/pkg/replay"
)

var gateCmd = &cobra.Command{
	Use:   "gate",
	Short: "Deployment gate commands",
	Long:  `Replay baseline traces with a different model and produce a pass/fail verdict`,
}

var gateCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Replay a baseline trace and diff the results",
	Long:  `Sends each baseline prompt to a variant model via agentgateway, diffs the results, and produces a CI/CD-friendly exit code (0=pass, 1=fail).`,
	RunE:  runGateCheck,
	SilenceUsage: true, // Don't print usage on runtime errors
}

var gateReportCmd = &cobra.Command{
	Use:          "report <experiment-id>",
	Short:        "Show a gate check report for an experiment",
	Args:         cobra.ExactArgs(1),
	RunE:         runGateReport,
	SilenceUsage: true,
}

func init() {
	gateCmd.AddCommand(gateCheckCmd)
	gateCmd.AddCommand(gateReportCmd)

	gateCheckCmd.Flags().String("baseline", "", "Baseline trace ID to replay (required)")
	gateCheckCmd.Flags().String("model", "", "Variant model name (required)")
	gateCheckCmd.Flags().String("provider", "", "Variant provider (optional)")
	gateCheckCmd.Flags().Float64("threshold", 0.8, "Similarity threshold for pass verdict")
	_ = gateCheckCmd.MarkFlagRequired("baseline")
	_ = gateCheckCmd.MarkFlagRequired("model")
}

func runGateCheck(cmd *cobra.Command, args []string) error {
	baselineTraceID, _ := cmd.Flags().GetString("baseline")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")
	threshold, _ := cmd.Flags().GetFloat64("threshold")

	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("--threshold must be between 0.0 and 1.0, got %.2f", threshold)
	}

	// Load config and validate agentgateway is configured
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.RequireAgentgateway(); err != nil {
		return err
	}

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()

	// Create agentgateway client
	client := agwclient.NewClient(agwclient.ClientConfig{
		BaseURL:    cfg.AgentgatewayURL,
		Timeout:    cfg.AgentgatewayTimeout,
		MaxRetries: cfg.AgentgatewayRetries,
	})

	// Build variant config
	variant := replay.VariantConfig{
		Model:    model,
		Provider: provider,
	}

	// Run replay
	cmd.Printf("Replaying baseline %s with model %s...\n", baselineTraceID, model)

	engine := replay.NewEngine(store, client)
	result, err := engine.Run(ctx, baselineTraceID, variant)
	if err != nil {
		return fmt.Errorf("replay failed: %w", err)
	}

	// Reload traces for diff comparison
	baselineSteps, err := store.GetReplayTraceSpans(ctx, baselineTraceID)
	if err != nil {
		return fmt.Errorf("reload baseline: %w", err)
	}

	variantSteps, err := store.GetReplayTraceSpans(ctx, result.VariantTraceID)
	if err != nil {
		return fmt.Errorf("reload variant: %w", err)
	}

	// Diff
	diffCfg := diff.Config{SimilarityThreshold: threshold}
	report := diff.Compare(baselineSteps, variantSteps, diffCfg)

	// Persist analysis result
	analysisResult := diff.ToAnalysisResult(report, result.ExperimentID, result.BaselineRunID, result.VariantRunID)
	if err := store.CreateAnalysisResult(ctx, analysisResult); err != nil {
		cmd.PrintErrf("Warning: failed to persist analysis result: %v\n", err)
	}

	// Print summary
	printGateReport(cmd, baselineTraceID, model, result.ExperimentID, report)

	if report.Verdict == "fail" {
		return ErrGateFailed
	}

	return nil
}

// ErrGateFailed is returned when the gate check verdict is "fail".
// Callers should exit with code 1 when they see this error.
var ErrGateFailed = errors.New("gate check failed")

func runGateReport(cmd *cobra.Command, args []string) error {
	experimentID, err := uuid.Parse(args[0])
	if err != nil {
		return fmt.Errorf("invalid experiment ID: %w", err)
	}

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()

	exp, err := store.GetExperiment(ctx, experimentID)
	if err != nil {
		return fmt.Errorf("experiment not found: %w", err)
	}

	runs, err := store.ListExperimentRuns(ctx, experimentID)
	if err != nil {
		return fmt.Errorf("failed to list runs: %w", err)
	}

	results, err := store.GetAnalysisResults(ctx, experimentID)
	if err != nil {
		return fmt.Errorf("failed to get analysis results: %w", err)
	}

	// Print experiment header
	cmd.Printf("Gate Report\n")
	cmd.Printf("===========\n")
	cmd.Printf("Experiment:  %s\n", exp.ID)
	cmd.Printf("Baseline:    %s\n", exp.BaselineTraceID)
	cmd.Printf("Status:      %s\n", exp.Status)
	cmd.Printf("Created:     %s\n", exp.CreatedAt.Format("2006-01-02 15:04:05"))
	if exp.CompletedAt != nil {
		cmd.Printf("Completed:   %s\n", exp.CompletedAt.Format("2006-01-02 15:04:05"))
	}

	// Print runs
	cmd.Printf("\nRuns:\n")
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tSTATUS\tTRACE ID")
	for _, run := range runs {
		traceID := "<pending>"
		if run.TraceID != nil {
			traceID = *run.TraceID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", run.ID, run.RunType, run.Status, traceID)
	}
	w.Flush()

	// Print analysis results
	if len(results) > 0 {
		cmd.Printf("\nAnalysis:\n")
		for _, r := range results {
			cmd.Printf("  Similarity: %.4f\n", r.SimilarityScore)
			cmd.Printf("  Token Delta: %+d\n", r.TokenDelta)
			cmd.Printf("  Latency Delta: %+dms\n", r.LatencyDelta)

			if verdict, ok := r.BehaviorDiff["verdict"].(string); ok {
				cmd.Printf("  Verdict: %s\n", verdictDisplay(verdict))
			}
		}
	} else {
		cmd.Printf("\nNo analysis results found.\n")
	}

	return nil
}

func printGateReport(cmd *cobra.Command, baselineTraceID, model string, experimentID uuid.UUID, report *diff.Report) {
	cmd.Printf("\nGate Check Result\n")
	cmd.Printf("=================\n")
	cmd.Printf("Baseline:   %s\n", baselineTraceID)
	cmd.Printf("Variant:    %s\n", model)
	cmd.Printf("Steps:      %d replayed\n", report.StepCount)
	cmd.Printf("\n")
	cmd.Printf("Similarity: %.4f\n", report.SimilarityScore)
	cmd.Printf("Verdict:    %s\n", verdictDisplay(report.Verdict))

	if len(report.StepDiffs) > 0 {
		cmd.Printf("\nStep Breakdown:\n")
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  STEP\tTOKEN DELTA")
		for _, sd := range report.StepDiffs {
			fmt.Fprintf(w, "  %d\t%+d\n", sd.StepIndex, sd.TokenDelta)
		}
		w.Flush()
	}

	cmd.Printf("\nTotals: token_delta=%+d  latency_delta=%+dms\n", report.TokenDelta, report.LatencyDelta)
	cmd.Printf("Experiment: %s\n", experimentID)
}

