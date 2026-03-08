package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/diff"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

const (
	traceToolSourceCaptures = "tool_captures"
	traceToolSourceMetadata = "replay_metadata"
	traceToolSourceNone     = "none"
)

type traceComparisonReport struct {
	BaselineStepCount   int
	CandidateStepCount  int
	BaselineToolCount   int
	CandidateToolCount  int
	CandidateToolSource string
	Report              *diff.Report
}

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Demo-oriented workflows",
	Long:  `Run and inspect the judge-facing demo flows without changing the generic product commands.`,
}

var demoMigrationCmd = &cobra.Command{
	Use:   "migration",
	Short: "Database migration demo helpers",
	Long:  `Inspect the database migration governance demo traces and verdicts.`,
}

var demoMigrationVerdictCmd = &cobra.Command{
	Use:          "verdict",
	Short:        "Compare migration demo traces and print a verdict",
	Long:         `Loads a baseline trace and a candidate trace from CMDR storage and prints a judge-friendly PASS/FAIL comparison with first divergence.`,
	Args:         cobra.NoArgs,
	RunE:         runMigrationVerdict,
	SilenceUsage: true,
}

func init() {
	demoCmd.AddCommand(demoSeedCmd)
	demoCmd.AddCommand(demoGateCmd)
	demoCmd.AddCommand(demoMigrationCmd)
	demoMigrationCmd.AddCommand(demoMigrationVerdictCmd)

	demoMigrationVerdictCmd.Flags().String("baseline", "", "Baseline trace ID to compare against (required)")
	demoMigrationVerdictCmd.Flags().String("candidate", "", "Candidate trace ID to evaluate (required)")
	demoMigrationVerdictCmd.Flags().String("candidate-label", "candidate", "Human-readable label for the candidate trace")
	demoMigrationVerdictCmd.Flags().Float64("threshold", 0.8, "Similarity threshold used to label PASS/FAIL")
	_ = demoMigrationVerdictCmd.MarkFlagRequired("baseline")
	_ = demoMigrationVerdictCmd.MarkFlagRequired("candidate")
}

func runMigrationVerdict(cmd *cobra.Command, args []string) error {
	baselineTraceID, _ := cmd.Flags().GetString("baseline")
	candidateTraceID, _ := cmd.Flags().GetString("candidate")
	candidateLabel, _ := cmd.Flags().GetString("candidate-label")
	threshold, _ := cmd.Flags().GetFloat64("threshold")

	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("--threshold must be between 0.0 and 1.0, got %.2f", threshold)
	}

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	comparison, err := buildTraceComparisonReport(context.Background(), store, baselineTraceID, candidateTraceID, threshold)
	if err != nil {
		return err
	}

	printMigrationVerdict(cmd, baselineTraceID, candidateTraceID, candidateLabel, comparison)
	return nil
}

func buildTraceComparisonReport(
	ctx context.Context,
	store storage.Storage,
	baselineTraceID string,
	candidateTraceID string,
	threshold float64,
) (*traceComparisonReport, error) {
	baselineSteps, err := store.GetReplayTraceSpans(ctx, baselineTraceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get baseline replay traces: %w", err)
	}
	if len(baselineSteps) == 0 {
		return nil, fmt.Errorf("no replay traces found for baseline trace %s", baselineTraceID)
	}

	baselineCaptures, err := store.GetToolCapturesByTrace(ctx, baselineTraceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get baseline tool captures: %w", err)
	}

	candidateSteps, err := store.GetReplayTraceSpans(ctx, candidateTraceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get candidate replay traces: %w", err)
	}
	if len(candidateSteps) == 0 {
		return nil, fmt.Errorf("no replay traces found for candidate trace %s", candidateTraceID)
	}

	candidateCaptures, err := store.GetToolCapturesByTrace(ctx, candidateTraceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get candidate tool captures: %w", err)
	}

	candidateTools, candidateToolSource := resolveCandidateToolComparisonInputs(candidateSteps, candidateCaptures)

	report := diff.CompareAll(diff.CompareInput{
		Baseline:      baselineSteps,
		Variant:       candidateSteps,
		BaselineTools: baselineCaptures,
		VariantTools:  candidateTools,
	}, diff.Config{SimilarityThreshold: threshold})

	return &traceComparisonReport{
		BaselineStepCount:   len(baselineSteps),
		CandidateStepCount:  len(candidateSteps),
		BaselineToolCount:   len(baselineCaptures),
		CandidateToolCount:  len(candidateTools),
		CandidateToolSource: candidateToolSource,
		Report:              report,
	}, nil
}

func resolveCandidateToolComparisonInputs(candidateSteps []*storage.ReplayTrace, candidateCaptures []*storage.ToolCapture) ([]diff.ToolCall, string) {
	if len(candidateCaptures) > 0 {
		return diff.BaselineToolCalls(candidateCaptures), traceToolSourceCaptures
	}

	candidateTools := diff.ExtractVariantToolCalls(candidateSteps)
	if len(candidateTools) > 0 {
		return candidateTools, traceToolSourceMetadata
	}

	return nil, traceToolSourceNone
}

func printMigrationVerdict(cmd *cobra.Command, baselineTraceID, candidateTraceID, candidateLabel string, comparison *traceComparisonReport) {
	if candidateLabel == "" {
		candidateLabel = "candidate"
	}

	report := comparison.Report

	cmd.Printf("Migration Demo Verdict\n")
	cmd.Printf("=====================\n")
	cmd.Printf("Scenario:   database-migration\n")
	cmd.Printf("Baseline:   %s\n", baselineTraceID)
	cmd.Printf("Candidate:  %s (%s)\n", candidateTraceID, candidateLabel)
	cmd.Printf("Steps:      baseline=%d candidate=%d\n", comparison.BaselineStepCount, comparison.CandidateStepCount)
	cmd.Printf("Tools:      baseline=%d candidate=%d (%s)\n", comparison.BaselineToolCount, comparison.CandidateToolCount, displayTraceToolSource(comparison.CandidateToolSource))
	cmd.Printf("\n")
	cmd.Printf("Similarity: %.4f\n", report.SimilarityScore)
	cmd.Printf("Verdict:    %s\n", verdictDisplay(report.Verdict))

	cmd.Printf("\nDimensions:\n")
	if report.ToolCallScore != nil {
		cmd.Printf("  tool_calls    %.2f  (seq=%.2f, freq=%.2f)\n",
			report.ToolCallScore.Score, report.ToolCallScore.SequenceSimilarity, report.ToolCallScore.FrequencySimilarity)
	}
	if report.RiskScore != nil {
		escalation := "no escalation"
		if report.RiskScore.Escalation {
			escalation = "ESCALATION"
		}
		cmd.Printf("  risk          %.2f  (%s)\n", report.RiskScore.Score, escalation)
	}
	if report.ResponseScore != nil {
		cmd.Printf("  response      %.2f  (jaccard=%.2f, length=%.2f)\n",
			report.ResponseScore.Score, report.ResponseScore.ContentOverlap, report.ResponseScore.LengthSimilarity)
	}
	if summary := formatFirstDivergence(report.FirstDivergence); summary != "" {
		cmd.Printf("\nFirst Divergence:\n")
		cmd.Printf("  %s\n", summary)
	}

	if len(report.StepDiffs) > 0 {
		cmd.Printf("\nStep Breakdown:\n")
		cmd.Printf("%s", formatStepBreakdown(report.StepDiffs))
	}

	cmd.Printf("\nTotals: token_delta=%+d  latency_delta=%+dms\n", report.TokenDelta, report.LatencyDelta)
}

func displayTraceToolSource(source string) string {
	switch source {
	case traceToolSourceCaptures:
		return "candidate tool_captures"
	case traceToolSourceMetadata:
		return "candidate replay metadata"
	default:
		return "no candidate tool data"
	}
}
