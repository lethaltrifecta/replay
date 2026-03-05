package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	mu              sync.Mutex
	replayTraces    map[string][]*storage.ReplayTrace
	experiments     map[uuid.UUID]*storage.Experiment
	experimentRuns  map[uuid.UUID][]*storage.ExperimentRun
	analysisResults map[uuid.UUID][]*storage.AnalysisResult
	toolCaptures    map[string][]*storage.ToolCapture
	createdReplays  []*storage.ReplayTrace
	pingErr         error
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		replayTraces:    make(map[string][]*storage.ReplayTrace),
		experiments:     make(map[uuid.UUID]*storage.Experiment),
		experimentRuns:  make(map[uuid.UUID][]*storage.ExperimentRun),
		analysisResults: make(map[uuid.UUID][]*storage.AnalysisResult),
		toolCaptures:    make(map[string][]*storage.ToolCapture),
	}
}

func (m *mockStorage) Close() error                          { return nil }
func (m *mockStorage) Ping(_ context.Context) error          { return m.pingErr }
func (m *mockStorage) Migrate(_ context.Context) error       { return nil }

func (m *mockStorage) GetReplayTraceSpans(_ context.Context, traceID string) ([]*storage.ReplayTrace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.replayTraces[traceID], nil
}

func (m *mockStorage) CreateExperiment(_ context.Context, exp *storage.Experiment) error {
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
	return nil, errors.New("not found")
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
	return nil, errors.New("not found")
}

func (m *mockStorage) UpdateExperimentRun(_ context.Context, run *storage.ExperimentRun) error {
	return nil
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
	m.analysisResults[result.ExperimentID] = append(m.analysisResults[result.ExperimentID], result)
	return nil
}

func (m *mockStorage) GetAnalysisResults(_ context.Context, experimentID uuid.UUID) ([]*storage.AnalysisResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.analysisResults[experimentID], nil
}

func (m *mockStorage) GetToolCapturesByTrace(_ context.Context, traceID string) ([]*storage.ToolCapture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.toolCaptures[traceID], nil
}

// Stub remaining Storage interface methods
func (m *mockStorage) CreateOTELTrace(_ context.Context, _ *storage.OTELTrace) error { return nil }
func (m *mockStorage) GetOTELTraceSpans(_ context.Context, _ string) ([]*storage.OTELTrace, error) {
	return nil, nil
}
func (m *mockStorage) ListReplayTraces(_ context.Context, _ storage.TraceFilters) ([]*storage.ReplayTrace, error) {
	return nil, nil
}
func (m *mockStorage) CreateIngestionBatch(_ context.Context, _ []*storage.OTELTrace, _ []*storage.ReplayTrace, _ []*storage.ToolCapture) (storage.IngestCounts, error) {
	return storage.IngestCounts{}, nil
}
func (m *mockStorage) CreateToolCapture(_ context.Context, _ *storage.ToolCapture) error { return nil }
func (m *mockStorage) GetToolCaptureByArgs(_ context.Context, _ string, _ string) (*storage.ToolCapture, error) {
	return nil, nil
}
func (m *mockStorage) ListExperiments(_ context.Context, _ storage.ExperimentFilters) ([]*storage.Experiment, error) {
	return nil, nil
}
func (m *mockStorage) CreateEvaluator(_ context.Context, _ *storage.Evaluator) error   { return nil }
func (m *mockStorage) GetEvaluator(_ context.Context, _ int) (*storage.Evaluator, error) { return nil, nil }
func (m *mockStorage) GetEvaluatorByName(_ context.Context, _ string) (*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStorage) UpdateEvaluator(_ context.Context, _ *storage.Evaluator) error { return nil }
func (m *mockStorage) ListEvaluators(_ context.Context, _ bool) ([]*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStorage) DeleteEvaluator(_ context.Context, _ int) error                     { return nil }
func (m *mockStorage) CreateEvaluationRun(_ context.Context, _ *storage.EvaluationRun) error { return nil }
func (m *mockStorage) GetEvaluationRun(_ context.Context, _ uuid.UUID) (*storage.EvaluationRun, error) {
	return nil, nil
}
func (m *mockStorage) UpdateEvaluationRun(_ context.Context, _ *storage.EvaluationRun) error { return nil }
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
func (m *mockStorage) MarkTraceAsBaseline(_ context.Context, _ *storage.Baseline) error { return nil }
func (m *mockStorage) GetBaseline(_ context.Context, _ string) (*storage.Baseline, error) {
	return nil, nil
}
func (m *mockStorage) ListBaselines(_ context.Context) ([]*storage.Baseline, error) { return nil, nil }
func (m *mockStorage) UnmarkBaseline(_ context.Context, _ string) error             { return nil }
func (m *mockStorage) CreateDriftResult(_ context.Context, _ *storage.DriftResult) error { return nil }
func (m *mockStorage) GetDriftResults(_ context.Context, _ string) ([]*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStorage) GetDriftResultsByBaseline(_ context.Context, _ string, _ int) ([]*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStorage) GetLatestDriftResult(_ context.Context, _ string) (*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStorage) ListDriftResults(_ context.Context, _ int) ([]*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStorage) HasDriftResultForBaseline(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
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

// --- Tests ---

func TestHandleGateCheck_202(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "trace-abc", 2)
	completer := &mockCompleter{}

	srv := newTestServer(t, store, completer, 5)

	body, _ := json.Marshal(gateCheckRequest{
		BaselineTraceID: "trace-abc",
		Model:           "gpt-4o",
		Threshold:       0.8,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleGateCheck(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp gateCheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "running", resp.Status)
	assert.NotEmpty(t, resp.ExperimentID)

	// Verify experiment was created
	expID, err := uuid.Parse(resp.ExperimentID)
	require.NoError(t, err)
	_, err = store.GetExperiment(context.Background(), expID)
	require.NoError(t, err)
}

func TestHandleGateCheck_MissingFields_400(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	tests := []struct {
		name string
		body gateCheckRequest
	}{
		{"missing baseline", gateCheckRequest{Model: "gpt-4o"}},
		{"missing model", gateCheckRequest{BaselineTraceID: "trace-abc"}},
		{"bad threshold", gateCheckRequest{BaselineTraceID: "trace-abc", Model: "gpt-4o", Threshold: 1.5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
			w := httptest.NewRecorder()

			srv.handleGateCheck(w, req)
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

	body, _ := json.Marshal(gateCheckRequest{
		BaselineTraceID: "trace-abc",
		Model:           "gpt-4o",
		Threshold:       0.8,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate/check", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handleGateCheck(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Drain semaphore
	<-srv.sem
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
	req.SetPathValue("id", expID.String())
	w := httptest.NewRecorder()

	srv.handleGateStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp gateStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "running", resp.Status)
	assert.Equal(t, 0.5, resp.Progress)
}

func TestHandleGateStatus_NotFound(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	fakeID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate/status/"+fakeID.String(), nil)
	req.SetPathValue("id", fakeID.String())
	w := httptest.NewRecorder()

	srv.handleGateStatus(w, req)
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
			BehaviorDiff:    storage.JSONB{"verdict": "pass"},
		},
	}

	srv := newTestServer(t, store, &mockCompleter{}, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate/report/"+expID.String(), nil)
	req.SetPathValue("id", expID.String())
	w := httptest.NewRecorder()

	srv.handleGateReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp gateReportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "completed", resp.Status)
	assert.Equal(t, "pass", resp.Verdict)
	require.NotNil(t, resp.SimilarityScore)
	assert.Equal(t, 0.92, *resp.SimilarityScore)
	assert.Len(t, resp.Runs, 2)
}

func TestHandleHealth(t *testing.T) {
	store := newMockStorage()
	srv := newTestServer(t, store, &mockCompleter{}, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	srv.handleHealth(w, req)

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

	srv.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "degraded", resp["status"])
}
