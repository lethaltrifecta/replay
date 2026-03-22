package api

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

const (
	defaultListLimit = 20
	maxListLimit     = 100
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Header already sent; best-effort log would go here via middleware.
		_ = err
	}
}

func readJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty request body")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func readOptionalJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, msg string, code string) {
	writeJSON(w, status, Error{
		Error: msg,
		Code:  &code,
	})
}

func clampLimit(value *int, defaultLimit int, maxLimit int) int {
	if value == nil || *value <= 0 {
		return defaultLimit
	}
	if *value > maxLimit {
		return maxLimit
	}
	return *value
}

func clampOffset(value *int) int {
	if value == nil || *value < 0 {
		return 0
	}
	return *value
}

func float32PtrFromFloat64(v *float64) *float32 {
	if v == nil {
		return nil
	}
	out := float32(*v)
	return &out
}

func float64PtrFromFloat32(v *float32) *float64 {
	if v == nil {
		return nil
	}
	out := float64(*v)
	return &out
}

func apiVariantConfigFromStorage(cfg storage.VariantConfig) *VariantConfig {
	if cfg.IsZero() {
		return nil
	}

	out := VariantConfig{}
	if cfg.Model != "" {
		model := cfg.Model
		out.Model = &model
	}
	if cfg.Provider != "" {
		provider := cfg.Provider
		out.Provider = &provider
	}
	if cfg.Temperature != nil {
		temperature := float32(*cfg.Temperature)
		out.Temperature = &temperature
	}
	if cfg.TopP != nil {
		topP := float32(*cfg.TopP)
		out.TopP = &topP
	}
	if cfg.MaxTokens != nil {
		maxTokens := *cfg.MaxTokens
		out.MaxTokens = &maxTokens
	}
	if headers := maps.Clone(cfg.RequestHeaders); len(headers) > 0 {
		out.RequestHeaders = &headers
	}
	return &out
}

func apiVariantConfigFromExperiment(cfg storage.ExperimentConfig) *VariantConfig {
	return apiVariantConfigFromStorage(cfg.ToVariantConfig())
}

func latestAnalysisResult(results []*storage.AnalysisResult) *storage.AnalysisResult {
	if len(results) == 0 {
		return nil
	}
	return slices.MaxFunc(results, func(a, b *storage.AnalysisResult) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

func driftResultResponse(result *storage.DriftResult) DriftResult {
	createdAt := result.CreatedAt
	score := float32(result.DriftScore)
	verdict := DriftResultVerdict(result.Verdict)

	return DriftResult{
		TraceId:         &result.TraceID,
		BaselineTraceId: &result.BaselineTraceID,
		DriftScore:      &score,
		Verdict:         &verdict,
		Details: &DriftDetails{
			Reason:         &result.Details.Reason,
			DivergenceStep: &result.Details.DivergenceStep,
			RiskEscalation: &result.Details.RiskEscalation,
		},
		CreatedAt: &createdAt,
	}
}
