package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
	"github.com/lethaltrifecta/replay/pkg/api"
	"github.com/lethaltrifecta/replay/pkg/config"
	"github.com/lethaltrifecta/replay/pkg/diff"
	"github.com/lethaltrifecta/replay/pkg/replay"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

var gateCmd = &cobra.Command{
	Use:   "gate",
	Short: "Deployment gate commands",
	Long:  `Replay baseline traces with a different model and produce a pass/fail verdict`,
}

var gateCheckCmd = &cobra.Command{
	Use:          "check",
	Short:        "Replay a baseline trace and diff the results",
	Long:         `Sends each baseline prompt to a variant model via agentgateway, diffs the results, and produces a CI/CD-friendly exit code (0=pass, 1=fail).`,
	RunE:         runGateCheck,
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
	gateCheckCmd.Flags().String("freeze-trace-id", "", "Replay header convenience flag for X-Freeze-Trace-ID")
	gateCheckCmd.Flags().StringArray("request-header", nil, "Additional replay request header in KEY=VALUE form (repeatable)")
	gateCheckCmd.Flags().Float64("threshold", 0.8, "Similarity threshold for pass verdict")
	gateCheckCmd.Flags().String("server", "", "CMDR server URL for remote execution (e.g. http://localhost:8080)")
	gateCheckCmd.Flags().Int("max-turns", 0, "Maximum agent loop turns (overrides CMDR_AGENT_LOOP_MAX_TURNS)")
	_ = gateCheckCmd.MarkFlagRequired("baseline")
	_ = gateCheckCmd.MarkFlagRequired("model")

	gateReportCmd.Flags().String("server", "", "CMDR server URL for remote execution")
	gateReportCmd.Flags().String("model", "", "Variant model name (for display in remote report)")
}

func runGateCheck(cmd *cobra.Command, args []string) error {
	baselineTraceID, _ := cmd.Flags().GetString("baseline")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")
	freezeTraceID, _ := cmd.Flags().GetString("freeze-trace-id")
	requestHeaderSpecs, _ := cmd.Flags().GetStringArray("request-header")
	threshold, _ := cmd.Flags().GetFloat64("threshold")
	server, _ := cmd.Flags().GetString("server")
	maxTurns, _ := cmd.Flags().GetInt("max-turns")

	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("--threshold must be between 0.0 and 1.0, got %.2f", threshold)
	}

	requestHeaders, err := buildReplayHeaders(freezeTraceID, requestHeaderSpecs)
	if err != nil {
		return err
	}

	if server != "" {
		return runGateCheckRemote(cmd, server, baselineTraceID, model, provider, requestHeaders, threshold, maxTurns)
	}

	return runGateCheckLocal(cmd, baselineTraceID, model, provider, requestHeaders, threshold, maxTurns)
}

func runGateCheckLocal(cmd *cobra.Command, baselineTraceID, model, provider string, requestHeaders map[string]string, threshold float64, maxTurnsOverride int) error {
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

	ctx := commandContext(cmd)

	// Create agentgateway client
	client := agwclient.NewClient(agwclient.ClientConfig{
		BaseURL:    cfg.AgentgatewayURL,
		Timeout:    cfg.AgentgatewayTimeout,
		MaxRetries: cfg.AgentgatewayRetries,
	})

	// Build variant config
	variant := replay.VariantConfig{
		Model:          model,
		Provider:       provider,
		RequestHeaders: api.SanitizeRequestHeaders(requestHeaders),
	}

	// Run replay
	engine := replay.NewEngine(store, client)
	prepared, err := engine.Setup(ctx, baselineTraceID, variant, threshold)
	if err != nil {
		return fmt.Errorf("prepare replay: %w", err)
	}

	// Determine replay mode: agent loop vs prompt-only
	var result *replay.Result
	var agentLoopResult *replay.AgentLoopResult

	if cfg.MCPURL != "" {
		// Resolve max turns: flag > config > default
		agentMaxTurns := cfg.AgentLoopMaxTurns
		if maxTurnsOverride > 0 {
			agentMaxTurns = maxTurnsOverride
		}

		// Extract freeze headers for MCP session
		freezeHeaders := api.FreezeHeaders(requestHeaders, baselineTraceID)

		cmd.Printf("Connecting to freeze-mcp at %s...\n", cfg.MCPURL)
		toolExec, mcpErr := replay.NewMCPToolExecutor(ctx, cfg.MCPURL, freezeHeaders)
		if mcpErr != nil {
			cmd.PrintErrf("Warning: MCP connection failed (%v), falling back to prompt-only replay\n", mcpErr)
		} else {
			defer func() { _ = toolExec.Close() }()
			cmd.Printf("Agent loop: replaying baseline %s with model %s (max %d turns)...\n", baselineTraceID, model, agentMaxTurns)
			loopResult, loopErr := engine.ExecuteAgentLoop(ctx, prepared, toolExec, replay.AgentLoopConfig{MaxTurns: agentMaxTurns})
			if loopErr != nil {
				return fmt.Errorf("agent loop failed: %w", loopErr)
			}
			agentLoopResult = loopResult
			result = &loopResult.Result
		}
	}

	// Prompt-only fallback
	if result == nil {
		cmd.Printf("Prompt-only: replaying baseline %s with model %s...\n", baselineTraceID, model)
		promptResult, err := engine.Execute(ctx, prepared)
		if err != nil {
			return fmt.Errorf("replay failed: %w", err)
		}
		result = promptResult
	}

	// Reload traces for diff comparison
	baselineSteps, err := store.GetReplayTraceSpans(ctx, baselineTraceID)
	if err != nil {
		return failPreparedRun(engine, prepared, "reload baseline", err)
	}

	variantSteps, err := store.GetReplayTraceSpans(ctx, result.VariantTraceID)
	if err != nil {
		return failPreparedRun(engine, prepared, "reload variant", err)
	}

	captures, err := store.GetToolCapturesByTrace(ctx, baselineTraceID)
	baselineCaptures, variantToolCalls := resolveToolComparisonInputs(cmd, captures, err, variantSteps)

	// Diff (6-dimension when tool data available, 4-dimension fallback otherwise)
	diffCfg := diff.Config{SimilarityThreshold: threshold}
	report := diff.CompareAll(diff.CompareInput{
		Baseline:      baselineSteps,
		Variant:       variantSteps,
		BaselineTools: baselineCaptures,
		VariantTools:  variantToolCalls,
	}, diffCfg)

	// Persist analysis result
	analysisResult := diff.ToAnalysisResult(report, result.ExperimentID, result.BaselineRunID, result.VariantRunID)
	if err := store.CreateAnalysisResult(ctx, analysisResult); err != nil {
		return failPreparedRun(engine, prepared, "persist analysis", err)
	}

	// Print summary
	printGateReport(cmd, baselineTraceID, model, result.ExperimentID, report)

	// Print agent loop summary if applicable
	if agentLoopResult != nil {
		cmd.Printf("\nAgent Loop Summary:\n")
		cmd.Printf("  Turns:       %d\n", agentLoopResult.TurnsExecuted)
		cmd.Printf("  Stop Reason: %s\n", agentLoopResult.StopReason)
		if len(agentLoopResult.Divergences) > 0 {
			cmd.Printf("  Divergences: %d\n", len(agentLoopResult.Divergences))
			for _, d := range agentLoopResult.Divergences {
				cmd.Printf("    turn %d: %s (%s)\n", d.Turn, d.ToolName, d.ErrorType)
			}
		}
	}

	if report.Verdict == "fail" {
		return ErrGateFailed
	}

	return nil
}

// resolveToolComparisonInputs decides whether semantic tool dimensions can be used.
func resolveToolComparisonInputs(
	cmd *cobra.Command,
	captures []*storage.ToolCapture,
	captureErr error,
	variantSteps []*storage.ReplayTrace,
) ([]*storage.ToolCapture, []diff.ToolCall) {
	if captureErr != nil {
		cmd.PrintErrf("Warning: failed to load baseline tool captures, falling back to 4-dimension structural+response diff: %v\n", captureErr)
		return nil, nil
	}

	return captures, diff.ExtractVariantToolCalls(variantSteps)
}

func failPreparedRun(engine *replay.Engine, prepared *replay.PreparedRun, operation string, err error) error {
	runErr := fmt.Errorf("%s: %w", operation, err)
	if cleanupErr := engine.FailPreparedRun(prepared, runErr); cleanupErr != nil {
		return fmt.Errorf("%w (cleanup: %v)", runErr, cleanupErr)
	}
	return runErr
}

// ErrGateFailed is returned when the gate check verdict is "fail".
var ErrGateFailed = errors.New("gate check failed")

func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func runGateReport(cmd *cobra.Command, args []string) error {
	experimentID, err := uuid.Parse(args[0])
	if err != nil {
		return fmt.Errorf("invalid experiment ID: %w", err)
	}

	server, _ := cmd.Flags().GetString("server")
	if server != "" {
		model, _ := cmd.Flags().GetString("model")
		client, err := newRemoteAPIClient(server)
		if err != nil {
			return err
		}
		return fetchAndPrintRemoteReport(commandContext(cmd), client, cmd, openapiClientUUID(experimentID), model)
	}

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := commandContext(cmd)

	exp, err := store.GetExperiment(ctx, experimentID)
	if err != nil {
		if errors.Is(err, storage.ErrExperimentNotFound) {
			return fmt.Errorf("experiment not found: %w", err)
		}
		return fmt.Errorf("failed to load experiment: %w", err)
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

			if r.BehaviorDiff.Verdict != "" {
				cmd.Printf("  Verdict: %s\n", verdictDisplay(r.BehaviorDiff.Verdict))
			}
			if summary := formatFirstDivergence(r.FirstDivergence); summary != "" {
				cmd.Printf("  First Divergence: %s\n", summary)
			}
		}
	} else {
		cmd.Printf("\nNo analysis results found.\n")
		for _, run := range runs {
			if run.Status == storage.StatusFailed && run.Error != nil {
				return fmt.Errorf("experiment failed: %s", *run.Error)
			}
		}
		return fmt.Errorf("analysis results not available for experiment status=%s", exp.Status)
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

	// Dimension breakdown
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
	if summary := formatFirstDivergence(toStructuredDivergence(report.FirstDivergence)); summary != "" {
		cmd.Printf("\nFirst Divergence:\n")
		cmd.Printf("  %s\n", summary)
	}

	if len(report.StepDiffs) > 0 {
		cmd.Printf("\nStep Breakdown:\n")
		cmd.Printf("%s", formatStepBreakdown(report.StepDiffs))
	}

	cmd.Printf("\nTotals: token_delta=%+d  latency_delta=%+dms\n", report.TokenDelta, report.LatencyDelta)
	cmd.Printf("Experiment: %s\n", experimentID)
}

func buildReplayHeaders(freezeTraceID string, headerSpecs []string) (map[string]string, error) {
	headers := map[string]string{}
	if freezeTraceID != "" {
		headers[http.CanonicalHeaderKey("X-Freeze-Trace-ID")] = freezeTraceID
	}

	for _, spec := range headerSpecs {
		key, value, ok := strings.Cut(spec, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --request-header %q: expected KEY=VALUE", spec)
		}
		canonical := http.CanonicalHeaderKey(key)
		if _, exists := headers[canonical]; exists {
			return nil, fmt.Errorf("duplicate request header %q (case-insensitive collision)", key)
		}
		headers[canonical] = value
	}

	if len(headers) == 0 {
		return nil, nil
	}

	return headers, nil
}

func formatStepBreakdown(stepDiffs []diff.StepDiff) string {
	var builder strings.Builder

	w := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  STEP\tTOKEN DELTA")
	for _, sd := range stepDiffs {
		fmt.Fprintf(w, "  %d\t%+d\n", sd.StepIndex, sd.TokenDelta)
	}
	_ = w.Flush()

	return builder.String()
}
