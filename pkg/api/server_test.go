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
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
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
	mu                sync.Mutex
	replayTraces      map[string][]*storage.ReplayTrace
	experiments       map[uuid.UUID]*storage.Experiment
	experimentRuns    map[uuid.UUID][]*storage.ExperimentRun
	analysisResults   map[uuid.UUID][]*storage.AnalysisResult
	toolCaptures      map[string][]*storage.ToolCapture
	baselines         map[string]*storage.Baseline
	driftResults      []*storage.DriftResult
	createdReplays    []*storage.ReplayTrace
	pingErr           error
	toolErr           error // injected error for tool captures
	analysisErr       error
	createExperimentErr error
	createAnalysisErr error
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
		return e, nil
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
				return r, nil
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
	return m.experimentRuns[experimentID], nil
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
	if m.analysisErr != nil {
		return nil, m.analysisErr
	}
	return m.analysisResults[experimentID], nil
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
		return b, nil
	}
	return nil, storage.ErrBaselineNotFound
}

func (m *mockStorage) ListBaselines(_ context.Context) ([]*storage.Baseline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []*storage.Baseline
	for _, b := range m.baselines {
		all = append(all, b)
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
	return experiments, nil
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
		RequestHeaders:  &map[string]string{"X-Freeze-Trace-ID": "trace-abc", "Authorization": "redacted"},
		Threshold:       float32Ptr(0.8),
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

func TestHandleBaselines_Lifecycle(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-1", 1)
	srv := newTestServer(t, store, nil, 5)

	// 1. Create Baseline
	body, _ := json.Marshal(map[string]string{"name": "B1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/baselines/trace-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// 2. List Baselines
	req = httptest.NewRequest(http.MethodGet, "/api/v1/baselines", nil)
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	var list []Baseline
	json.Unmarshal(w.Body.Bytes(), &list)
	assert.Len(t, list, 1)
	assert.Equal(t, "trace-1", *list[0].TraceId)

	// 3. Delete Baseline
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/baselines/trace-1", nil)
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// 4. Verify gone
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

func TestHandleTraces_ListAndGet(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-1", 2)
	store.replayTraces["trace-1"][1].Model = "claude-3-5-sonnet"
	store.replayTraces["trace-1"][1].Provider = "anthropic"
	srv := newTestServer(t, store, nil, 5)

	// List Traces
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?limit=10&offset=0", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var list []TraceSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Len(t, list, 1)
	assert.Equal(t, "trace-1", *list[0].TraceId)
	assert.Equal(t, 2, *list[0].StepCount)
	assert.Equal(t, []string{"claude-3-5-sonnet", "gpt-4"}, *list[0].Models)
	assert.Equal(t, []string{"anthropic", "openai"}, *list[0].Providers)

	// Get Trace Detail
	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces/trace-1", nil)
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var detail TraceDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	assert.Equal(t, "trace-1", *detail.TraceId)
	assert.Len(t, *detail.Steps, 2)
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
	assert.Equal(t, "trace-1", (*resp.VariantConfig.RequestHeaders)["X-Freeze-Trace-ID"])
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

func TestHandleCompareTraces_CorrectPair(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "base-1", 1)
	seedBaseline(store, "cand-1", 1)

	// Create drift result for specific pair
	store.CreateDriftResult(context.Background(), &storage.DriftResult{
		TraceID:         "cand-1",
		BaselineTraceID: "base-1",
		DriftScore:      0.88,
		Details:         storage.DriftDetails{Reason: "matches perfectly"},
	})

	srv := newTestServer(t, store, nil, 5)

	// 1. Success case
	req := httptest.NewRequest(http.MethodGet, "/api/v1/compare/base-1/cand-1", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp TraceComparison
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Diff.SimilarityScore)
	assert.Equal(t, float32(0.88), *resp.Diff.SimilarityScore)

	// 2. No result case (should omit score)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/compare/base-1/other-cand", nil)
	// Seed other-cand traces first so we don't 404 on traces
	seedBaseline(store, "other-cand", 1)
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp2 TraceComparison
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp2))
	assert.Nil(t, resp2.Diff.SimilarityScore)
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

func TestHandleGateReport_Completed(t *testing.T) {
	store := newMockStorage()
	expID := uuid.New()
	baselineRunID := uuid.New()
	variantRunID := uuid.New()

	store.experiments[expID] = &storage.Experiment{
		ID:              expID,
		BaselineTraceID: "trace-abc",
		Status:          storage.StatusCompleted,
		Progress:        1.0,
	}
	store.experimentRuns[expID] = []*storage.ExperimentRun{
		{ID: baselineRunID, ExperimentID: expID, RunType: storage.RunTypeBaseline, Status: storage.StatusCompleted},
		{ID: variantRunID, ExperimentID: expID, RunType: storage.RunTypeVariant, Status: storage.StatusCompleted},
	}
	store.analysisResults[expID] = []*storage.AnalysisResult{
		{
			ExperimentID:    expID,
			SimilarityScore: 0.92,
			TokenDelta:      150,
			LatencyDelta:    200,
			BehaviorDiff:    storage.BehaviorDiff{Verdict: "pass"},
		},
	}

	srv := newTestServer(t, store, &mockCompleter{}, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments/"+expID.String()+"/report", nil)
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp ExperimentReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "completed", *resp.Status)
	assert.Equal(t, "pass", *resp.Verdict)
	require.NotNil(t, resp.SimilarityScore)
	assert.Equal(t, float32(0.92), *resp.SimilarityScore)
	assert.Len(t, *resp.Runs, 2)
}

func TestHandleHealth(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp["status"])
}

func TestHandleHealth_Degraded(t *testing.T) {
	store := newMockStorage()
	store.pingErr = errors.New("db down")
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "degraded", resp["status"])
}

func TestSanitizeRequestHeaders_AllowlistFilters(t *testing.T) {
	result := SanitizeRequestHeaders(map[string]string{
		"X-Freeze-Trace-ID": "trace-1",
		"Authorization":     "Bearer sk-secret",
		"X-Custom":          "should-be-dropped",
	})

	assert.Equal(t, "trace-1", result["X-Freeze-Trace-Id"])
	assert.Empty(t, result["Authorization"])
	assert.Empty(t, result["X-Custom"])
	assert.Len(t, result, 1)
}

func TestSanitizeRequestHeaders_CanonicalizesKeys(t *testing.T) {
	result := SanitizeRequestHeaders(map[string]string{
		"x-freeze-trace-id": "trace-1",
		"X-FREEZE-SPAN-ID":  "span-1",
	})

	assert.Equal(t, "trace-1", result["X-Freeze-Trace-Id"])
	assert.Equal(t, "span-1", result["X-Freeze-Span-Id"])
}

func TestSanitizeRequestHeaders_NilOnEmpty(t *testing.T) {
	assert.Nil(t, SanitizeRequestHeaders(nil))
	assert.Nil(t, SanitizeRequestHeaders(map[string]string{}))
	assert.Nil(t, SanitizeRequestHeaders(map[string]string{"Authorization": "secret"}))
}

func TestSanitizeRequestHeaders_AllFreeze(t *testing.T) {
	result := SanitizeRequestHeaders(map[string]string{
		"X-Freeze-Trace-ID":   "trace-1",
		"X-Freeze-Span-ID":    "span-1",
		"X-Freeze-Step-Index": "2",
	})

	assert.Len(t, result, 3)
	assert.Equal(t, "trace-1", result["X-Freeze-Trace-Id"])
	assert.Equal(t, "span-1", result["X-Freeze-Span-Id"])
	assert.Equal(t, "2", result["X-Freeze-Step-Index"])
}

func TestHandleGateCheck_NilCompleter_503(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-abc", 1)

	// Untyped nil completer
	log, err := logger.New("debug")
	require.NoError(t, err)
	srv := NewServer(ServerConfig{
		Port:                 0,
		MaxConcurrentReplays: 5,
	}, store, nil, log)

	body, _ := json.Marshal(GateCheckRequest{
		BaselineTraceId: "trace-abc",
		Model:           "gpt-4",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "agentgateway not configured")
}

func TestHandleGateCheck_TypedNilCompleter_503(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-abc", 1)

	// Typed nil: simulates the serve.go path where *agwclient.Client is nil
	// but stored in a replay.Completer interface (non-nil interface, nil value).
	var typedNil *mockCompleter // nil pointer to a concrete type
	log, err := logger.New("debug")
	require.NoError(t, err)
	srv := NewServer(ServerConfig{
		Port:                 0,
		MaxConcurrentReplays: 5,
	}, store, typedNil, log)

	body, _ := json.Marshal(GateCheckRequest{
		BaselineTraceId: "trace-abc",
		Model:           "gpt-4",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "agentgateway not configured")
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
