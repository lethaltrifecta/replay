package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultMigrationDemoPostgresURL = "postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable"

type migrationDemoRunSummary struct {
	Scenario            string                `json:"scenario"`
	BaselineTraceID     string                `json:"baseline_trace_id"`
	SafeReplayTraceID   string                `json:"safe_replay_trace_id"`
	UnsafeReplayTraceID string                `json:"unsafe_replay_trace_id"`
	Logs                migrationDemoLogPaths `json:"logs"`
}

type migrationDemoLogPaths struct {
	RunLog              string `json:"run_log"`
	CMDR                string `json:"cmdr"`
	FreezeMCP           string `json:"freeze_mcp"`
	MigrationMCP        string `json:"migration_mcp"`
	MockLLM             string `json:"mock_llm"`
	CaptureAgentgateway string `json:"capture_agentgateway"`
	ReplayAgentgateway  string `json:"replay_agentgateway"`
	SafeVerdict         string `json:"safe_verdict"`
	UnsafeVerdict       string `json:"unsafe_verdict"`
}

type migrationDemoReportArtifact struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	ReportDir   string                    `json:"report_dir"`
	Summary     *migrationDemoRunSummary  `json:"summary"`
	Safe        *migrationDemoVerdictInfo `json:"safe"`
	Unsafe      *migrationDemoVerdictInfo `json:"unsafe"`
}

type migrationDemoVerdictInfo struct {
	Label           string                 `json:"label"`
	TraceID         string                 `json:"trace_id"`
	FirstDivergence string                 `json:"first_divergence,omitempty"`
	Comparison      *traceComparisonReport `json:"comparison"`
}

func runMigrationDemo(cmd *cobra.Command, args []string) error {
	threshold, _ := cmd.Flags().GetFloat64("threshold")
	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("--threshold must be between 0.0 and 1.0, got %.2f", threshold)
	}

	repoRoot, err := findRepoRootForDemo()
	if err != nil {
		return err
	}

	reportDir, err := resolveMigrationDemoReportDir(cmd, repoRoot)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}

	runLogPath := filepath.Join(reportDir, "run.log")
	runLog, err := os.Create(runLogPath)
	if err != nil {
		return fmt.Errorf("create run log: %w", err)
	}
	defer runLog.Close()

	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}

	summaryPath := filepath.Join(reportDir, "run-summary.json")

	runCmd := exec.Command(filepath.Join(repoRoot, "scripts", "test-migration-demo-full-loop.sh"))
	runCmd.Dir = repoRoot
	runCmd.Env = buildMigrationDemoEnv(cmd, reportDir, executablePath, summaryPath)
	runCmd.Stdout = io.MultiWriter(cmd.OutOrStdout(), runLog)
	runCmd.Stderr = io.MultiWriter(cmd.ErrOrStderr(), runLog)

	cmd.Printf("Running migration demo harness...\n")
	cmd.Printf("Artifacts: %s\n\n", reportDir)

	if err := runCmd.Run(); err != nil {
		return fmt.Errorf("migration demo harness failed (see %s): %w", runLogPath, err)
	}

	summary, err := loadMigrationDemoRunSummary(summaryPath)
	if err != nil {
		return err
	}
	if summary.Logs.RunLog == "" {
		summary.Logs.RunLog = runLogPath
	}

	previousPostgresURL, hadPostgresURL := os.LookupEnv("CMDR_POSTGRES_URL")
	postgresURL := resolveMigrationDemoPostgresURL()
	if err := os.Setenv("CMDR_POSTGRES_URL", postgresURL); err != nil {
		return fmt.Errorf("set CMDR_POSTGRES_URL: %w", err)
	}
	defer restoreEnv("CMDR_POSTGRES_URL", previousPostgresURL, hadPostgresURL)

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	safeComparison, err := buildTraceComparisonReport(ctx, store, summary.BaselineTraceID, summary.SafeReplayTraceID, threshold)
	if err != nil {
		return fmt.Errorf("build safe comparison: %w", err)
	}
	unsafeComparison, err := buildTraceComparisonReport(ctx, store, summary.BaselineTraceID, summary.UnsafeReplayTraceID, threshold)
	if err != nil {
		return fmt.Errorf("build unsafe comparison: %w", err)
	}

	artifact := &migrationDemoReportArtifact{
		GeneratedAt: time.Now().UTC(),
		ReportDir:   reportDir,
		Summary:     summary,
		Safe: &migrationDemoVerdictInfo{
			Label:           "safe-replay",
			TraceID:         summary.SafeReplayTraceID,
			FirstDivergence: formatFirstDivergence(safeComparison.Report.FirstDivergence),
			Comparison:      safeComparison,
		},
		Unsafe: &migrationDemoVerdictInfo{
			Label:           "unsafe-replay",
			TraceID:         summary.UnsafeReplayTraceID,
			FirstDivergence: formatFirstDivergence(unsafeComparison.Report.FirstDivergence),
			Comparison:      unsafeComparison,
		},
	}

	jsonArtifactPath := filepath.Join(reportDir, "report.json")
	if err := writeJSONFile(jsonArtifactPath, artifact); err != nil {
		return err
	}

	markdownArtifactPath := filepath.Join(reportDir, "report.md")
	if err := os.WriteFile(markdownArtifactPath, []byte(renderMigrationDemoReportMarkdown(artifact)), 0o644); err != nil {
		return fmt.Errorf("write markdown report: %w", err)
	}

	cmd.Printf("\nSaved artifacts:\n")
	cmd.Printf("  Summary: %s\n", summaryPath)
	cmd.Printf("  JSON:    %s\n", jsonArtifactPath)
	cmd.Printf("  Markdown: %s\n", markdownArtifactPath)
	cmd.Printf("\nVerdicts:\n")
	cmd.Printf("  Safe replay:   %s (%.4f)\n", verdictDisplay(safeComparison.Report.Verdict), safeComparison.Report.SimilarityScore)
	cmd.Printf("  Unsafe replay: %s (%.4f)\n", verdictDisplay(unsafeComparison.Report.Verdict), unsafeComparison.Report.SimilarityScore)
	if artifact.Unsafe.FirstDivergence != "" {
		cmd.Printf("  Unsafe first divergence: %s\n", artifact.Unsafe.FirstDivergence)
	}

	return nil
}

func findRepoRootForDemo() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	return findRepoRoot(start)
}

func findRepoRoot(start string) (string, error) {
	current := start

	for {
		goMod := filepath.Join(current, "go.mod")
		harness := filepath.Join(current, "scripts", "test-migration-demo-full-loop.sh")
		if fileExists(goMod) && fileExists(harness) {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("could not find repo root from %s", start)
		}
		current = parent
	}
}

func resolveMigrationDemoReportDir(cmd *cobra.Command, repoRoot string) (string, error) {
	reportDir, _ := cmd.Flags().GetString("report-dir")
	if reportDir == "" {
		reportDir = filepath.Join(repoRoot, "artifacts", "migration-demo", time.Now().UTC().Format("20060102-150405"))
	}

	if !filepath.IsAbs(reportDir) {
		reportDir = filepath.Join(repoRoot, reportDir)
	}

	return filepath.Clean(reportDir), nil
}

func buildMigrationDemoEnv(cmd *cobra.Command, reportDir, executablePath, summaryPath string) []string {
	env := os.Environ()
	env = setEnvValue(env, "CMDR_BIN", executablePath)
	env = setEnvValue(env, "REPORT_SUMMARY_FILE", summaryPath)
	env = setEnvValue(env, "RUN_LOG_FILE", filepath.Join(reportDir, "run.log"))
	env = setEnvValue(env, "CMDR_LOG", filepath.Join(reportDir, "cmdr.log"))
	env = setEnvValue(env, "FREEZE_LOG", filepath.Join(reportDir, "freeze-mcp.log"))
	env = setEnvValue(env, "MCP_LOG", filepath.Join(reportDir, "migration-mcp.log"))
	env = setEnvValue(env, "LLM_LOG", filepath.Join(reportDir, "mock-llm.log"))
	env = setEnvValue(env, "CAPTURE_AGW_LOG", filepath.Join(reportDir, "agentgateway-capture.log"))
	env = setEnvValue(env, "REPLAY_AGW_LOG", filepath.Join(reportDir, "agentgateway-replay.log"))
	env = setEnvValue(env, "SAFE_VERDICT_LOG", filepath.Join(reportDir, "safe-verdict.log"))
	env = setEnvValue(env, "UNSAFE_VERDICT_LOG", filepath.Join(reportDir, "unsafe-verdict.log"))
	env = setEnvValue(env, "CMDR_POSTGRES_URL", resolveMigrationDemoPostgresURL())

	if pythonBin, _ := cmd.Flags().GetString("python-bin"); pythonBin != "" {
		env = setEnvValue(env, "PYTHON_BIN", pythonBin)
	}
	if goBin, _ := cmd.Flags().GetString("go-bin"); goBin != "" {
		env = setEnvValue(env, "GO_BIN", goBin)
	}
	if agentgatewayDir, _ := cmd.Flags().GetString("agentgateway-dir"); agentgatewayDir != "" {
		env = setEnvValue(env, "AGENTGATEWAY_DIR", agentgatewayDir)
	}
	if freezeDir, _ := cmd.Flags().GetString("freeze-dir"); freezeDir != "" {
		env = setEnvValue(env, "FREEZE_DIR", freezeDir)
	}

	return env
}

func resolveMigrationDemoPostgresURL() string {
	if value := strings.TrimSpace(os.Getenv("CMDR_POSTGRES_URL")); value != "" {
		return value
	}
	return defaultMigrationDemoPostgresURL
}

func loadMigrationDemoRunSummary(path string) (*migrationDemoRunSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read run summary: %w", err)
	}

	var summary migrationDemoRunSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("decode run summary: %w", err)
	}

	if summary.BaselineTraceID == "" || summary.SafeReplayTraceID == "" || summary.UnsafeReplayTraceID == "" {
		return nil, fmt.Errorf("run summary missing required trace IDs")
	}

	return &summary, nil
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

func renderMigrationDemoReportMarkdown(artifact *migrationDemoReportArtifact) string {
	var builder strings.Builder

	builder.WriteString("# Migration Demo Report\n\n")
	builder.WriteString(fmt.Sprintf("Generated: `%s`\n\n", artifact.GeneratedAt.Format(time.RFC3339)))
	builder.WriteString("## Trace IDs\n\n")
	builder.WriteString(fmt.Sprintf("- Baseline: `%s`\n", artifact.Summary.BaselineTraceID))
	builder.WriteString(fmt.Sprintf("- Safe replay: `%s`\n", artifact.Summary.SafeReplayTraceID))
	builder.WriteString(fmt.Sprintf("- Unsafe replay: `%s`\n\n", artifact.Summary.UnsafeReplayTraceID))

	builder.WriteString("## Verdicts\n\n")
	writeMigrationVerdictMarkdown(&builder, artifact.Safe)
	writeMigrationVerdictMarkdown(&builder, artifact.Unsafe)

	builder.WriteString("## Logs\n\n")
	builder.WriteString(fmt.Sprintf("- Full run: `%s`\n", artifact.Summary.Logs.RunLog))
	builder.WriteString(fmt.Sprintf("- CMDR: `%s`\n", artifact.Summary.Logs.CMDR))
	builder.WriteString(fmt.Sprintf("- freeze-mcp: `%s`\n", artifact.Summary.Logs.FreezeMCP))
	builder.WriteString(fmt.Sprintf("- migration MCP: `%s`\n", artifact.Summary.Logs.MigrationMCP))
	builder.WriteString(fmt.Sprintf("- mock LLM: `%s`\n", artifact.Summary.Logs.MockLLM))
	builder.WriteString(fmt.Sprintf("- capture agentgateway: `%s`\n", artifact.Summary.Logs.CaptureAgentgateway))
	builder.WriteString(fmt.Sprintf("- replay agentgateway: `%s`\n", artifact.Summary.Logs.ReplayAgentgateway))
	builder.WriteString(fmt.Sprintf("- safe verdict: `%s`\n", artifact.Summary.Logs.SafeVerdict))
	builder.WriteString(fmt.Sprintf("- unsafe verdict: `%s`\n", artifact.Summary.Logs.UnsafeVerdict))

	return builder.String()
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

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func setEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}

	return append(env, prefix+value)
}

func restoreEnv(key, value string, hadValue bool) {
	if hadValue {
		_ = os.Setenv(key, value)
		return
	}
	_ = os.Unsetenv(key)
}
