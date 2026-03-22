package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/lethaltrifecta/replay/pkg/replay"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// ensure Server implements ServerInterface
var _ ServerInterface = (*Server)(nil)

// --- Conversion Helpers ---

// mapToStruct strictly converts a map (usually storage.JSONB) to a target struct.
// Returns a 500-level error if conversion fails, as this indicates a contract/schema drift.
func (s *Server) mapToStruct(w http.ResponseWriter, m any, target any, context string) bool {
	if m == nil {
		return true
	}
	b, err := json.Marshal(m)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to marshal %s: %v", context, err), "MARSHAL_ERROR")
		return false
	}
	if err := json.Unmarshal(b, target); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to unmarshal %s: %v", context, err), "SCHEMA_DRIFT")
		return false
	}
	return true
}

// --- Governance Handlers ---

func (s *Server) ListBaselines(w http.ResponseWriter, r *http.Request) {
	baselines, err := s.store.ListBaselines(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list baselines", "DB_ERROR")
		return
	}

	resp := make([]Baseline, 0, len(baselines))
	for _, b := range baselines {
		createdAt := b.CreatedAt
		resp = append(resp, Baseline{
			TraceId:     &b.TraceID,
			Name:        b.Name,
			Description: b.Description,
			CreatedAt:   &createdAt,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) CreateBaseline(w http.ResponseWriter, r *http.Request, traceId string) {
	var body CreateBaselineJSONRequestBody
	if err := readOptionalJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_BODY")
		return
	}

	baseline := &storage.Baseline{
		TraceID:     traceId,
		Name:        body.Name,
		Description: body.Description,
	}

	if err := s.store.MarkTraceAsBaseline(r.Context(), baseline); err != nil {
		if errors.Is(err, storage.ErrTraceNotFound) {
			writeError(w, http.StatusNotFound, err.Error(), "TRACE_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create baseline", "DB_ERROR")
		return
	}

	stored, err := s.store.GetBaseline(r.Context(), traceId)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "baseline created but failed to retrieve", "DB_ERROR")
		return
	}

	resp := Baseline{
		TraceId:     &stored.TraceID,
		Name:        stored.Name,
		Description: stored.Description,
		CreatedAt:   &stored.CreatedAt,
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) DeleteBaseline(w http.ResponseWriter, r *http.Request, traceId string) {
	if err := s.store.UnmarkBaseline(r.Context(), traceId); err != nil {
		if errors.Is(err, storage.ErrBaselineNotFound) {
			writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete baseline", "DB_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ListDriftResults(w http.ResponseWriter, r *http.Request, params ListDriftResultsParams) {
	limit := clampLimit(params.Limit, defaultListLimit, maxListLimit)
	offset := clampOffset(params.Offset)

	results, err := s.store.ListDriftResults(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list drift results", "DB_ERROR")
		return
	}

	resp := make([]DriftResult, 0, len(results))
	for _, res := range results {
		resp = append(resp, driftResultResponse(res))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) GetDriftResult(w http.ResponseWriter, r *http.Request, traceId string, params GetDriftResultParams) {
	if params.BaselineTraceId != nil && *params.BaselineTraceId != "" {
		result, err := s.store.GetDriftResultForPair(r.Context(), traceId, *params.BaselineTraceId)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusNotFound, "drift result not found", "NOT_FOUND")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to get drift result", "DB_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, driftResultResponse(result))
		return
	}

	results, err := s.store.GetDriftResults(r.Context(), traceId)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get drift results", "DB_ERROR")
		return
	}
	if len(results) == 0 {
		writeError(w, http.StatusNotFound, "drift result not found", "NOT_FOUND")
		return
	}
	if len(results) > 1 {
		writeError(w, http.StatusBadRequest, "multiple drift results found; provide baselineTraceId", "AMBIGUOUS_RESULT")
		return
	}

	writeJSON(w, http.StatusOK, driftResultResponse(results[0]))
}

// --- Experiments Handlers ---

func (s *Server) ListExperiments(w http.ResponseWriter, r *http.Request, params ListExperimentsParams) {
	filters := storage.ExperimentFilters{
		Limit:  defaultListLimit,
		Offset: 0,
	}
	filters.Limit = clampLimit(params.Limit, defaultListLimit, maxListLimit)
	filters.Offset = clampOffset(params.Offset)
	if params.Status != nil {
		st := string(*params.Status)
		filters.Status = &st
	}

	experiments, err := s.store.ListExperiments(r.Context(), filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list experiments", "DB_ERROR")
		return
	}

	resp := make([]Experiment, 0, len(experiments))
	for _, e := range experiments {
		id := openapi_types.UUID(e.ID)
		progress := float32(e.Progress)

		exp := Experiment{
			Id:              &id,
			Name:            &e.Name,
			BaselineTraceId: &e.BaselineTraceID,
			Status:          &e.Status,
			Progress:        &progress,
			CreatedAt:       &e.CreatedAt,
			CompletedAt:     e.CompletedAt,
		}
		if threshold := float32PtrFromFloat64(e.Config.Threshold); threshold != nil {
			exp.Threshold = threshold
		}
		if variantConfig := apiVariantConfigFromExperiment(e.Config); variantConfig != nil {
			exp.VariantConfig = variantConfig
		}

		if e.Status == storage.StatusCompleted || e.Status == storage.StatusFailed {
			results, err := s.store.GetAnalysisResults(r.Context(), e.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to fetch analysis results", "DB_ERROR")
				return
			}
			if latest := latestAnalysisResult(results); latest != nil {
				v := latest.BehaviorDiff.Verdict
				exp.Verdict = &v
			}
		}

		resp = append(resp, exp)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) GetExperiment(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	exp, err := s.store.GetExperiment(r.Context(), uuid.UUID(id))
	if err != nil {
		if errors.Is(err, storage.ErrExperimentNotFound) {
			writeError(w, http.StatusNotFound, "experiment not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get experiment", "DB_ERROR")
		return
	}

	runs, err := s.store.ListExperimentRuns(r.Context(), exp.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs", "DB_ERROR")
		return
	}

	oapiRuns := buildExperimentRuns(runs)

	oid := openapi_types.UUID(exp.ID)
	progress := float32(exp.Progress)

	resp := ExperimentDetail{}
	resp.Id = &oid
	resp.Name = &exp.Name
	resp.BaselineTraceId = &exp.BaselineTraceID
	resp.Status = &exp.Status
	resp.Progress = &progress
	resp.CreatedAt = &exp.CreatedAt
	resp.CompletedAt = exp.CompletedAt
	resp.Runs = &oapiRuns
	if threshold := float32PtrFromFloat64(exp.Config.Threshold); threshold != nil {
		resp.Threshold = threshold
	}
	if variantConfig := apiVariantConfigFromExperiment(exp.Config); variantConfig != nil {
		resp.VariantConfig = variantConfig
	}

	if exp.Status == storage.StatusCompleted || exp.Status == storage.StatusFailed {
		results, err := s.store.GetAnalysisResults(r.Context(), exp.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch analysis results", "DB_ERROR")
			return
		}
		if latest := latestAnalysisResult(results); latest != nil {
			v := latest.BehaviorDiff.Verdict
			resp.Verdict = &v
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) GetExperimentReport(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	exp, err := s.store.GetExperiment(r.Context(), uuid.UUID(id))
	if err != nil {
		if errors.Is(err, storage.ErrExperimentNotFound) {
			writeError(w, http.StatusNotFound, "experiment not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get experiment", "DB_ERROR")
		return
	}

	runs, err := s.store.ListExperimentRuns(r.Context(), exp.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs", "DB_ERROR")
		return
	}

	oapiRuns := buildExperimentRuns(runs)

	oid := openapi_types.UUID(exp.ID)
	resp := ExperimentReport{
		ExperimentId:    &oid,
		BaselineTraceId: &exp.BaselineTraceID,
		Status:          &exp.Status,
		Runs:            &oapiRuns,
	}

	if exp.Status == storage.StatusCompleted || exp.Status == storage.StatusFailed {
		results, err := s.store.GetAnalysisResults(r.Context(), exp.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get analysis results", "DB_ERROR")
			return
		}
		if ar := latestAnalysisResult(results); ar != nil {
			score := float32(ar.SimilarityScore)
			td := ar.TokenDelta
			ld := ar.LatencyDelta

			resp.SimilarityScore = &score
			resp.TokenDelta = &td
			resp.LatencyDelta = &ld

			resp.Analysis = &AnalysisResult{
				BehaviorDiff: &BehaviorDiff{
					Verdict: &ar.BehaviorDiff.Verdict,
					Reason:  &ar.BehaviorDiff.Reason,
				},
				FirstDivergence: &FirstDivergence{
					StepIndex: &ar.FirstDivergence.StepIndex,
					Type:      &ar.FirstDivergence.Type,
					Baseline:  &ar.FirstDivergence.Baseline,
					Variant:   &ar.FirstDivergence.Variant,
				},
				SafetyDiff: &SafetyDiff{
					RiskEscalation: &ar.SafetyDiff.RiskEscalation,
					BaselineRisk:   &ar.SafetyDiff.BaselineRisk,
					VariantRisk:    &ar.SafetyDiff.VariantRisk,
				},
			}

			resp.Verdict = &ar.BehaviorDiff.Verdict
		} else if exp.Status == storage.StatusFailed {
			for _, run := range runs {
				if run.Status == storage.StatusFailed && run.Error != nil {
					resp.Error = run.Error
					break
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- Gate Handlers ---

func (s *Server) CreateGateCheck(w http.ResponseWriter, r *http.Request) {
	var req GateCheckRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_REQUEST")
		return
	}

	if req.BaselineTraceId == "" {
		writeError(w, http.StatusBadRequest, "baselineTraceId is required", "MISSING_FIELD")
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required", "MISSING_FIELD")
		return
	}

	threshold := float64(0.8)
	if req.Threshold != nil {
		threshold = float64(*req.Threshold)
		if threshold < 0 || threshold > 1 {
			writeError(w, http.StatusBadRequest, "threshold must be between 0 and 1", "INVALID_THRESHOLD")
			return
		}
	}

	select {
	case s.sem <- struct{}{}:
	default:
		writeError(w, http.StatusServiceUnavailable, "server at replay capacity", "CAPACITY_EXCEEDED")
		return
	}

	if s.completer == nil {
		<-s.sem
		writeError(w, http.StatusServiceUnavailable, "agentgateway not configured", "MISSING_DEPENDENCY")
		return
	}

	provider := ""
	if req.Provider != nil {
		provider = *req.Provider
	}

	reqHeaders := map[string]string{}
	if req.RequestHeaders != nil {
		reqHeaders = *req.RequestHeaders
	}

	engine := replay.NewEngine(s.store, s.completer)
	variant := replay.VariantConfig{
		Model:          req.Model,
		Provider:       provider,
		Temperature:    float64PtrFromFloat32(req.Temperature),
		TopP:           float64PtrFromFloat32(req.TopP),
		MaxTokens:      req.MaxTokens,
		RequestHeaders: SanitizeRequestHeaders(reqHeaders),
	}

	prepared, err := engine.Setup(r.Context(), req.BaselineTraceId, variant, threshold)
	if err != nil {
		<-s.sem
		if errors.Is(err, storage.ErrTraceNotFound) {
			writeError(w, http.StatusNotFound, err.Error(), "BASELINE_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "INTERNAL_ERROR")
		return
	}

	id := openapi_types.UUID(prepared.ExperimentID)
	st := storage.StatusRunning
	writeJSON(w, http.StatusAccepted, GateCheckResponse{
		ExperimentId: &id,
		Status:       &st,
	})

	go func() {
		defer func() { <-s.sem }()
		RunGatePipeline(s.ctx, s.store, engine, prepared, threshold, s.log)
	}()
}

func (s *Server) GetGateStatus(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	exp, err := s.store.GetExperiment(r.Context(), uuid.UUID(id))
	if err != nil {
		if errors.Is(err, storage.ErrExperimentNotFound) {
			writeError(w, http.StatusNotFound, "experiment not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get status", "DB_ERROR")
		return
	}

	oid := openapi_types.UUID(exp.ID)
	progress := float32(exp.Progress)
	writeJSON(w, http.StatusOK, GateStatusResponse{
		ExperimentId: &oid,
		Status:       &exp.Status,
		Progress:     &progress,
	})
}

func (s *Server) GetGateReport(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	s.GetExperimentReport(w, r, id)
}

// --- Trace Handlers ---

func (s *Server) ListTraces(w http.ResponseWriter, r *http.Request, params ListTracesParams) {
	filters := storage.TraceFilters{
		Limit:  defaultListLimit,
		Offset: 0,
	}
	filters.Limit = clampLimit(params.Limit, defaultListLimit, maxListLimit)
	filters.Offset = clampOffset(params.Offset)
	filters.Model = params.Model
	filters.Provider = params.Provider

	summaries, err := s.store.ListUniqueTraces(r.Context(), filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list traces", "DB_ERROR")
		return
	}

	resp := make([]TraceSummary, 0, len(summaries))
	for _, info := range summaries {
		ca := info.CreatedAt

		models := info.Models
		providers := info.Providers

		resp = append(resp, TraceSummary{
			TraceId:   &info.TraceID,
			Models:    &models,
			Providers: &providers,
			StepCount: &info.StepCount,
			CreatedAt: &ca,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) GetTrace(w http.ResponseWriter, r *http.Request, traceId string) {
	resp, ok := s.buildTraceDetail(w, r.Context(), traceId, "failed to get trace spans", "trace not found", "NOT_FOUND", "trace prompt content", "failed to get tool captures")
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) CompareTraces(w http.ResponseWriter, r *http.Request, baselineTraceId string, candidateTraceId string) {
	baseline, ok := s.buildTraceDetail(w, r.Context(), baselineTraceId, "failed to get baseline spans", "baseline trace not found", "BASELINE_NOT_FOUND", "baseline prompt", "failed to get baseline tool captures")
	if !ok {
		return
	}

	candidate, ok := s.buildTraceDetail(w, r.Context(), candidateTraceId, "failed to get candidate spans", "candidate trace not found", "CANDIDATE_NOT_FOUND", "candidate prompt", "failed to get candidate tool captures")
	if !ok {
		return
	}

	// 3. Get Specific Drift Result for this pair
	var score *float32
	var divergenceReason *string
	var divergenceIdx *int

	dr, err := s.store.GetDriftResultForPair(r.Context(), candidateTraceId, baselineTraceId)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "failed to get drift result", "DB_ERROR")
		return
	}

	if err == nil && dr != nil {
		sval := float32(dr.DriftScore)
		score = &sval
		divergenceReason = &dr.Details.Reason
		divergenceIdx = &dr.Details.DivergenceStep
	}

	resp := TraceComparison{
		Baseline:  baseline,
		Candidate: candidate,
		Diff: &struct {
			DivergenceReason    *string  `json:"divergenceReason,omitempty"`
			DivergenceStepIndex *int     `json:"divergenceStepIndex,omitempty"`
			SimilarityScore     *float32 `json:"similarityScore,omitempty"`
		}{
			DivergenceReason:    divergenceReason,
			DivergenceStepIndex: divergenceIdx,
			SimilarityScore:     score,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

func buildExperimentRuns(runs []*storage.ExperimentRun) []ExperimentRun {
	oapiRuns := make([]ExperimentRun, 0, len(runs))
	for _, run := range runs {
		rid := openapi_types.UUID(run.ID)
		rt := ExperimentRunRunType(run.RunType)

		oapiRun := ExperimentRun{
			Id:        &rid,
			RunType:   &rt,
			TraceId:   run.TraceID,
			Status:    &run.Status,
			Error:     run.Error,
			CreatedAt: &run.CreatedAt,
		}
		if variantConfig := apiVariantConfigFromStorage(run.VariantConfig); variantConfig != nil {
			oapiRun.VariantConfig = variantConfig
		}
		oapiRuns = append(oapiRuns, oapiRun)
	}
	return oapiRuns
}

func buildToolCaptures(captures []*storage.ToolCapture) []ToolCapture {
	oapiCaptures := make([]ToolCapture, 0, len(captures))
	for _, c := range captures {
		lms := c.LatencyMS
		rc := ToolCaptureRiskClass(c.RiskClass)

		argsMap := make(map[string]any, len(c.Args))
		for k, v := range c.Args {
			argsMap[k] = v
		}
		resultMap := make(map[string]any, len(c.Result))
		for k, v := range c.Result {
			resultMap[k] = v
		}

		oapiCaptures = append(oapiCaptures, ToolCapture{
			ToolName:  &c.ToolName,
			Args:      &argsMap,
			Result:    &resultMap,
			RiskClass: &rc,
			LatencyMs: &lms,
		})
	}
	return oapiCaptures
}

func (s *Server) buildTraceDetail(w http.ResponseWriter, ctx context.Context, traceID string, spanErrorMessage string, notFoundMessage string, notFoundCode string, promptContext string, toolCaptureError string) (*TraceDetail, bool) {
	spans, err := s.store.GetReplayTraceSpans(ctx, traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, spanErrorMessage, "DB_ERROR")
		return nil, false
	}
	if len(spans) == 0 {
		writeError(w, http.StatusNotFound, notFoundMessage, notFoundCode)
		return nil, false
	}

	oapiSteps := make([]TraceStep, 0, len(spans))
	for _, span := range spans {
		idx := span.StepIndex
		pt := span.PromptTokens
		ct := span.CompletionTokens
		lms := span.LatencyMS

		var prompt PromptContent
		if !s.mapToStruct(w, span.Prompt, &prompt, promptContext) {
			return nil, false
		}

		oapiSteps = append(oapiSteps, TraceStep{
			SpanId:           &span.SpanID,
			StepIndex:        &idx,
			Provider:         &span.Provider,
			Model:            &span.Model,
			Prompt:           &prompt,
			Completion:       &span.Completion,
			PromptTokens:     &pt,
			CompletionTokens: &ct,
			LatencyMs:        &lms,
		})
	}

	captures, err := s.store.GetToolCapturesByTrace(ctx, traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, toolCaptureError, "DB_ERROR")
		return nil, false
	}

	oapiCaptures := buildToolCaptures(captures)
	return &TraceDetail{
		Steps:        &oapiSteps,
		ToolCaptures: &oapiCaptures,
		TraceId:      &traceID,
	}, true
}

// --- Health ---

func (s *Server) GetHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	if err := s.store.Ping(r.Context()); err != nil {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

// --- Helpers ---

// allowedRequestHeaders is the set of headers that the gate check API accepts.
var allowedRequestHeaders = map[string]bool{
	http.CanonicalHeaderKey("X-Freeze-Trace-ID"):   true,
	http.CanonicalHeaderKey("X-Freeze-Span-ID"):    true,
	http.CanonicalHeaderKey("X-Freeze-Step-Index"): true,
}

// SanitizeRequestHeaders canonicalizes keys and filters to the allowlist.
func SanitizeRequestHeaders(raw map[string]string) map[string]string {
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
