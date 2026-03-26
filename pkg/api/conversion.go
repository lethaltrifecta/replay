package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

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

func baselineResponse(b *storage.Baseline) Baseline {
	createdAt := b.CreatedAt
	return Baseline{
		TraceId:     &b.TraceID,
		Name:        b.Name,
		Description: b.Description,
		CreatedAt:   &createdAt,
	}
}

func experimentResponse(exp *storage.Experiment, verdict *string) Experiment {
	id := openapi_types.UUID(exp.ID)
	progress := float32(exp.Progress)

	resp := Experiment{
		Id:              &id,
		Name:            &exp.Name,
		BaselineTraceId: &exp.BaselineTraceID,
		Status:          &exp.Status,
		Progress:        &progress,
		CreatedAt:       &exp.CreatedAt,
		CompletedAt:     exp.CompletedAt,
		Verdict:         verdict,
	}
	if threshold := float32PtrFromFloat64(exp.Config.Threshold); threshold != nil {
		resp.Threshold = threshold
	}
	if variantConfig := apiVariantConfigFromExperiment(exp.Config); variantConfig != nil {
		resp.VariantConfig = variantConfig
	}
	return resp
}

func experimentDetailResponse(exp *storage.Experiment, runs []ExperimentRun, verdict *string) ExperimentDetail {
	id := openapi_types.UUID(exp.ID)
	progress := float32(exp.Progress)

	resp := ExperimentDetail{
		Id:              &id,
		Name:            &exp.Name,
		BaselineTraceId: &exp.BaselineTraceID,
		Status:          &exp.Status,
		Progress:        &progress,
		CreatedAt:       &exp.CreatedAt,
		CompletedAt:     exp.CompletedAt,
		Runs:            &runs,
		Verdict:         verdict,
	}
	if threshold := float32PtrFromFloat64(exp.Config.Threshold); threshold != nil {
		resp.Threshold = threshold
	}
	if variantConfig := apiVariantConfigFromExperiment(exp.Config); variantConfig != nil {
		resp.VariantConfig = variantConfig
	}
	return resp
}

func analysisResultResponse(ar *storage.AnalysisResult) *AnalysisResult {
	if ar == nil {
		return nil
	}
	resp := &AnalysisResult{
		BehaviorDiff: &BehaviorDiff{
			Verdict: &ar.BehaviorDiff.Verdict,
			Reason:  &ar.BehaviorDiff.Reason,
		},
	}
	if !ar.FirstDivergence.IsZero() {
		resp.FirstDivergence = &FirstDivergence{
			StepIndex:       ar.FirstDivergence.StepIndex,
			ToolIndex:       ar.FirstDivergence.ToolIndex,
			Type:            stringPtr(ar.FirstDivergence.Type),
			Baseline:        stringPtr(ar.FirstDivergence.Baseline),
			Variant:         stringPtr(ar.FirstDivergence.Variant),
			BaselineExcerpt: stringPtr(ar.FirstDivergence.BaselineExcerpt),
			VariantExcerpt:  stringPtr(ar.FirstDivergence.VariantExcerpt),
			BaselineCount:   ar.FirstDivergence.BaselineCount,
			VariantCount:    ar.FirstDivergence.VariantCount,
			BaselineSteps:   ar.FirstDivergence.BaselineSteps,
			VariantSteps:    ar.FirstDivergence.VariantSteps,
		}
	}
	if !ar.SafetyDiff.IsZero() {
		resp.SafetyDiff = &SafetyDiff{
			RiskEscalation: boolPtrIfTrue(ar.SafetyDiff.RiskEscalation),
			BaselineRisk:   stringPtr(ar.SafetyDiff.BaselineRisk),
			VariantRisk:    stringPtr(ar.SafetyDiff.VariantRisk),
		}
	}
	return resp
}

func experimentReportResponse(exp *storage.Experiment, runs []ExperimentRun, analysis *storage.AnalysisResult, runFailure *string) ExperimentReport {
	id := openapi_types.UUID(exp.ID)
	resp := ExperimentReport{
		ExperimentId:    &id,
		BaselineTraceId: &exp.BaselineTraceID,
		Status:          &exp.Status,
		Runs:            &runs,
		Error:           runFailure,
	}
	if analysis == nil {
		return resp
	}

	score := float32(analysis.SimilarityScore)
	tokenDelta := analysis.TokenDelta
	latencyDelta := analysis.LatencyDelta

	resp.SimilarityScore = &score
	resp.TokenDelta = &tokenDelta
	resp.LatencyDelta = &latencyDelta
	resp.Analysis = analysisResultResponse(analysis)
	resp.Verdict = &analysis.BehaviorDiff.Verdict
	return resp
}

func gateStatusResponse(exp *storage.Experiment) GateStatusResponse {
	id := openapi_types.UUID(exp.ID)
	progress := float32(exp.Progress)
	return GateStatusResponse{
		ExperimentId: &id,
		Status:       &exp.Status,
		Progress:     &progress,
	}
}

func traceSummaryResponse(info *storage.TraceSummary) TraceSummary {
	createdAt := info.CreatedAt
	models := info.Models
	providers := info.Providers
	return TraceSummary{
		TraceId:   &info.TraceID,
		Models:    &models,
		Providers: &providers,
		StepCount: &info.StepCount,
		CreatedAt: &createdAt,
	}
}

func traceComparisonResponse(baseline, candidate *TraceDetail, drift *storage.DriftResult) TraceComparison {
	var score *float32
	var divergenceReason *string
	var divergenceIdx *int
	if drift != nil {
		value := float32(drift.DriftScore)
		score = &value
		divergenceReason = stringPtr(drift.Details.Reason)
		divergenceIdx = drift.Details.DivergenceStep
	}

	return TraceComparison{
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
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func boolPtrIfTrue(value bool) *bool {
	if !value {
		return nil
	}
	return &value
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
		stepIndex := c.StepIndex
		latencyMs := c.LatencyMS
		riskClass := ToolCaptureRiskClass(c.RiskClass)

		argsMap := make(map[string]any, len(c.Args))
		for k, v := range c.Args {
			argsMap[k] = v
		}
		resultMap := make(map[string]any, len(c.Result))
		for k, v := range c.Result {
			resultMap[k] = v
		}

		oapiCaptures = append(oapiCaptures, ToolCapture{
			StepIndex: &stepIndex,
			ToolName:  &c.ToolName,
			Args:      &argsMap,
			Result:    &resultMap,
			RiskClass: &riskClass,
			LatencyMs: &latencyMs,
			Error:     c.Error,
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

		step := TraceStep{
			SpanId:           &span.SpanID,
			StepIndex:        &idx,
			Provider:         &span.Provider,
			Model:            &span.Model,
			Prompt:           &prompt,
			Completion:       &span.Completion,
			PromptTokens:     &pt,
			CompletionTokens: &ct,
			LatencyMs:        &lms,
		}
		if len(span.Metadata) > 0 {
			md := map[string]interface{}(span.Metadata)
			step.Metadata = &md
		}
		oapiSteps = append(oapiSteps, step)
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
