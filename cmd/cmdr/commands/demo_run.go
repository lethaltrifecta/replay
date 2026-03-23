package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func runMigrationDemo(cmd *cobra.Command, args []string) error {
	reportDir, _ := cmd.Flags().GetString("report-dir")
	goBin, _ := cmd.Flags().GetString("go-bin")
	agwDir, _ := cmd.Flags().GetString("agentgateway-dir")
	freezeDir, _ := cmd.Flags().GetString("freeze-dir")
	threshold, _ := cmd.Flags().GetFloat64("threshold")

	if reportDir == "" {
		reportDir = filepath.Join("artifacts", "migration-demo", time.Now().Format("20060102-150405"))
	}

	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	cmd.Printf("Starting full-loop migration demo...\n")
	cmd.Printf("Artifacts will be saved to: %s\n", reportDir)

	harnessArgs := []string{
		"scripts/test-migration-demo-full-loop.sh",
	}

	env := os.Environ()
	env = setEnvValue(env, "REPORT_DIR", reportDir)
	if goBin != "" {
		env = setEnvValue(env, "GO_BIN", goBin)
	}
	if agwDir != "" {
		env = setEnvValue(env, "AGENTGATEWAY_DIR", agwDir)
	}
	if freezeDir != "" {
		env = setEnvValue(env, "FREEZE_DIR", freezeDir)
	}

	if err := runCommandWithStream(cmd, harnessArgs[0], harnessArgs[1:], env); err != nil {
		return fmt.Errorf("demo harness failed: %w", err)
	}

	summaryPath := filepath.Join(reportDir, "run-summary.json")
	if !fileExists(summaryPath) {
		return fmt.Errorf("demo completed but summary file not found at %s", summaryPath)
	}

	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		return fmt.Errorf("failed to read summary file: %w", err)
	}

	var summary migrationDemoRunSummary
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		return fmt.Errorf("failed to parse summary file: %w", err)
	}

	cmd.Printf("\nDemo cycle complete. Building comparison reports...\n")

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	safeComparison, err := buildTraceComparisonReport(context.Background(), store, summary.BaselineTraceID, summary.SafeReplayTraceID, threshold)
	if err != nil {
		return fmt.Errorf("build safe comparison: %w", err)
	}

	unsafeComparison, err := buildTraceComparisonReport(context.Background(), store, summary.BaselineTraceID, summary.UnsafeReplayTraceID, threshold)
	if err != nil {
		return fmt.Errorf("build unsafe comparison: %w", err)
	}

	artifact := &migrationDemoReportArtifact{
		GeneratedAt:    time.Now().UTC(),
		ReportDir:      reportDir,
		Scenario:       "database-migration",
		JudgeHighlight: migrationJudgeHighlight(&summary, unsafeComparison),
		Summary:        &summary,
		Safe: &migrationDemoVerdictInfo{
			Label:           "safe-replay",
			TraceID:         summary.SafeReplayTraceID,
			FirstDivergence: formatFirstDivergence(toStructuredDivergence(safeComparison.Report.FirstDivergence)),
			Comparison:      safeComparison,
		},
		Unsafe: &migrationDemoVerdictInfo{
			Label:           "unsafe-replay",
			TraceID:         summary.UnsafeReplayTraceID,
			FirstDivergence: formatFirstDivergence(toStructuredDivergence(unsafeComparison.Report.FirstDivergence)),
			Comparison:      unsafeComparison,
		},
	}

	jsonArtifactPath := filepath.Join(reportDir, "report.json")
	if err := writeJSONFile(jsonArtifactPath, artifact); err != nil {
		return err
	}

	markdownArtifactPath := filepath.Join(reportDir, "report.md")
	if err := os.WriteFile(markdownArtifactPath, []byte(renderMigrationDemoReportMarkdown(artifact)), 0o644); err != nil {
		return err
	}

	cmd.Printf("\nComparison Reports Generated:\n")
	cmd.Printf("  JSON: %s\n", jsonArtifactPath)
	cmd.Printf("  Markdown: %s\n", markdownArtifactPath)

	cmd.Printf("\nFinal Verdicts:\n")
	cmd.Printf("  Safe Replay:   %s (Similarity: %.4f)\n", verdictDisplay(safeComparison.Report.Verdict), safeComparison.Report.SimilarityScore)
	cmd.Printf("  Unsafe Replay: %s (Similarity: %.4f)\n", verdictDisplay(unsafeComparison.Report.Verdict), unsafeComparison.Report.SimilarityScore)

	return nil
}

func renderMigrationDemoReportMarkdown(artifact *migrationDemoReportArtifact) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("# Migration Demo Report: %s\n\n", artifact.Scenario))
	builder.WriteString(fmt.Sprintf("**Generated at:** %s\n\n", artifact.GeneratedAt.Format(time.RFC1123)))

	builder.WriteString("## Executive Summary\n\n")
	builder.WriteString(artifact.JudgeHighlight + "\n\n")

	writeMigrationVerdictMarkdown(&builder, artifact.Safe)
	writeMigrationVerdictMarkdown(&builder, artifact.Unsafe)

	builder.WriteString("## Artifacts\n\n")
	builder.WriteString(fmt.Sprintf("- `report.json`: `%s`\n", filepath.Join(artifact.ReportDir, "report.json")))
	builder.WriteString(fmt.Sprintf("- `report.md`: `%s`\n", filepath.Join(artifact.ReportDir, "report.md")))
	builder.WriteString(fmt.Sprintf("- safe verdict log: `%s`\n", artifact.Summary.Logs.SafeVerdict))
	builder.WriteString(fmt.Sprintf("- unsafe verdict log: `%s`\n", artifact.Summary.Logs.UnsafeVerdict))
	builder.WriteString(fmt.Sprintf("- full run log: `%s`\n", artifact.Summary.Logs.RunLog))

	return builder.String()
}

func migrationJudgeHighlight(summary *migrationDemoRunSummary, unsafeComparison *traceComparisonReport) string {
	highlight := "CMDR captured an approved database migration through agentgateway, replayed the same task against a frozen MCP environment, and blocked an unsafe candidate when it diverged from the approved baseline."
	if unsafeComparison != nil {
		if divergence := formatFirstDivergence(toStructuredDivergence(unsafeComparison.Report.FirstDivergence)); divergence != "" {
			highlight = fmt.Sprintf("%s In the failing replay, the first divergence was %s.", highlight, divergence)
		}
	}
	if summary != nil && summary.BaselineTraceID != "" {
		highlight = fmt.Sprintf("%s Baseline trace: `%s`.", highlight, summary.BaselineTraceID)
	}
	return highlight
}

func writeMigrationVerdictMarkdown(builder *strings.Builder, verdict *migrationDemoVerdictInfo) {
	builder.WriteString(fmt.Sprintf("### %s\n\n", verdict.Label))
	builder.WriteString(fmt.Sprintf("- Trace: `%s`\n", verdict.TraceID))
	builder.WriteString(fmt.Sprintf("- Verdict: `%s`\n", verdictDisplay(verdict.Comparison.Report.Verdict)))
	builder.WriteString(fmt.Sprintf("- Similarity: `%.4f`\n", verdict.Comparison.Report.SimilarityScore))
	if verdict.FirstDivergence != "" {
		builder.WriteString(fmt.Sprintf("- First divergence: `%s`\n", verdict.FirstDivergence))
	}
	builder.WriteString(fmt.Sprintf("- Steps: baseline=%d candidate=%d\n", verdict.Comparison.BaselineStepCount, verdict.Comparison.CandidateStepCount))
	builder.WriteString(fmt.Sprintf("- Tools: baseline=%d candidate=%d (%s)\n\n",
		verdict.Comparison.BaselineToolCount,
		verdict.Comparison.CandidateToolCount,
		displayTraceToolSource(verdict.Comparison.CandidateToolSource),
	))
}

func restoreEnv(key, value string, hadValue bool) {
	if hadValue {
		_ = os.Setenv(key, value)
		return
	}
	_ = os.Unsetenv(key)
}
