package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
	"github.com/lethaltrifecta/replay/pkg/replay"
	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/lethaltrifecta/replay/pkg/utils/logger"
)

// --- Mock Completer ---

type mockCompleter struct {
	responses []*agwclient.CompletionResponse
	errors    []error
	calls     int
}

func (m *mockCompleter) Complete(_ context.Context, _ *agwclient.CompletionRequest) (*agwclient.CompletionResponse, error) {
	idx := m.calls
	m.calls++
	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return &agwclient.CompletionResponse{
		ID:    "default",
		Model: "test-model",
		Choices: []agwclient.Choice{{
			Message: agwclient.ChatMessage{Role: "assistant", Content: "ok"},
		}},
		Usage: agwclient.Usage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20},
	}, nil
}

// --- Mock Storage ---

type mockStorage struct {
	mu                  sync.Mutex
	replayTraces        map[string][]*storage.ReplayTrace
	experiments         map[uuid.UUID]*storage.Experiment
	experimentRuns      map[uuid.UUID][]*storage.ExperimentRun
	analysisResults     map[uuid.UUID][]*storage.AnalysisResult
	toolCaptures        map[string][]*storage.ToolCapture
	baselines           map[string]*storage.Baseline
	driftResults        []*storage.DriftResult
	createdReplays      []*storage.ReplayTrace
	pingErr             error
	toolErr             error // injected error for tool captures
	analysisErr         error
	analysisCalls       int
	latestAnalysisCalls int
	createExperimentErr error
	createAnalysisErr   error
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		replayTraces:    make(map[string][]*storage.ReplayTrace),
		experiments:     make(map[uuid.UUID]*storage.Experiment),
		experimentRuns:  make(map[uuid.UUID][]*storage.ExperimentRun),
		analysisResults: make(map[uuid.UUID][]*storage.AnalysisResult),
		toolCaptures:    make(map[string][]*storage.ToolCapture),
		baselines:       make(map[string]*storage.Baseline),
	}
}

func (m *mockStorage) Close() error                    { return nil }
func (m *mockStorage) Ping(_ context.Context) error    { return m.pingErr }
func (m *mockStorage) Migrate(_ context.Context) error { return nil }

func (m *mockStorage) GetReplayTraceSpans(_ context.Context, traceID string) ([]*storage.ReplayTrace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.replayTraces[traceID], nil
}

func (m *mockStorage) ListReplayTraces(_ context.Context, filters storage.TraceFilters) ([]*storage.ReplayTrace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []*storage.ReplayTrace
	for _, spans := range m.replayTraces {
		all = append(all, spans...)
	}
	return all, nil
}

func (m *mockStorage) ListUniqueTraces(_ context.Context, filters storage.TraceFilters) ([]*storage.TraceSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var summaries []*storage.TraceSummary
	for id, spans := range m.replayTraces {
		if len(spans) == 0 {
			continue
		}

		matches := filters.Model == nil && filters.Provider == nil
		modelSet := make(map[string]struct{})
		providerSet := make(map[string]struct{})
		createdAt := spans[0].CreatedAt
		for _, span := range spans {
			if span.CreatedAt.Before(createdAt) {
				createdAt = span.CreatedAt
			}
			if span.Model != "" {
				modelSet[span.Model] = struct{}{}
			}
			if span.Provider != "" {
				providerSet[span.Provider] = struct{}{}
			}
			modelMatch := filters.Model == nil || span.Model == *filters.Model
			providerMatch := filters.Provider == nil || span.Provider == *filters.Provider
			if modelMatch && providerMatch {
				matches = true
			}
		}
		if !matches {
			continue
		}

		models := make([]string, 0, len(modelSet))
		for model := range modelSet {
			models = append(models, model)
		}
		sort.Strings(models)
		providers := make([]string, 0, len(providerSet))
		for provider := range providerSet {
			providers = append(providers, provider)
		}
		sort.Strings(providers)
		summaries = append(summaries, &storage.TraceSummary{
			TraceID:   id,
			Models:    models,
			Providers: providers,
			StepCount: len(spans),
			CreatedAt: createdAt,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].CreatedAt.Equal(summaries[j].CreatedAt) {
			if filters.SortAsc {
				return summaries[i].TraceID < summaries[j].TraceID
			}
			return summaries[i].TraceID > summaries[j].TraceID
		}
		if filters.SortAsc {
			return summaries[i].CreatedAt.Before(summaries[j].CreatedAt)
		}
		return summaries[i].CreatedAt.After(summaries[j].CreatedAt)
	})

	if filters.Offset >= len(summaries) {
		return nil, nil
	}
	end := filters.Offset + filters.Limit
	if filters.Limit == 0 || end > len(summaries) {
		end = len(summaries)
	}
	return summaries[filters.Offset:end], nil
}

func (m *mockStorage) CreateExperiment(_ context.Context, exp *storage.Experiment) error {
	if m.createExperimentErr != nil {
		return m.createExperimentErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.experiments[exp.ID] = exp
	return nil
}

func (m *mockStorage) GetExperiment(_ context.Context, id uuid.UUID) (*storage.Experiment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.experiments[id]; ok {
		clone := *e
		return &clone, nil
	}
	return nil, storage.ErrExperimentNotFound
}

func (m *mockStorage) UpdateExperiment(_ context.Context, exp *storage.Experiment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.experiments[exp.ID] = exp
	return nil
}

func (m *mockStorage) CreateExperimentRun(_ context.Context, run *storage.ExperimentRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.experimentRuns[run.ExperimentID] = append(m.experimentRuns[run.ExperimentID], run)
	return nil
}

func (m *mockStorage) GetExperimentRun(_ context.Context, id uuid.UUID) (*storage.ExperimentRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, runs := range m.experimentRuns {
		for _, r := range runs {
			if r.ID == id {
				clone := *r
				return &clone, nil
			}
		}
	}
	return nil, storage.ErrNotFound
}

func (m *mockStorage) UpdateExperimentRun(_ context.Context, run *storage.ExperimentRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for experimentID, runs := range m.experimentRuns {
		for i, existing := range runs {
			if existing.ID == run.ID {
				m.experimentRuns[experimentID][i] = run
				return nil
			}
		}
	}
	return storage.ErrNotFound
}

func (m *mockStorage) ListExperimentRuns(_ context.Context, experimentID uuid.UUID) ([]*storage.ExperimentRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runs := m.experimentRuns[experimentID]
	out := make([]*storage.ExperimentRun, 0, len(runs))
	for _, run := range runs {
		clone := *run
		out = append(out, &clone)
	}
	return out, nil
}

func (m *mockStorage) CreateReplayTrace(_ context.Context, trace *storage.ReplayTrace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createdReplays = append(m.createdReplays, trace)
	m.replayTraces[trace.TraceID] = append(m.replayTraces[trace.TraceID], trace)
	return nil
}

func (m *mockStorage) CreateAnalysisResult(_ context.Context, result *storage.AnalysisResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createAnalysisErr != nil {
		return m.createAnalysisErr
	}
	m.analysisResults[result.ExperimentID] = append(m.analysisResults[result.ExperimentID], result)
	return nil
}

func (m *mockStorage) GetAnalysisResults(_ context.Context, experimentID uuid.UUID) ([]*storage.AnalysisResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.analysisCalls++
	if m.analysisErr != nil {
		return nil, m.analysisErr
	}
	results := m.analysisResults[experimentID]
	out := make([]*storage.AnalysisResult, 0, len(results))
	for _, result := range results {
		clone := *result
		out = append(out, &clone)
	}
	return out, nil
}

func (m *mockStorage) GetLatestAnalysisResults(_ context.Context, experimentIDs []uuid.UUID) (map[uuid.UUID]*storage.AnalysisResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latestAnalysisCalls++
	if m.analysisErr != nil {
		return nil, m.analysisErr
	}

	results := make(map[uuid.UUID]*storage.AnalysisResult, len(experimentIDs))
	for _, experimentID := range experimentIDs {
		if latest := latestAnalysisResult(m.analysisResults[experimentID]); latest != nil {
			clone := *latest
			results[experimentID] = &clone
		}
	}
	return results, nil
}

func (m *mockStorage) GetToolCapturesByTrace(_ context.Context, traceID string) ([]*storage.ToolCapture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.toolErr != nil {
		return nil, m.toolErr
	}
	return m.toolCaptures[traceID], nil
}

func (m *mockStorage) MarkTraceAsBaseline(_ context.Context, b *storage.Baseline) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.replayTraces[b.TraceID]; !ok {
		return storage.ErrTraceNotFound
	}
	b.CreatedAt = time.Now()
	m.baselines[b.TraceID] = b
	return nil
}

func (m *mockStorage) GetBaseline(_ context.Context, traceID string) (*storage.Baseline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.baselines[traceID]; ok {
		clone := *b
		return &clone, nil
	}
	return nil, storage.ErrBaselineNotFound
}

func (m *mockStorage) ListBaselines(_ context.Context) ([]*storage.Baseline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []*storage.Baseline
	for _, b := range m.baselines {
		clone := *b
		all = append(all, &clone)
	}
	return all, nil
}

func (m *mockStorage) UnmarkBaseline(_ context.Context, traceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.baselines[traceID]; ok {
		delete(m.baselines, traceID)
		return nil
	}
	return storage.ErrBaselineNotFound
}

func (m *mockStorage) CreateDriftResult(_ context.Context, dr *storage.DriftResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.driftResults = append(m.driftResults, dr)
	return nil
}

func (m *mockStorage) GetDriftResults(_ context.Context, traceID string) ([]*storage.DriftResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var results []*storage.DriftResult
	for _, dr := range m.driftResults {
		if dr.TraceID == traceID {
			results = append(results, dr)
		}
	}
	return results, nil
}

func (m *mockStorage) GetLatestDriftResult(_ context.Context, traceID string) (*storage.DriftResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.driftResults) - 1; i >= 0; i-- {
		if m.driftResults[i].TraceID == traceID {
			return m.driftResults[i], nil
		}
	}
	return nil, storage.ErrNotFound
}

func (m *mockStorage) GetDriftResultForPair(_ context.Context, traceID string, baselineTraceID string) (*storage.DriftResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.driftResults) - 1; i >= 0; i-- {
		if m.driftResults[i].TraceID == traceID && m.driftResults[i].BaselineTraceID == baselineTraceID {
			return m.driftResults[i], nil
		}
	}
	return nil, storage.ErrNotFound
}

func (m *mockStorage) ListDriftResults(_ context.Context, limit int, offset int) ([]*storage.DriftResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if offset >= len(m.driftResults) {
		return nil, nil
	}
	end := offset + limit
	if limit == 0 || end > len(m.driftResults) {
		end = len(m.driftResults)
	}
	return m.driftResults[offset:end], nil
}

func (m *mockStorage) HasDriftResultForBaseline(_ context.Context, traceID string, baselineTraceID string) (bool, error) {
	dr, _ := m.GetDriftResultForPair(context.Background(), traceID, baselineTraceID)
	return dr != nil, nil
}

func (m *mockStorage) GetDriftResultsByBaseline(_ context.Context, baselineTraceID string, limit int) ([]*storage.DriftResult, error) {
	return nil, nil
}

// Stub remaining Storage interface methods
func (m *mockStorage) CreateOTELTrace(_ context.Context, _ *storage.OTELTrace) error { return nil }
func (m *mockStorage) GetOTELTraceSpans(_ context.Context, _ string) ([]*storage.OTELTrace, error) {
	return nil, nil
}
func (m *mockStorage) CreateIngestionBatch(_ context.Context, _ []*storage.OTELTrace, _ []*storage.ReplayTrace, _ []*storage.ToolCapture) (storage.IngestCounts, error) {
	return storage.IngestCounts{}, nil
}
func (m *mockStorage) CreateToolCapture(_ context.Context, _ *storage.ToolCapture) error { return nil }
func (m *mockStorage) GetToolCaptureByArgs(_ context.Context, _ string, _ string) (*storage.ToolCapture, error) {
	return nil, nil
}
func (m *mockStorage) ListExperiments(_ context.Context, filters storage.ExperimentFilters) ([]*storage.Experiment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	experiments := make([]*storage.Experiment, 0, len(m.experiments))
	for _, exp := range m.experiments {
		if filters.Status != nil && exp.Status != *filters.Status {
			continue
		}
		experiments = append(experiments, exp)
	}
	sort.Slice(experiments, func(i, j int) bool {
		if experiments[i].CreatedAt.Equal(experiments[j].CreatedAt) {
			return experiments[i].ID.String() < experiments[j].ID.String()
		}
		return experiments[i].CreatedAt.After(experiments[j].CreatedAt)
	})
	if filters.Offset >= len(experiments) {
		return nil, nil
	}
	if filters.Offset > 0 {
		experiments = experiments[filters.Offset:]
	}
	if filters.Limit > 0 && filters.Limit < len(experiments) {
		experiments = experiments[:filters.Limit]
	}
	out := make([]*storage.Experiment, 0, len(experiments))
	for _, exp := range experiments {
		clone := *exp
		out = append(out, &clone)
	}
	return out, nil
}
func (m *mockStorage) CreateEvaluator(_ context.Context, _ *storage.Evaluator) error { return nil }
func (m *mockStorage) GetEvaluator(_ context.Context, _ int) (*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStorage) GetEvaluatorByName(_ context.Context, _ string) (*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStorage) UpdateEvaluator(_ context.Context, _ *storage.Evaluator) error { return nil }
func (m *mockStorage) ListEvaluators(_ context.Context, _ bool) ([]*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStorage) DeleteEvaluator(_ context.Context, _ int) error { return nil }
func (m *mockStorage) CreateEvaluationRun(_ context.Context, _ *storage.EvaluationRun) error {
	return nil
}
func (m *mockStorage) GetEvaluationRun(_ context.Context, _ uuid.UUID) (*storage.EvaluationRun, error) {
	return nil, nil
}
func (m *mockStorage) UpdateEvaluationRun(_ context.Context, _ *storage.EvaluationRun) error {
	return nil
}
func (m *mockStorage) CreateEvaluatorResult(_ context.Context, _ *storage.EvaluatorResult) error {
	return nil
}
func (m *mockStorage) GetEvaluatorResults(_ context.Context, _ uuid.UUID) ([]*storage.EvaluatorResult, error) {
	return nil, nil
}
func (m *mockStorage) CreateHumanEvaluation(_ context.Context, _ *storage.HumanEvaluation) error {
	return nil
}
func (m *mockStorage) GetHumanEvaluation(_ context.Context, _ uuid.UUID) (*storage.HumanEvaluation, error) {
	return nil, nil
}
func (m *mockStorage) UpdateHumanEvaluation(_ context.Context, _ *storage.HumanEvaluation) error {
	return nil
}
func (m *mockStorage) ListPendingHumanEvaluations(_ context.Context, _ *string) ([]*storage.HumanEvaluation, error) {
	return nil, nil
}
func (m *mockStorage) CreateGroundTruth(_ context.Context, _ *storage.GroundTruth) error { return nil }
func (m *mockStorage) GetGroundTruth(_ context.Context, _ string) (*storage.GroundTruth, error) {
	return nil, nil
}
func (m *mockStorage) UpdateGroundTruth(_ context.Context, _ *storage.GroundTruth) error { return nil }
func (m *mockStorage) ListGroundTruth(_ context.Context, _ *string) ([]*storage.GroundTruth, error) {
	return nil, nil
}
func (m *mockStorage) DeleteGroundTruth(_ context.Context, _ string) error { return nil }
func (m *mockStorage) CreateEvaluationSummary(_ context.Context, _ *storage.EvaluationSummary) error {
	return nil
}
func (m *mockStorage) GetEvaluationSummary(_ context.Context, _ uuid.UUID) ([]*storage.EvaluationSummary, error) {
	return nil, nil
}

// --- Test helpers ---

func newTestServer(t *testing.T, store *mockStorage, completer *mockCompleter, maxConcurrent int) *Server {
	t.Helper()
	log, err := logger.New("debug")
	require.NoError(t, err)
	return NewServer(ServerConfig{
		Port:                 0,
		MaxConcurrentReplays: maxConcurrent,
	}, store, completer, log)
}

func seedBaseline(store *mockStorage, traceID string, steps int) {
	for i := 0; i < steps; i++ {
		store.replayTraces[traceID] = append(store.replayTraces[traceID], &storage.ReplayTrace{
			TraceID:   traceID,
			SpanID:    uuid.New().String(),
			RunID:     traceID,
			StepIndex: i,
			CreatedAt: time.Now(),
			Provider:  "openai",
			Model:     "gpt-4",
			Prompt: storage.JSONB{
				"messages": []interface{}{
					map[string]interface{}{"role": "user", "content": "Hello"},
				},
			},
			Completion:       "baseline response",
			PromptTokens:     50,
			CompletionTokens: 50,
			TotalTokens:      100,
			LatencyMS:        200,
		})
	}
}

func jsonRequest(t *testing.T, srv *Server, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		payload, err := json.Marshal(body)
		require.NoError(t, err)

		req = httptest.NewRequest(method, target, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recorder, req)
	return recorder
}

func decodeJSON[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var value T
	require.NotEmpty(t, recorder.Body.Bytes())
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &value))
	return value
}

func completionResponse(content string) *agwclient.CompletionResponse {
	return &agwclient.CompletionResponse{
		ID:    uuid.NewString(),
		Model: "test-model",
		Choices: []agwclient.Choice{{
			Message: agwclient.ChatMessage{Role: "assistant", Content: content},
		}},
		Usage: agwclient.Usage{
			PromptTokens:     50,
			CompletionTokens: 50,
			TotalTokens:      100,
		},
	}
}

func float32Ptr(f float32) *float32 { return &f }
func float64Ptr(f float64) *float64 { return &f }
func intPtr(i int) *int             { return &i }

// --- Tests ---

func TestHandleGateCheck_202(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-abc", 2)
	completer := &mockCompleter{}

	srv := newTestServer(t, store, completer, 5)

	body, _ := json.Marshal(GateCheckRequest{
		BaselineTraceId: "trace-abc",
		Model:           "gpt-4o",
		Provider:        ptr("openai"),
		Temperature:     float32Ptr(0.2),
		TopP:            float32Ptr(0.9),
		MaxTokens:       intPtr(256),
		RequestHeaders: &ReplayRequestHeaders{
			FreezeTraceId: ptr("trace-abc"),
		},
		Threshold: float32Ptr(0.8),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp GateCheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "running", *resp.Status)
	assert.NotNil(t, resp.ExperimentId)

	// Verify experiment was created
	expID := uuid.UUID(*resp.ExperimentId)
	exp, err := store.GetExperiment(context.Background(), expID)
	require.NoError(t, err)
	require.NotNil(t, exp.Config.Threshold)
	assert.Equal(t, "gpt-4o", exp.Config.Model)
	assert.Equal(t, "openai", exp.Config.Provider)
	assert.InDelta(t, 0.8, *exp.Config.Threshold, 0.000001)
	require.NotNil(t, exp.Config.Temperature)
	require.NotNil(t, exp.Config.TopP)
	require.NotNil(t, exp.Config.MaxTokens)
	assert.InDelta(t, 0.2, *exp.Config.Temperature, 0.000001)
	assert.InDelta(t, 0.9, *exp.Config.TopP, 0.000001)
	assert.Equal(t, 256, *exp.Config.MaxTokens)
	require.Len(t, exp.Config.RequestHeaders, 1)
	assert.Equal(t, "trace-abc", exp.Config.RequestHeaders[http.CanonicalHeaderKey("X-Freeze-Trace-ID")])

	runs, err := store.ListExperimentRuns(context.Background(), expID)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	require.NotNil(t, runs[1].VariantConfig.Temperature)
	require.NotNil(t, runs[1].VariantConfig.TopP)
	require.NotNil(t, runs[1].VariantConfig.MaxTokens)
	assert.InDelta(t, 0.2, *runs[1].VariantConfig.Temperature, 0.000001)
	assert.InDelta(t, 0.9, *runs[1].VariantConfig.TopP, 0.000001)
	assert.Equal(t, 256, *runs[1].VariantConfig.MaxTokens)
}

func TestHandleGateCheck_InvalidThreshold_400(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-abc", 1)
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	body, _ := json.Marshal(GateCheckRequest{
		BaselineTraceId: "trace-abc",
		Model:           "gpt-4o",
		Threshold:       float32Ptr(1.5),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "threshold must be between 0 and 1")
}

func TestHandleGateCheck_RequestBodyTooLarge_400(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	oversized := fmt.Sprintf(`{"baselineTraceId":"trace-1","model":"%s"}`, strings.Repeat("a", maxJSONBodyBytes))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", strings.NewReader(oversized))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "request body too large")
}

func TestHandleGateCheck_BaselineNotFound_404(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	body, _ := json.Marshal(GateCheckRequest{
		BaselineTraceId: "nonexistent",
		Model:           "gpt-4o",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "trace not found")
}

func TestHandleBaselines_DeleteRemovesBaseline(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-1", 1)
	require.NoError(t, store.MarkTraceAsBaseline(context.Background(), &storage.Baseline{
		TraceID: "trace-1",
	}))
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/baselines/trace-1", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	_, err := store.GetBaseline(context.Background(), "trace-1")
	assert.Error(t, err)
}

func TestHandleBaselines_CreateNotFound_404(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, nil, 5)

	body, _ := json.Marshal(map[string]string{"name": "B1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/baselines/nonexistent", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleBaselines_CreateAllowsEmptyBody(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-1", 1)
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/baselines/trace-1", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp Baseline
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.TraceId)
	assert.Equal(t, "trace-1", *resp.TraceId)
	assert.Nil(t, resp.Name)
	assert.Nil(t, resp.Description)
}

func TestHandleBaselines_CreateRejectsOversizedBody_400(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-1", 1)
	srv := newTestServer(t, store, nil, 5)

	oversized := fmt.Sprintf(`{"name":"%s"}`, strings.Repeat("a", maxJSONBodyBytes))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/baselines/trace-1", strings.NewReader(oversized))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "INVALID_BODY")
}

func TestHandleTraces_GetReturnsDetailAndToolCaptures(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-1", 2)
	store.replayTraces["trace-1"][1].Model = "claude-3-5-sonnet"
	store.replayTraces["trace-1"][1].Provider = "anthropic"
	store.toolCaptures["trace-1"] = []*storage.ToolCapture{
		{
			TraceID:   "trace-1",
			SpanID:    "capture-span-1",
			StepIndex: 1,
			ToolName:  "calculator",
			Args:      storage.JSONB{"a": 2, "b": 2},
			Result:    storage.JSONB{"sum": 4},
			Error:     ptr("tool warning"),
			LatencyMS: 42,
			RiskClass: storage.RiskClassRead,
		},
	}
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/trace-1", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var detail TraceDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	assert.Equal(t, "trace-1", *detail.TraceId)
	assert.Len(t, *detail.Steps, 2)
	require.Len(t, *detail.ToolCaptures, 1)
	assert.Equal(t, 1, *(*detail.ToolCaptures)[0].StepIndex)
	assert.Equal(t, "tool warning", *(*detail.ToolCaptures)[0].Error)
}

func TestHandleTraces_GetPreservesPromptToolFields(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-tools", 1)
	store.replayTraces["trace-tools"][0].Prompt = storage.JSONB{
		"messages": []interface{}{
			map[string]interface{}{
				"role":         "assistant",
				"content":      "Calling calculator",
				"name":         "planner",
				"tool_call_id": "call-123",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   "call-123",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "calculator",
							"arguments": "{\"a\":2,\"b\":2}",
						},
					},
				},
			},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "calculator",
				},
			},
		},
		"tool_choice": map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": "calculator",
			},
		},
	}
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/trace-tools", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))

	steps, ok := payload["steps"].([]interface{})
	require.True(t, ok)
	require.Len(t, steps, 1)

	step, ok := steps[0].(map[string]interface{})
	require.True(t, ok)

	prompt, ok := step["prompt"].(map[string]interface{})
	require.True(t, ok)

	messages, ok := prompt["messages"].([]interface{})
	require.True(t, ok)
	require.Len(t, messages, 1)

	message, ok := messages[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "planner", message["name"])
	assert.Equal(t, "call-123", message["tool_call_id"])
	toolCalls, ok := message["tool_calls"].([]interface{})
	require.True(t, ok)
	require.Len(t, toolCalls, 1)

	tools, ok := prompt["tools"].([]interface{})
	require.True(t, ok)
	require.Len(t, tools, 1)

	toolChoice, ok := prompt["tool_choice"].(map[string]interface{})
	require.True(t, ok)
	function, ok := toolChoice["function"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "calculator", function["name"])
}

func TestHandleTraces_ListFilterPreservesFullSummary(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-1", 2)
	store.replayTraces["trace-1"][1].Model = "claude-3-5-sonnet"
	store.replayTraces["trace-1"][1].Provider = "anthropic"
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?model=gpt-4", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var list []TraceSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, 2, *list[0].StepCount)
	assert.Equal(t, []string{"claude-3-5-sonnet", "gpt-4"}, *list[0].Models)
	assert.Equal(t, []string{"anthropic", "openai"}, *list[0].Providers)
}

func TestHandleTraces_ListNormalizesPagination(t *testing.T) {
	store := newMockStorage()
	for i := 0; i < 120; i++ {
		traceID := fmt.Sprintf("trace-%03d", i)
		seedBaseline(store, traceID, 1)
		store.replayTraces[traceID][0].CreatedAt = time.Unix(int64(i), 0)
	}
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?limit=1000&offset=-5", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var list []TraceSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Len(t, list, maxListLimit)
	assert.Equal(t, "trace-119", *list[0].TraceId)
}

func TestHandleTraces_GetPromptSchemaDrift_500(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-1", 1)
	store.replayTraces["trace-1"][0].Prompt = storage.JSONB{
		"messages": []interface{}{
			map[string]interface{}{"role": 123, "content": "Hello"},
		},
	}
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/trace-1", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "SCHEMA_DRIFT")
}

func TestHandleTraces_GetToolError_500(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-1", 1)
	store.toolErr = errors.New("db error")
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/trace-1", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to get tool captures")
}

func TestHandleExperiments_AnalysisError_500(t *testing.T) {
	store := newMockStorage()
	store.analysisErr = errors.New("analysis unavailable")
	expID := uuid.New()
	store.experiments[expID] = &storage.Experiment{
		ID:              expID,
		Name:            "exp",
		BaselineTraceID: "trace-1",
		Status:          storage.StatusCompleted,
		Progress:        1.0,
		Config: storage.ExperimentConfig{
			Model:     "gpt-4o",
			Provider:  "openai",
			Threshold: float64Ptr(0.8),
		},
		CreatedAt: time.Now(),
	}
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to fetch analysis results")
}

func TestHandleExperiments_UsesLatestAnalysisBatchLookup(t *testing.T) {
	store := newMockStorage()
	now := time.Now()

	completedID := uuid.New()
	failedID := uuid.New()
	runningID := uuid.New()
	store.experiments[completedID] = &storage.Experiment{
		ID:              completedID,
		Name:            "completed-exp",
		BaselineTraceID: "trace-completed",
		Status:          storage.StatusCompleted,
		Progress:        1.0,
		CreatedAt:       now,
	}
	store.experiments[failedID] = &storage.Experiment{
		ID:              failedID,
		Name:            "failed-exp",
		BaselineTraceID: "trace-failed",
		Status:          storage.StatusFailed,
		Progress:        1.0,
		CreatedAt:       now.Add(-time.Second),
	}
	store.experiments[runningID] = &storage.Experiment{
		ID:              runningID,
		Name:            "running-exp",
		BaselineTraceID: "trace-running",
		Status:          storage.StatusRunning,
		Progress:        0.5,
		CreatedAt:       now.Add(-2 * time.Second),
	}
	store.analysisResults[completedID] = []*storage.AnalysisResult{{
		ID:           10,
		ExperimentID: completedID,
		BehaviorDiff: storage.BehaviorDiff{Verdict: "pass"},
		CreatedAt:    now,
	}}
	store.analysisResults[failedID] = []*storage.AnalysisResult{{
		ID:           11,
		ExperimentID: failedID,
		BehaviorDiff: storage.BehaviorDiff{Verdict: "fail"},
		CreatedAt:    now,
	}}
	srv := newTestServer(t, store, nil, 5)

	recorder := jsonRequest(t, srv, http.MethodGet, "/api/v1/experiments", nil)
	require.Equal(t, http.StatusOK, recorder.Code)

	experiments := decodeJSON[[]Experiment](t, recorder)
	require.Len(t, experiments, 3)
	assert.Equal(t, 1, store.latestAnalysisCalls)
	assert.Zero(t, store.analysisCalls)
	require.NotNil(t, experiments[0].Verdict)
	assert.Equal(t, "pass", string(*experiments[0].Verdict))
	require.NotNil(t, experiments[1].Verdict)
	assert.Equal(t, "fail", string(*experiments[1].Verdict))
	assert.Nil(t, experiments[2].Verdict)
}

func TestHandleExperiment_TypedConfigAndOptionalVariantRuns(t *testing.T) {
	store := newMockStorage()
	expID := uuid.New()
	baselineRunID := uuid.New()
	variantRunID := uuid.New()
	store.experiments[expID] = &storage.Experiment{
		ID:              expID,
		Name:            "typed-exp",
		BaselineTraceID: "trace-1",
		Status:          storage.StatusRunning,
		Progress:        0.5,
		Config: storage.ExperimentConfig{
			Model:          "gpt-4o",
			Provider:       "openai",
			RequestHeaders: map[string]string{"X-Freeze-Trace-ID": "trace-1"},
			Threshold:      float64Ptr(0.8),
		},
		CreatedAt: time.Now(),
	}
	store.experimentRuns[expID] = []*storage.ExperimentRun{
		{
			ID:            baselineRunID,
			ExperimentID:  expID,
			RunType:       storage.RunTypeBaseline,
			TraceID:       ptr("trace-1"),
			VariantConfig: storage.VariantConfig{},
			Status:        storage.StatusCompleted,
			CreatedAt:     time.Now(),
		},
		{
			ID:           variantRunID,
			ExperimentID: expID,
			RunType:      storage.RunTypeVariant,
			TraceID:      ptr("trace-2"),
			VariantConfig: storage.VariantConfig{
				Model:          "gpt-4o",
				Provider:       "openai",
				RequestHeaders: map[string]string{"X-Freeze-Trace-ID": "trace-1"},
			},
			Status:    storage.StatusRunning,
			CreatedAt: time.Now(),
		},
	}
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments/"+expID.String(), nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp ExperimentDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Threshold)
	assert.Equal(t, float32(0.8), *resp.Threshold)
	require.NotNil(t, resp.VariantConfig)
	assert.Equal(t, "gpt-4o", *resp.VariantConfig.Model)
	assert.Equal(t, "openai", *resp.VariantConfig.Provider)
	require.NotNil(t, resp.VariantConfig.RequestHeaders)
	assert.Equal(t, "trace-1", *resp.VariantConfig.RequestHeaders.FreezeTraceId)
	require.Len(t, *resp.Runs, 2)
	assert.Nil(t, (*resp.Runs)[0].VariantConfig)
	require.NotNil(t, (*resp.Runs)[1].VariantConfig)
}

func TestHandleExperimentReport_UsesLatestAnalysisResult(t *testing.T) {
	store := newMockStorage()
	expID := uuid.New()
	store.experiments[expID] = &storage.Experiment{
		ID:              expID,
		Name:            "report-exp",
		BaselineTraceID: "trace-1",
		Status:          storage.StatusCompleted,
		Progress:        1.0,
		Config:          storage.ExperimentConfig{Model: "gpt-4o"},
		CreatedAt:       time.Now(),
	}
	store.analysisResults[expID] = []*storage.AnalysisResult{
		{
			ID:              1,
			ExperimentID:    expID,
			SimilarityScore: 0.2,
			TokenDelta:      1,
			LatencyDelta:    1,
			BehaviorDiff:    storage.BehaviorDiff{Verdict: "fail", Reason: "older"},
			FirstDivergence: storage.FirstDivergence{StepIndex: intPtr(1)},
			SafetyDiff:      storage.SafetyDiff{},
			CreatedAt:       time.Now().Add(-time.Minute),
		},
		{
			ID:              2,
			ExperimentID:    expID,
			SimilarityScore: 0.95,
			TokenDelta:      2,
			LatencyDelta:    3,
			BehaviorDiff:    storage.BehaviorDiff{Verdict: "pass", Reason: "latest"},
			FirstDivergence: storage.FirstDivergence{StepIndex: intPtr(2)},
			SafetyDiff:      storage.SafetyDiff{},
			CreatedAt:       time.Now(),
		},
	}
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments/"+expID.String()+"/report", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp ExperimentReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Verdict)
	assert.Equal(t, "pass", *resp.Verdict)
	require.NotNil(t, resp.SimilarityScore)
	assert.Equal(t, float32(0.95), *resp.SimilarityScore)
	require.NotNil(t, resp.Analysis)
	require.NotNil(t, resp.Analysis.BehaviorDiff)
	assert.Equal(t, "latest", *resp.Analysis.BehaviorDiff.Reason)
}

func TestGovernanceWorkflowContract(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "baseline-trace", 2)
	seedBaseline(store, "candidate-trace", 2)
	store.replayTraces["candidate-trace"][1].Completion = "candidate response"
	store.replayTraces["candidate-trace"][1].Model = "claude-3-5-sonnet"
	store.replayTraces["candidate-trace"][1].Provider = "anthropic"
	store.toolCaptures["candidate-trace"] = []*storage.ToolCapture{
		{
			TraceID:   "candidate-trace",
			SpanID:    "candidate-span-1",
			StepIndex: 1,
			ToolName:  "file_write",
			Args:      storage.JSONB{"path": "report.md"},
			Result:    storage.JSONB{"written": true},
			Error:     ptr("requires approval"),
			LatencyMS: 85,
			RiskClass: storage.RiskClassWrite,
		},
	}

	divergenceStep := 1
	require.NoError(t, store.CreateDriftResult(context.Background(), &storage.DriftResult{
		TraceID:         "candidate-trace",
		BaselineTraceID: "baseline-trace",
		DriftScore:      0.61,
		Verdict:         storage.DriftVerdictWarn,
		Details: storage.DriftDetails{
			Reason:         "tool risk escalated",
			DivergenceStep: &divergenceStep,
			RiskEscalation: true,
		},
		CreatedAt: time.Now(),
	}))

	srv := newTestServer(t, store, &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			completionResponse("baseline response"),
			completionResponse("baseline response"),
		},
	}, 5)

	t.Run("baseline selection", func(t *testing.T) {
		recorder := jsonRequest(t, srv, http.MethodPost, "/api/v1/baselines/baseline-trace", map[string]string{
			"name":        "Approved baseline",
			"description": "Known-good agent behavior",
		})
		require.Equal(t, http.StatusCreated, recorder.Code)

		created := decodeJSON[Baseline](t, recorder)
		require.NotNil(t, created.TraceId)
		assert.Equal(t, "baseline-trace", *created.TraceId)
		require.NotNil(t, created.Name)
		assert.Equal(t, "Approved baseline", *created.Name)

		recorder = jsonRequest(t, srv, http.MethodGet, "/api/v1/baselines", nil)
		require.Equal(t, http.StatusOK, recorder.Code)

		baselines := decodeJSON[[]Baseline](t, recorder)
		require.Len(t, baselines, 1)
		assert.Equal(t, "baseline-trace", *baselines[0].TraceId)
	})

	t.Run("trace inbox", func(t *testing.T) {
		recorder := jsonRequest(t, srv, http.MethodGet, "/api/v1/traces?limit=10&offset=0", nil)
		require.Equal(t, http.StatusOK, recorder.Code)

		traces := decodeJSON[[]TraceSummary](t, recorder)
		require.Len(t, traces, 2)
		assert.Equal(t, "candidate-trace", *traces[0].TraceId)
		assert.Equal(t, "baseline-trace", *traces[1].TraceId)
	})

	t.Run("drift inbox and detail", func(t *testing.T) {
		recorder := jsonRequest(t, srv, http.MethodGet, "/api/v1/drift-results?limit=10&offset=0", nil)
		require.Equal(t, http.StatusOK, recorder.Code)

		results := decodeJSON[[]DriftResult](t, recorder)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].Verdict)
		assert.Equal(t, DriftResultVerdict(storage.DriftVerdictWarn), *results[0].Verdict)

		recorder = jsonRequest(t, srv, http.MethodGet, "/api/v1/drift-results/candidate-trace?baselineTraceId=baseline-trace", nil)
		require.Equal(t, http.StatusOK, recorder.Code)

		result := decodeJSON[DriftResult](t, recorder)
		require.NotNil(t, result.BaselineTraceId)
		assert.Equal(t, "baseline-trace", *result.BaselineTraceId)
		require.NotNil(t, result.Details)
		require.NotNil(t, result.Details.Reason)
		assert.Equal(t, "tool risk escalated", *result.Details.Reason)
		require.NotNil(t, result.Details.RiskEscalation)
		assert.True(t, *result.Details.RiskEscalation)
	})

	t.Run("side-by-side comparison", func(t *testing.T) {
		recorder := jsonRequest(t, srv, http.MethodGet, "/api/v1/compare/baseline-trace/candidate-trace", nil)
		require.Equal(t, http.StatusOK, recorder.Code)

		comparison := decodeJSON[TraceComparison](t, recorder)
		require.NotNil(t, comparison.Baseline)
		require.NotNil(t, comparison.Candidate)
		require.NotNil(t, comparison.Diff)
		require.NotNil(t, comparison.Diff.SimilarityScore)
		assert.Equal(t, float32(0.61), *comparison.Diff.SimilarityScore)
		require.NotNil(t, comparison.Diff.DivergenceReason)
		assert.Equal(t, "tool risk escalated", *comparison.Diff.DivergenceReason)
		require.NotNil(t, comparison.Candidate.ToolCaptures)
		require.Len(t, *comparison.Candidate.ToolCaptures, 1)
		assert.Equal(t, "file_write", *(*comparison.Candidate.ToolCaptures)[0].ToolName)
		assert.Equal(t, "requires approval", *(*comparison.Candidate.ToolCaptures)[0].Error)
	})

	t.Run("gate kickoff, polling, and report", func(t *testing.T) {
		recorder := jsonRequest(t, srv, http.MethodPost, "/api/v1/gate/check", GateCheckRequest{
			BaselineTraceId: "baseline-trace",
			Model:           "gpt-4o",
			Provider:        ptr("openai"),
			RequestHeaders: &ReplayRequestHeaders{
				FreezeTraceId: ptr("baseline-trace"),
			},
			Threshold: float32Ptr(0.8),
		})
		require.Equal(t, http.StatusAccepted, recorder.Code)

		started := decodeJSON[GateCheckResponse](t, recorder)
		require.NotNil(t, started.ExperimentId)
		experimentID := uuid.UUID(*started.ExperimentId)

		require.Eventually(t, func() bool {
			statusRecorder := jsonRequest(t, srv, http.MethodGet, "/api/v1/gate/status/"+experimentID.String(), nil)
			if statusRecorder.Code != http.StatusOK {
				return false
			}
			status := decodeJSON[GateStatusResponse](t, statusRecorder)
			return status.Status != nil && *status.Status == storage.StatusCompleted
		}, time.Second, 10*time.Millisecond)

		recorder = jsonRequest(t, srv, http.MethodGet, "/api/v1/experiments?limit=10&offset=0", nil)
		require.Equal(t, http.StatusOK, recorder.Code)

		experiments := decodeJSON[[]Experiment](t, recorder)
		require.Len(t, experiments, 1)
		assert.Equal(t, experimentID, uuid.UUID(*experiments[0].Id))
		require.NotNil(t, experiments[0].Verdict)
		assert.Equal(t, "pass", *experiments[0].Verdict)

		recorder = jsonRequest(t, srv, http.MethodGet, "/api/v1/experiments/"+experimentID.String()+"/report", nil)
		require.Equal(t, http.StatusOK, recorder.Code)

		report := decodeJSON[ExperimentReport](t, recorder)
		require.NotNil(t, report.Status)
		assert.Equal(t, storage.StatusCompleted, *report.Status)
		require.NotNil(t, report.Verdict)
		assert.Equal(t, "pass", *report.Verdict)
		require.NotNil(t, report.SimilarityScore)
		assert.InDelta(t, 0.85, *report.SimilarityScore, 0.000001)
		require.NotNil(t, report.Analysis)
		require.NotNil(t, report.Analysis.BehaviorDiff)
		require.NotNil(t, report.Runs)
		assert.Len(t, *report.Runs, 2)
	})
}

func TestGovernanceWorkflowContract_FailVerdict(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "baseline-fail-trace", 2)

	srv := newTestServer(t, store, &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			completionResponse("drop the production database immediately"),
			completionResponse("delete every user account right now"),
		},
	}, 5)

	recorder := jsonRequest(t, srv, http.MethodPost, "/api/v1/gate/check", GateCheckRequest{
		BaselineTraceId: "baseline-fail-trace",
		Model:           "gpt-4o",
		Provider:        ptr("openai"),
		RequestHeaders: &ReplayRequestHeaders{
			FreezeTraceId: ptr("baseline-fail-trace"),
		},
		Threshold: float32Ptr(0.8),
	})
	require.Equal(t, http.StatusAccepted, recorder.Code)

	started := decodeJSON[GateCheckResponse](t, recorder)
	require.NotNil(t, started.ExperimentId)
	experimentID := uuid.UUID(*started.ExperimentId)

	require.Eventually(t, func() bool {
		statusRecorder := jsonRequest(t, srv, http.MethodGet, "/api/v1/gate/status/"+experimentID.String(), nil)
		if statusRecorder.Code != http.StatusOK {
			return false
		}
		status := decodeJSON[GateStatusResponse](t, statusRecorder)
		return status.Status != nil && *status.Status == storage.StatusCompleted
	}, time.Second, 10*time.Millisecond)

	recorder = jsonRequest(t, srv, http.MethodGet, "/api/v1/experiments?limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, recorder.Code)

	experiments := decodeJSON[[]Experiment](t, recorder)
	require.Len(t, experiments, 1)
	require.NotNil(t, experiments[0].Verdict)
	assert.Equal(t, "fail", *experiments[0].Verdict)

	recorder = jsonRequest(t, srv, http.MethodGet, "/api/v1/experiments/"+experimentID.String()+"/report", nil)
	require.Equal(t, http.StatusOK, recorder.Code)

	report := decodeJSON[ExperimentReport](t, recorder)
	require.NotNil(t, report.Status)
	assert.Equal(t, storage.StatusCompleted, *report.Status)
	require.NotNil(t, report.Verdict)
	assert.Equal(t, "fail", *report.Verdict)
	require.NotNil(t, report.SimilarityScore)
	assert.Less(t, *report.SimilarityScore, float32(0.8))
	assert.Nil(t, report.Error)

	require.NotNil(t, report.Analysis)
	require.NotNil(t, report.Analysis.BehaviorDiff)
	require.NotNil(t, report.Analysis.BehaviorDiff.Verdict)
	assert.Equal(t, "fail", *report.Analysis.BehaviorDiff.Verdict)
	require.NotNil(t, report.Analysis.FirstDivergence)
	require.NotNil(t, report.Analysis.FirstDivergence.Type)
	assert.Equal(t, "response_content", *report.Analysis.FirstDivergence.Type)
	require.NotNil(t, report.Analysis.FirstDivergence.StepIndex)
	assert.Equal(t, 0, *report.Analysis.FirstDivergence.StepIndex)
	require.NotNil(t, report.Analysis.FirstDivergence.BaselineExcerpt)
	assert.Equal(t, "baseline response", *report.Analysis.FirstDivergence.BaselineExcerpt)
	require.NotNil(t, report.Analysis.FirstDivergence.VariantExcerpt)
	assert.Contains(t, *report.Analysis.FirstDivergence.VariantExcerpt, "drop the production database")

	require.NotNil(t, report.Runs)
	require.Len(t, *report.Runs, 2)
	require.NotNil(t, (*report.Runs)[1].Status)
	assert.Equal(t, storage.StatusCompleted, *(*report.Runs)[1].Status)
	require.NotNil(t, (*report.Runs)[1].TraceId)
	assert.NotEmpty(t, *(*report.Runs)[1].TraceId)
}

func TestHandleDriftResults_Pagination(t *testing.T) {
	store := newMockStorage()
	for i := 0; i < 5; i++ {
		store.CreateDriftResult(context.Background(), &storage.DriftResult{
			TraceID:         fmt.Sprintf("trace-%d", i),
			BaselineTraceID: "base",
			DriftScore:      0.5,
		})
	}

	srv := newTestServer(t, store, nil, 5)

	// Page 1 (limit 2)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/drift-results?limit=2&offset=0", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	var resp1 []DriftResult
	json.Unmarshal(w.Body.Bytes(), &resp1)
	assert.Len(t, resp1, 2)
	assert.Equal(t, "trace-0", *resp1[0].TraceId)

	// Page 2 (offset 2)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/drift-results?limit=2&offset=2", nil)
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	var resp2 []DriftResult
	json.Unmarshal(w.Body.Bytes(), &resp2)
	assert.Len(t, resp2, 2)
	assert.Equal(t, "trace-2", *resp2[0].TraceId)
}

func TestHandleDriftResults_LimitIsClamped(t *testing.T) {
	store := newMockStorage()
	for i := 0; i < 150; i++ {
		store.CreateDriftResult(context.Background(), &storage.DriftResult{
			TraceID:         fmt.Sprintf("trace-%03d", i),
			BaselineTraceID: "base",
			DriftScore:      0.5,
		})
	}

	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/drift-results?limit=1000", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp []DriftResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp, 100)
}

func TestHandleCompareTraces_NoDriftResultOmitsScore(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "base-1", 1)
	seedBaseline(store, "other-cand", 1)

	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compare/base-1/other-cand", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp TraceComparison
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Diff.SimilarityScore)
}

func TestHandleDriftResult_AmbiguousRequiresBaseline(t *testing.T) {
	store := newMockStorage()
	store.CreateDriftResult(context.Background(), &storage.DriftResult{
		TraceID:         "trace-1",
		BaselineTraceID: "base-1",
		DriftScore:      0.8,
	})
	store.CreateDriftResult(context.Background(), &storage.DriftResult{
		TraceID:         "trace-1",
		BaselineTraceID: "base-2",
		DriftScore:      0.6,
	})
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/drift-results/trace-1", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "AMBIGUOUS_RESULT")
}

func TestHandleDriftResult_BaselineQueryReturnsPair(t *testing.T) {
	store := newMockStorage()
	store.CreateDriftResult(context.Background(), &storage.DriftResult{
		TraceID:         "trace-1",
		BaselineTraceID: "base-1",
		DriftScore:      0.8,
	})
	store.CreateDriftResult(context.Background(), &storage.DriftResult{
		TraceID:         "trace-1",
		BaselineTraceID: "base-2",
		DriftScore:      0.6,
	})
	srv := newTestServer(t, store, nil, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/drift-results/trace-1?baselineTraceId=base-2", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp DriftResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.BaselineTraceId)
	assert.Equal(t, "base-2", *resp.BaselineTraceId)
	require.NotNil(t, resp.DriftScore)
	assert.Equal(t, float32(0.6), *resp.DriftScore)
}

func ptr(s string) *string { return &s }

func TestHandleGateCheck_MissingFields_400(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	tests := []struct {
		name string
		body GateCheckRequest
	}{
		{"missing baseline", GateCheckRequest{Model: "gpt-4o"}},
		{"missing model", GateCheckRequest{BaselineTraceId: "trace-abc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandleGateCheck_AtCapacity_503(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-abc", 1)
	srv := newTestServer(t, store, &mockCompleter{}, 1)

	// Fill the semaphore
	srv.sem <- struct{}{}

	body, _ := json.Marshal(GateCheckRequest{
		BaselineTraceId: "trace-abc",
		Model:           "gpt-4o",
		Threshold:       float32Ptr(0.8),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Drain semaphore
	<-srv.sem
}

func TestHandleGateCheck_InternalSetupErrorDoesNotLeak(t *testing.T) {
	store := newMockStorage()
	store.createExperimentErr = errors.New("sql: connection refused")
	seedBaseline(store, "trace-abc", 1)
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	body, _ := json.Marshal(GateCheckRequest{
		BaselineTraceId: "trace-abc",
		Model:           "gpt-4o",
		Threshold:       float32Ptr(0.8),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to initialize gate check")
	assert.NotContains(t, w.Body.String(), "connection refused")
}

func TestHandleGateStatus_Running(t *testing.T) {
	store := newMockStorage()
	expID := uuid.New()
	store.experiments[expID] = &storage.Experiment{
		ID:       expID,
		Status:   storage.StatusRunning,
		Progress: 0.5,
	}

	srv := newTestServer(t, store, &mockCompleter{}, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate/status/"+expID.String(), nil)
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp GateStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "running", *resp.Status)
	assert.Equal(t, float32(0.5), *resp.Progress)
}

func TestHandleGateStatus_NotFound(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate/status/"+fakeID.String(), nil)
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleExperimentReport_FailedRunExposesError(t *testing.T) {
	store := newMockStorage()
	expID := uuid.New()
	baselineRunID := uuid.New()
	variantRunID := uuid.New()

	store.experiments[expID] = &storage.Experiment{
		ID:              expID,
		BaselineTraceID: "trace-abc",
		Status:          storage.StatusFailed,
		Progress:        1.0,
	}
	store.experimentRuns[expID] = []*storage.ExperimentRun{
		{ID: baselineRunID, ExperimentID: expID, RunType: storage.RunTypeBaseline, Status: storage.StatusCompleted},
		{
			ID:           variantRunID,
			ExperimentID: expID,
			RunType:      storage.RunTypeVariant,
			Status:       storage.StatusFailed,
			Error:        ptr("agentgateway timeout"),
		},
	}

	srv := newTestServer(t, store, &mockCompleter{}, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments/"+expID.String()+"/report", nil)
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp ExperimentReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "failed", *resp.Status)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "agentgateway timeout", *resp.Error)
	assert.Nil(t, resp.Verdict)
	assert.Nil(t, resp.SimilarityScore)
	assert.Len(t, *resp.Runs, 2)
}

func TestHandleHealth(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus string
	}{
		{name: "ok", wantStatus: "ok"},
		{name: "degraded", pingErr: errors.New("db down"), wantStatus: "degraded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStorage()
			store.pingErr = tt.pingErr
			srv := newTestServer(t, store, &mockCompleter{}, 5)

			recorder := jsonRequest(t, srv, http.MethodGet, "/api/v1/health", nil)
			require.Equal(t, http.StatusOK, recorder.Code)

			resp := decodeJSON[map[string]string](t, recorder)
			assert.Equal(t, tt.wantStatus, resp["status"])
		})
	}
}

func TestRequestHeaderContract(t *testing.T) {
	t.Run("sanitize request headers", func(t *testing.T) {
		tests := []struct {
			name string
			raw  map[string]string
			want map[string]string
		}{
			{
				name: "allowlist filters unknown headers",
				raw: map[string]string{
					"X-Freeze-Trace-ID": "trace-1",
					"Authorization":     "Bearer sk-secret",
					"X-Custom":          "should-be-dropped",
				},
				want: map[string]string{
					http.CanonicalHeaderKey("X-Freeze-Trace-ID"): "trace-1",
				},
			},
			{
				name: "canonicalizes freeze keys",
				raw: map[string]string{
					"x-freeze-trace-id": "trace-1",
					"X-FREEZE-SPAN-ID":  "span-1",
				},
				want: map[string]string{
					http.CanonicalHeaderKey("X-Freeze-Trace-ID"): "trace-1",
					http.CanonicalHeaderKey("X-Freeze-Span-ID"):  "span-1",
				},
			},
			{
				name: "all freeze headers survive",
				raw: map[string]string{
					"X-Freeze-Trace-ID":   "trace-1",
					"X-Freeze-Span-ID":    "span-1",
					"X-Freeze-Step-Index": "2",
				},
				want: map[string]string{
					http.CanonicalHeaderKey("X-Freeze-Trace-ID"):   "trace-1",
					http.CanonicalHeaderKey("X-Freeze-Span-ID"):    "span-1",
					http.CanonicalHeaderKey("X-Freeze-Step-Index"): "2",
				},
			},
			{name: "nil remains nil", raw: nil, want: nil},
			{name: "empty remains nil", raw: map[string]string{}, want: nil},
			{name: "only disallowed headers become nil", raw: map[string]string{"Authorization": "secret"}, want: nil},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.want, SanitizeRequestHeaders(tt.raw))
			})
		}
	})

	t.Run("api storage round trip", func(t *testing.T) {
		headers := &ReplayRequestHeaders{
			FreezeTraceId:   ptr("trace-1"),
			FreezeSpanId:    ptr("span-1"),
			FreezeStepIndex: ptr("2"),
		}

		storageHeaders := storageRequestHeadersFromAPI(headers)
		require.Equal(t, map[string]string{
			http.CanonicalHeaderKey("X-Freeze-Trace-ID"):   "trace-1",
			http.CanonicalHeaderKey("X-Freeze-Span-ID"):    "span-1",
			http.CanonicalHeaderKey("X-Freeze-Step-Index"): "2",
		}, storageHeaders)

		roundTrip := apiReplayRequestHeadersFromStorage(storageHeaders)
		require.NotNil(t, roundTrip)
		require.NotNil(t, roundTrip.FreezeTraceId)
		require.NotNil(t, roundTrip.FreezeSpanId)
		require.NotNil(t, roundTrip.FreezeStepIndex)
		assert.Equal(t, "trace-1", *roundTrip.FreezeTraceId)
		assert.Equal(t, "span-1", *roundTrip.FreezeSpanId)
		assert.Equal(t, "2", *roundTrip.FreezeStepIndex)
	})
}

func TestHandleGateCheck_NilCompleterVariants_503(t *testing.T) {
	var typedNil *mockCompleter

	tests := []struct {
		name      string
		completer replay.Completer
	}{
		{name: "untyped nil", completer: nil},
		{name: "typed nil", completer: typedNil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStorage()
			seedBaseline(store, "trace-abc", 1)

			log, err := logger.New("debug")
			require.NoError(t, err)
			srv := NewServer(ServerConfig{
				Port:                 0,
				MaxConcurrentReplays: 5,
			}, store, tt.completer, log)

			recorder := jsonRequest(t, srv, http.MethodPost, "/api/v1/gate/check", GateCheckRequest{
				BaselineTraceId: "trace-abc",
				Model:           "gpt-4",
			})

			assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "agentgateway not configured")
		})
	}
}

type valueCompleter struct{}

func (valueCompleter) Complete(_ context.Context, _ *agwclient.CompletionRequest) (*agwclient.CompletionResponse, error) {
	return &agwclient.CompletionResponse{}, nil
}

func TestNewServer_ValueCompleter_DoesNotPanic(t *testing.T) {
	store := newMockStorage()
	log, err := logger.New("debug")
	require.NoError(t, err)

	require.NotPanics(t, func() {
		srv := NewServer(ServerConfig{
			Port:                 0,
			MaxConcurrentReplays: 5,
		}, store, valueCompleter{}, log)
		require.NotNil(t, srv.completer)
	})
}

func TestNewServer_SetsHTTPTimeouts(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, nil, 5)

	require.Equal(t, 15*time.Second, srv.httpServer.ReadTimeout)
	require.Equal(t, 30*time.Second, srv.httpServer.WriteTimeout)
	require.Equal(t, 60*time.Second, srv.httpServer.IdleTimeout)
}
