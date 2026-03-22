package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Header already sent; best-effort log would go here via middleware.
		_ = err
	}
}

func readJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("empty request body")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func readOptionalJSON(r *http.Request, v interface{}) error {
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

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
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
	if headers := cloneStringMap(cfg.RequestHeaders); len(headers) > 0 {
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

	latest := results[0]
	for _, result := range results[1:] {
		if result.CreatedAt.After(latest.CreatedAt) {
			latest = result
			continue
		}
		if result.CreatedAt.Equal(latest.CreatedAt) && result.ID > latest.ID {
			latest = result
		}
	}
	return latest
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
