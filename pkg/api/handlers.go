package api

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/lethaltrifecta/replay/pkg/diff"
	"github.com/lethaltrifecta/replay/pkg/replay"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// --- Request / Response types ---

type gateCheckRequest struct {
	BaselineTraceID string            `json:"baseline_trace_id"`
	Model           string            `json:"model"`
	Provider        string            `json:"provider"`
	Threshold       *float64          `json:"threshold"`
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
}

type gateCheckResponse struct {
	ExperimentID string `json:"experiment_id"`
	Status       string `json:"status"`
}

type gateStatusResponse struct {
	ExperimentID string  `json:"experiment_id"`
	Status       string  `json:"status"`
	Progress     float64 `json:"progress"`
}

type gateReportResponse struct {
	ExperimentID    string                  `json:"experiment_id"`
	Status          string                  `json:"status"`
	BaselineTraceID string                  `json:"baseline_trace_id"`
	Verdict         string                  `json:"verdict,omitempty"`
	SimilarityScore *float64                `json:"similarity_score,omitempty"`
	TokenDelta      *int                    `json:"token_delta,omitempty"`
	LatencyDelta    *int                    `json:"latency_delta,omitempty"`
	Analysis        *storage.AnalysisResult `json:"analysis,omitempty"`
	Runs            []*storage.ExperimentRun `json:"runs,omitempty"`
}

// --- Handlers ---

// allowedRequestHeaders is the set of headers that the gate check API accepts.
// Headers not in this set are silently dropped to prevent sensitive values
// (e.g. auth tokens) from being persisted in variant_config and echoed in reports.
var allowedRequestHeaders = map[string]bool{
	http.CanonicalHeaderKey("X-Freeze-Trace-ID"):   true,
	http.CanonicalHeaderKey("X-Freeze-Span-ID"):    true,
	http.CanonicalHeaderKey("X-Freeze-Step-Index"): true,
}

// sanitizeRequestHeaders canonicalizes keys and filters to the allowlist.
func sanitizeRequestHeaders(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	sanitized := make(map[string]string, len(raw))
	for k, v := range raw {
		canonical := http.CanonicalHeaderKey(k)
		if allowedRequestHeaders[canonical] {
			sanitized[canonical] = v
		}
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func (s *Server) handleGateCheck(w http.ResponseWriter, r *http.Request) {
	var req gateCheckRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.BaselineTraceID == "" {
		writeError(w, http.StatusBadRequest, "baseline_trace_id is required")
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}
	if req.Threshold == nil {
		defaultThreshold := 0.8
		req.Threshold = &defaultThreshold
	}
	if *req.Threshold < 0 || *req.Threshold > 1 {
		writeError(w, http.StatusBadRequest, "threshold must be between 0.0 and 1.0")
		return
	}

	// Acquire concurrency semaphore (non-blocking)
	select {
	case s.sem <- struct{}{}:
		// acquired
	default:
		writeError(w, http.StatusServiceUnavailable, "server at replay capacity, try again later")
		return
	}

	// Build engine and run Setup synchronously to get experiment ID
	engine := replay.NewEngine(s.store, s.completer)
	variant := replay.VariantConfig{
		Model:          req.Model,
		Provider:       req.Provider,
		RequestHeaders: sanitizeRequestHeaders(req.RequestHeaders),
	}

	prepared, err := engine.Setup(r.Context(), req.BaselineTraceID, variant)
	if err != nil {
		<-s.sem // release
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return 202 with experiment ID immediately
	writeJSON(w, http.StatusAccepted, gateCheckResponse{
		ExperimentID: prepared.ExperimentID.String(),
		Status:       storage.StatusRunning,
	})

	// Run the pipeline in the background
	go func() {
		defer func() { <-s.sem }()
		RunGatePipeline(s.ctx, s.store, engine, prepared, *req.Threshold, s.log)
	}()
}

func (s *Server) handleGateStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	experimentID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid experiment ID")
		return
	}

	exp, err := s.store.GetExperiment(r.Context(), experimentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "experiment not found")
		return
	}

	writeJSON(w, http.StatusOK, gateStatusResponse{
		ExperimentID: exp.ID.String(),
		Status:       exp.Status,
		Progress:     exp.Progress,
	})
}

func (s *Server) handleGateReport(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	experimentID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid experiment ID")
		return
	}

	exp, err := s.store.GetExperiment(r.Context(), experimentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "experiment not found")
		return
	}

	runs, err := s.store.ListExperimentRuns(r.Context(), experimentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}

	resp := gateReportResponse{
		ExperimentID:    exp.ID.String(),
		Status:          exp.Status,
		BaselineTraceID: exp.BaselineTraceID,
		Runs:            runs,
	}

	// If completed, attach analysis results
	if exp.Status == storage.StatusCompleted || exp.Status == storage.StatusFailed {
		results, err := s.store.GetAnalysisResults(r.Context(), experimentID)
		if err == nil && len(results) > 0 {
			ar := results[0]
			resp.Analysis = ar
			resp.SimilarityScore = &ar.SimilarityScore
			resp.TokenDelta = &ar.TokenDelta
			resp.LatencyDelta = &ar.LatencyDelta
			if verdict, ok := ar.BehaviorDiff["verdict"].(string); ok {
				resp.Verdict = verdict
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	if err := s.store.Ping(r.Context()); err != nil {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

// resolveToolInputs decides whether semantic tool dimensions can be used in the diff.
func resolveToolInputs(captures []*storage.ToolCapture, captureErr error, variantSteps []*storage.ReplayTrace) ([]*storage.ToolCapture, []diff.ToolCall) {
	if captureErr != nil {
		return nil, nil
	}
	return captures, diff.ExtractVariantToolCalls(variantSteps)
}
