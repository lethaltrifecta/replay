package api

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

const (
	defaultListLimit = 20
	maxListLimit     = 100
	maxJSONBodyBytes = 1 << 20
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Header already sent; best-effort log would go here via middleware.
		_ = err
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty request body")
	}
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return normalizeJSONDecodeError(err)
	}
	return nil
}

func readOptionalJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return normalizeJSONDecodeError(err)
	}
	return nil
}

func normalizeJSONDecodeError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return fmt.Errorf("request body too large (max %d bytes)", maxJSONBodyBytes)
	}
	return fmt.Errorf("invalid JSON: %w", err)
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
	if headers := apiReplayRequestHeadersFromStorage(cfg.RequestHeaders); headers != nil {
		out.RequestHeaders = headers
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

	resp := DriftResult{
		TraceId:         &result.TraceID,
		BaselineTraceId: &result.BaselineTraceID,
		DriftScore:      &score,
		Verdict:         &verdict,
		CreatedAt:       &createdAt,
	}
	if !result.Details.IsZero() {
		resp.Details = &DriftDetails{
			Reason:         stringPtr(result.Details.Reason),
			DivergenceStep: result.Details.DivergenceStep,
			RiskEscalation: boolPtrIfTrue(result.Details.RiskEscalation),
		}
	}
	return resp
}

func apiReplayRequestHeadersFromStorage(headers map[string]string) *ReplayRequestHeaders {
	if len(headers) == 0 {
		return nil
	}

	out := ReplayRequestHeaders{}
	for key, value := range headers {
		if value == "" {
			continue
		}
		switch http.CanonicalHeaderKey(key) {
		case http.CanonicalHeaderKey("X-Freeze-Trace-ID"):
			out.FreezeTraceId = &value
		case http.CanonicalHeaderKey("X-Freeze-Span-ID"):
			out.FreezeSpanId = &value
		case http.CanonicalHeaderKey("X-Freeze-Step-Index"):
			out.FreezeStepIndex = &value
		}
	}

	if out.FreezeTraceId == nil && out.FreezeSpanId == nil && out.FreezeStepIndex == nil {
		return nil
	}
	return &out
}

func storageRequestHeadersFromAPI(headers *ReplayRequestHeaders) map[string]string {
	if headers == nil {
		return nil
	}

	raw := map[string]string{}
	if headers.FreezeTraceId != nil && *headers.FreezeTraceId != "" {
		raw[http.CanonicalHeaderKey("X-Freeze-Trace-ID")] = *headers.FreezeTraceId
	}
	if headers.FreezeSpanId != nil && *headers.FreezeSpanId != "" {
		raw[http.CanonicalHeaderKey("X-Freeze-Span-ID")] = *headers.FreezeSpanId
	}
	if headers.FreezeStepIndex != nil && *headers.FreezeStepIndex != "" {
		raw[http.CanonicalHeaderKey("X-Freeze-Step-Index")] = *headers.FreezeStepIndex
	}

	return SanitizeRequestHeaders(raw)
}
