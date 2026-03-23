package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/lethaltrifecta/replay/pkg/replay"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// ensure Server implements ServerInterface
var _ ServerInterface = (*Server)(nil)

// --- Governance Handlers ---

func (s *Server) ListBaselines(w http.ResponseWriter, r *http.Request) {
	baselines, err := s.store.ListBaselines(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list baselines", "DB_ERROR")
		return
	}

	resp := make([]Baseline, 0, len(baselines))
	for _, b := range baselines {
		resp = append(resp, baselineResponse(b))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) CreateBaseline(w http.ResponseWriter, r *http.Request, traceId string) {
	var body CreateBaselineJSONRequestBody
	if err := readOptionalJSON(w, r, &body); err != nil {
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

	writeJSON(w, http.StatusCreated, baselineResponse(stored))
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
	needsVerdict := make([]uuid.UUID, 0, len(experiments))
	for _, e := range experiments {
		if e.Status == storage.StatusCompleted || e.Status == storage.StatusFailed {
			needsVerdict = append(needsVerdict, e.ID)
		}
	}

	latestResults := map[uuid.UUID]*storage.AnalysisResult{}
	if len(needsVerdict) > 0 {
		latestResults, err = s.store.GetLatestAnalysisResults(r.Context(), needsVerdict)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch analysis results", "DB_ERROR")
			return
		}
	}

	for _, e := range experiments {
		var verdict *string
		if latest := latestResults[e.ID]; latest != nil {
			verdict = &latest.BehaviorDiff.Verdict
		}

		resp = append(resp, experimentResponse(e, verdict))
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

	var verdict *string
	if exp.Status == storage.StatusCompleted || exp.Status == storage.StatusFailed {
		results, err := s.store.GetAnalysisResults(r.Context(), exp.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch analysis results", "DB_ERROR")
			return
		}
		if latest := latestAnalysisResult(results); latest != nil {
			verdict = &latest.BehaviorDiff.Verdict
		}
	}

	writeJSON(w, http.StatusOK, experimentDetailResponse(exp, oapiRuns, verdict))
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

	var analysis *storage.AnalysisResult
	var reportError *string
	if exp.Status == storage.StatusCompleted || exp.Status == storage.StatusFailed {
		results, err := s.store.GetAnalysisResults(r.Context(), exp.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get analysis results", "DB_ERROR")
			return
		}
		if ar := latestAnalysisResult(results); ar != nil {
			analysis = ar
		} else if exp.Status == storage.StatusFailed {
			for _, run := range runs {
				if run.Status == storage.StatusFailed && run.Error != nil {
					reportError = run.Error
					break
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, experimentReportResponse(exp, oapiRuns, analysis, reportError))
}

// --- Gate Handlers ---

func (s *Server) CreateGateCheck(w http.ResponseWriter, r *http.Request) {
	var req GateCheckRequest
	if err := readJSON(w, r, &req); err != nil {
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

	engine := replay.NewEngine(s.store, s.completer)
	variant := replay.VariantConfig{
		Model:          req.Model,
		Provider:       provider,
		Temperature:    float64PtrFromFloat32(req.Temperature),
		TopP:           float64PtrFromFloat32(req.TopP),
		MaxTokens:      req.MaxTokens,
		RequestHeaders: storageRequestHeadersFromAPI(req.RequestHeaders),
	}

	prepared, err := engine.Setup(r.Context(), req.BaselineTraceId, variant, threshold)
	if err != nil {
		<-s.sem
		if errors.Is(err, storage.ErrTraceNotFound) {
			writeError(w, http.StatusNotFound, "baseline trace not found", "BASELINE_NOT_FOUND")
			return
		}
		s.log.Errorw("failed to initialize gate check", "baselineTraceId", req.BaselineTraceId, "model", req.Model, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to initialize gate check", "INTERNAL_ERROR")
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

		var toolExec replay.ToolExecutor
		if s.mcpURL != "" {
			freezeHeaders := storageRequestHeadersFromAPI(req.RequestHeaders)
			te, mcpErr := replay.NewMCPToolExecutor(s.ctx, s.mcpURL, freezeHeaders)
			if mcpErr != nil {
				s.log.Warnw("MCP connection failed, falling back to prompt-only replay",
					"experiment_id", prepared.ExperimentID,
					"mcp_url", s.mcpURL,
					"error", mcpErr,
				)
			} else {
				toolExec = te
				defer func() { _ = te.Close() }()
			}
		}

		loopCfg := replay.AgentLoopConfig{MaxTurns: s.agentLoopMaxTurns}
		RunGatePipeline(s.ctx, s.store, engine, prepared, threshold, s.log, toolExec, loopCfg)
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

	writeJSON(w, http.StatusOK, gateStatusResponse(exp))
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
		resp = append(resp, traceSummaryResponse(info))
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

	dr, err := s.store.GetDriftResultForPair(r.Context(), candidateTraceId, baselineTraceId)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "failed to get drift result", "DB_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, traceComparisonResponse(baseline, candidate, dr))
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
