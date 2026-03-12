package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
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

	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("--threshold must be between 0.0 and 1.0, got %.2f", threshold)
	}

	requestHeaders, err := buildReplayHeaders(freezeTraceID, requestHeaderSpecs)
	if err != nil {
		return err
	}

	if server != "" {
		return runGateCheckRemote(cmd, server, baselineTraceID, model, provider, requestHeaders, threshold)
	}

	return runGateCheckLocal(cmd, baselineTraceID, model, provider, requestHeaders, threshold)
}

func runGateCheckLocal(cmd *cobra.Command, baselineTraceID, model, provider string, requestHeaders map[string]string, threshold float64) error {
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
		Model:          model,
		Provider:       provider,
		RequestHeaders: requestHeaders,
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
		cmd.PrintErrf("Warning: failed to persist analysis result: %v\n", err)
	}

	// Print summary
	printGateReport(cmd, baselineTraceID, model, result.ExperimentID, report)

	if report.Verdict == "fail" {
		return ErrGateFailed
	}

	return nil
}

// resolveToolComparisonInputs decides whether semantic tool dimensions can be used.
// If baseline tool captures cannot be loaded, both sides are nil so CompareAll
// falls back to 4-dimension structural+response scoring.
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

// ErrGateFailed is returned when the gate check verdict is "fail".
// Callers should exit with code 1 when they see this error.
var ErrGateFailed = errors.New("gate check failed")

// --- Remote execution via HTTP API ---

type remoteCheckRequest struct {
	BaselineTraceID string            `json:"baseline_trace_id"`
	Model           string            `json:"model"`
	Provider        string            `json:"provider,omitempty"`
	Threshold       float64           `json:"threshold"`
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
}

type remoteCheckResponse struct {
	ExperimentID string `json:"experiment_id"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

type remoteStatusResponse struct {
	ExperimentID string  `json:"experiment_id"`
	Status       string  `json:"status"`
	Progress     float64 `json:"progress"`
	Error        string  `json:"error,omitempty"`
}

type remoteReportResponse struct {
	ExperimentID    string   `json:"experiment_id"`
	Status          string   `json:"status"`
	BaselineTraceID string   `json:"baseline_trace_id"`
	Verdict         string   `json:"verdict,omitempty"`
	SimilarityScore *float64 `json:"similarity_score,omitempty"`
	TokenDelta      *int     `json:"token_delta,omitempty"`
	LatencyDelta    *int     `json:"latency_delta,omitempty"`
	Error           string   `json:"error,omitempty"`
}

func runGateCheckRemote(cmd *cobra.Command, server, baselineTraceID, model, provider string, requestHeaders map[string]string, threshold float64) error {
	serverURL := strings.TrimRight(server, "/")

	// Submit gate check
	reqBody := remoteCheckRequest{
		BaselineTraceID: baselineTraceID,
		Model:           model,
		Provider:        provider,
		Threshold:       threshold,
		RequestHeaders:  requestHeaders,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	cmd.Printf("Submitting gate check to %s...\n", server)

	resp, err := http.Post(serverURL+"/api/v1/gate/check", "application/json", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("submit gate check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var checkResp remoteCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&checkResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	cmd.Printf("Experiment %s submitted, polling for results...\n", checkResp.ExperimentID)

	// Poll for completion
	for {
		time.Sleep(2 * time.Second)

		statusResp, err := http.Get(serverURL + "/api/v1/gate/status/" + checkResp.ExperimentID)
		if err != nil {
			return fmt.Errorf("poll status: %w", err)
		}

		if statusResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(statusResp.Body)
			statusResp.Body.Close()
			return fmt.Errorf("status endpoint returned %d: %s", statusResp.StatusCode, string(body))
		}

		var status remoteStatusResponse
		if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
			statusResp.Body.Close()
			return fmt.Errorf("decode status: %w", err)
		}
		statusResp.Body.Close()

		switch status.Status {
		case storage.StatusRunning:
			cmd.Printf("  Progress: %.0f%%\n", status.Progress*100)
			continue
		case storage.StatusCompleted, storage.StatusFailed:
			// Done — fetch full report
		default:
			cmd.Printf("  Status: %s\n", status.Status)
			continue
		}

		// Fetch and display report
		return fetchAndPrintRemoteReport(cmd, serverURL, checkResp.ExperimentID, model)
	}
}

func fetchAndPrintRemoteReport(cmd *cobra.Command, serverURL, experimentID, model string) error {
	reportResp, err := http.Get(serverURL + "/api/v1/gate/report/" + experimentID)
	if err != nil {
		return fmt.Errorf("fetch report: %w", err)
	}
	defer reportResp.Body.Close()

	if reportResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(reportResp.Body)
		return fmt.Errorf("report endpoint returned %d: %s", reportResp.StatusCode, string(body))
	}

	var report remoteReportResponse
	if err := json.NewDecoder(reportResp.Body).Decode(&report); err != nil {
		return fmt.Errorf("decode report: %w", err)
	}

	cmd.Printf("\nGate Check Result\n")
	cmd.Printf("=================\n")
	cmd.Printf("Baseline:   %s\n", report.BaselineTraceID)
	cmd.Printf("Variant:    %s\n", model)
	cmd.Printf("Status:     %s\n", report.Status)

	if report.SimilarityScore != nil {
		cmd.Printf("Similarity: %.4f\n", *report.SimilarityScore)
	}
	if report.Verdict != "" {
		cmd.Printf("Verdict:    %s\n", verdictDisplay(report.Verdict))
	}
	if report.TokenDelta != nil {
		cmd.Printf("Token Delta: %+d\n", *report.TokenDelta)
	}
	if report.LatencyDelta != nil {
		cmd.Printf("Latency Delta: %+dms\n", *report.LatencyDelta)
	}
	cmd.Printf("Experiment: %s\n", experimentID)

	if report.Verdict == "fail" {
		return ErrGateFailed
	}

	// If experiment failed but no verdict was produced, that's an error
	if report.Status == storage.StatusFailed {
		if report.Error != "" {
			return fmt.Errorf("experiment failed: %s", report.Error)
		}
		return fmt.Errorf("experiment failed without producing a verdict")
	}

	return nil
}

func runGateReport(cmd *cobra.Command, args []string) error {
	experimentID, err := uuid.Parse(args[0])
	if err != nil {
		return fmt.Errorf("invalid experiment ID: %w", err)
	}

	server, _ := cmd.Flags().GetString("server")
	if server != "" {
		model, _ := cmd.Flags().GetString("model")
		return fetchAndPrintRemoteReport(cmd, strings.TrimRight(server, "/"), experimentID.String(), model)
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
			if summary := formatFirstDivergence(r.FirstDivergence); summary != "" {
				cmd.Printf("  First Divergence: %s\n", summary)
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
	if summary := formatFirstDivergence(report.FirstDivergence); summary != "" {
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

func formatFirstDivergence(divergence storage.JSONB) string {
	if len(divergence) == 0 {
		return ""
	}

	switch divergenceType, _ := divergence["type"].(string); divergenceType {
	case "tool_sequence":
		toolIndex, _ := intValue(divergence["tool_index"])
		baseline, _ := divergence["baseline"].(string)
		variant, _ := divergence["variant"].(string)
		return fmt.Sprintf("tool #%d changed: baseline=%q variant=%q", toolIndex, baseline, variant)
	case "tool_count":
		toolIndex, _ := intValue(divergence["tool_index"])
		baselineCount, _ := intValue(divergence["baseline_count"])
		variantCount, _ := intValue(divergence["variant_count"])
		return fmt.Sprintf("tool count diverged at tool #%d: baseline=%d variant=%d", toolIndex, baselineCount, variantCount)
	case "response_content":
		stepIndex, _ := intValue(divergence["step_index"])
		jaccard, _ := floatValue(divergence["jaccard"])
		baselineExcerpt, _ := divergence["baseline_excerpt"].(string)
		variantExcerpt, _ := divergence["variant_excerpt"].(string)
		return fmt.Sprintf("step %d response changed (jaccard=%.2f): baseline=%q variant=%q", stepIndex, jaccard, baselineExcerpt, variantExcerpt)
	case "step_count":
		stepIndex, _ := intValue(divergence["step_index"])
		baselineSteps, _ := intValue(divergence["baseline_steps"])
		variantSteps, _ := intValue(divergence["variant_steps"])
		return fmt.Sprintf("step count diverged at step %d: baseline=%d variant=%d", stepIndex, baselineSteps, variantSteps)
	default:
		return fmt.Sprintf("%v", divergence)
	}
}

func intValue(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func floatValue(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
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
