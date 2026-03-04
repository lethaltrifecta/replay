package replay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// --- Mock Completer ---

type mockCompleter struct {
	responses []*agwclient.CompletionResponse
	errors    []error
	calls     int
}

func (m *mockCompleter) Complete(ctx context.Context, req *agwclient.CompletionRequest) (*agwclient.CompletionResponse, error) {
	idx := m.calls
	m.calls++
	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return &agwclient.CompletionResponse{}, nil
}

// --- Mock Storage ---

type mockStorage struct {
	replayTraces    map[string][]*storage.ReplayTrace
	experiments     map[uuid.UUID]*storage.Experiment
	experimentRuns  map[uuid.UUID]*storage.ExperimentRun
	createdReplays  []*storage.ReplayTrace
	analysisResults []*storage.AnalysisResult

	createExperimentRunHook func(run *storage.ExperimentRun) error
	updateExperimentHook    func(exp *storage.Experiment, call int) error
	updateExperimentRunHook func(run *storage.ExperimentRun, call int) error
	updateExperimentCalls   int
	updateRunCalls          int
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		replayTraces:   make(map[string][]*storage.ReplayTrace),
		experiments:    make(map[uuid.UUID]*storage.Experiment),
		experimentRuns: make(map[uuid.UUID]*storage.ExperimentRun),
	}
}

func (m *mockStorage) GetReplayTraceSpans(_ context.Context, traceID string) ([]*storage.ReplayTrace, error) {
	return m.replayTraces[traceID], nil
}

func (m *mockStorage) CreateExperiment(_ context.Context, exp *storage.Experiment) error {
	m.experiments[exp.ID] = exp
	return nil
}

func (m *mockStorage) UpdateExperiment(_ context.Context, exp *storage.Experiment) error {
	m.updateExperimentCalls++
	if m.updateExperimentHook != nil {
		if err := m.updateExperimentHook(exp, m.updateExperimentCalls); err != nil {
			return err
		}
	}
	m.experiments[exp.ID] = exp
	return nil
}

func (m *mockStorage) GetExperiment(_ context.Context, id uuid.UUID) (*storage.Experiment, error) {
	if e, ok := m.experiments[id]; ok {
		return e, nil
	}
	return nil, errors.New("not found")
}

func (m *mockStorage) CreateExperimentRun(_ context.Context, run *storage.ExperimentRun) error {
	if m.createExperimentRunHook != nil {
		if err := m.createExperimentRunHook(run); err != nil {
			return err
		}
	}
	m.experimentRuns[run.ID] = run
	return nil
}

func (m *mockStorage) UpdateExperimentRun(_ context.Context, run *storage.ExperimentRun) error {
	m.updateRunCalls++
	if m.updateExperimentRunHook != nil {
		if err := m.updateExperimentRunHook(run, m.updateRunCalls); err != nil {
			return err
		}
	}
	m.experimentRuns[run.ID] = run
	return nil
}

func (m *mockStorage) GetExperimentRun(_ context.Context, id uuid.UUID) (*storage.ExperimentRun, error) {
	if r, ok := m.experimentRuns[id]; ok {
		return r, nil
	}
	return nil, errors.New("not found")
}

func (m *mockStorage) CreateReplayTrace(_ context.Context, trace *storage.ReplayTrace) error {
	m.createdReplays = append(m.createdReplays, trace)
	// Also add to the lookup map so GetReplayTraceSpans works for variant traces
	m.replayTraces[trace.TraceID] = append(m.replayTraces[trace.TraceID], trace)
	return nil
}

func (m *mockStorage) CreateAnalysisResult(_ context.Context, result *storage.AnalysisResult) error {
	m.analysisResults = append(m.analysisResults, result)
	return nil
}

// Stub remaining Storage interface methods (not exercised by engine tests)

func (m *mockStorage) Close() error                                                  { return nil }
func (m *mockStorage) Ping(_ context.Context) error                                  { return nil }
func (m *mockStorage) Migrate(_ context.Context) error                               { return nil }
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
func (m *mockStorage) GetToolCapturesByTrace(_ context.Context, _ string) ([]*storage.ToolCapture, error) {
	return nil, nil
}
func (m *mockStorage) GetToolCaptureByArgs(_ context.Context, _ string, _ string) (*storage.ToolCapture, error) {
	return nil, nil
}
func (m *mockStorage) ListExperiments(_ context.Context, _ storage.ExperimentFilters) ([]*storage.Experiment, error) {
	return nil, nil
}
func (m *mockStorage) ListExperimentRuns(_ context.Context, _ uuid.UUID) ([]*storage.ExperimentRun, error) {
	return nil, nil
}
func (m *mockStorage) GetAnalysisResults(_ context.Context, _ uuid.UUID) ([]*storage.AnalysisResult, error) {
	return nil, nil
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
func (m *mockStorage) MarkTraceAsBaseline(_ context.Context, _ *storage.Baseline) error { return nil }
func (m *mockStorage) GetBaseline(_ context.Context, _ string) (*storage.Baseline, error) {
	return nil, nil
}
func (m *mockStorage) ListBaselines(_ context.Context) ([]*storage.Baseline, error) { return nil, nil }
func (m *mockStorage) UnmarkBaseline(_ context.Context, _ string) error             { return nil }
func (m *mockStorage) CreateDriftResult(_ context.Context, _ *storage.DriftResult) error {
	return nil
}
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

// --- Tests ---

func makeBaselineStep(stepIndex int, prompt storage.JSONB, tokens int) *storage.ReplayTrace {
	return &storage.ReplayTrace{
		TraceID:          "baseline-trace-123",
		SpanID:           uuid.New().String(),
		RunID:            "baseline-trace-123",
		StepIndex:        stepIndex,
		CreatedAt:        time.Now(),
		Provider:         "openai",
		Model:            "gpt-4",
		Prompt:           prompt,
		Completion:       "baseline response",
		PromptTokens:     tokens / 2,
		CompletionTokens: tokens / 2,
		TotalTokens:      tokens,
		LatencyMS:        200,
	}
}

func defaultPrompt() storage.JSONB {
	return storage.JSONB{
		"messages": []interface{}{
			map[string]interface{}{"role": "system", "content": "You are helpful."},
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	}
}

func TestEngine_Run_HappyPath(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
		makeBaselineStep(1, defaultPrompt(), 150),
		makeBaselineStep(2, defaultPrompt(), 120),
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			{ID: "r1", Model: "gpt-4o", Choices: []agwclient.Choice{{Message: agwclient.ChatMessage{Role: "assistant", Content: "Hi 1"}}}, Usage: agwclient.Usage{PromptTokens: 50, CompletionTokens: 60, TotalTokens: 110}},
			{ID: "r2", Model: "gpt-4o", Choices: []agwclient.Choice{{Message: agwclient.ChatMessage{Role: "assistant", Content: "Hi 2"}}}, Usage: agwclient.Usage{PromptTokens: 70, CompletionTokens: 80, TotalTokens: 150}},
			{ID: "r3", Model: "gpt-4o", Choices: []agwclient.Choice{{Message: agwclient.ChatMessage{Role: "assistant", Content: "Hi 3"}}}, Usage: agwclient.Usage{PromptTokens: 55, CompletionTokens: 65, TotalTokens: 120}},
		},
	}

	engine := NewEngine(store, completer)
	result, err := engine.Run(context.Background(), "baseline-trace-123", VariantConfig{
		Model:    "gpt-4o",
		Provider: "openai",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 3, completer.calls)
	assert.Len(t, result.Steps, 3)
	assert.NotEmpty(t, result.VariantTraceID)
	assert.NotEqual(t, uuid.Nil, result.ExperimentID)

	// Verify experiment was created and completed
	exp := store.experiments[result.ExperimentID]
	require.NotNil(t, exp)
	assert.Equal(t, storage.StatusCompleted, exp.Status)
	assert.Equal(t, 1.0, exp.Progress)

	// Verify variant run completed with trace ID
	variantRun := store.experimentRuns[result.VariantRunID]
	require.NotNil(t, variantRun)
	assert.Equal(t, storage.StatusCompleted, variantRun.Status)
	require.NotNil(t, variantRun.TraceID)
	assert.Equal(t, result.VariantTraceID, *variantRun.TraceID)

	// Verify replay traces were stored
	assert.Len(t, store.createdReplays, 3)
	for i, rt := range store.createdReplays {
		assert.Equal(t, result.VariantTraceID, rt.TraceID)
		assert.Equal(t, i, rt.StepIndex)
		assert.Equal(t, "gpt-4o", rt.Model)
	}
}

func TestEngine_Run_StepFailure(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
		makeBaselineStep(1, defaultPrompt(), 150),
		makeBaselineStep(2, defaultPrompt(), 120),
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			{ID: "r1", Model: "gpt-4o", Choices: []agwclient.Choice{{Message: agwclient.ChatMessage{Role: "assistant", Content: "Hi"}}}, Usage: agwclient.Usage{TotalTokens: 100}},
		},
		errors: []error{nil, errors.New("model overloaded"), nil},
	}

	engine := NewEngine(store, completer)
	result, err := engine.Run(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "replay step 1")
	assert.NotNil(t, result)
	assert.Len(t, result.Steps, 1) // Only first step succeeded

	// Experiment should be marked failed
	exp := store.experiments[result.ExperimentID]
	require.NotNil(t, exp)
	assert.Equal(t, storage.StatusFailed, exp.Status)
}

func TestEngine_Run_EmptyBaseline(t *testing.T) {
	store := newMockStorage()
	// No traces for this ID

	engine := NewEngine(store, &mockCompleter{})
	_, err := engine.Run(context.Background(), "nonexistent-trace", VariantConfig{Model: "gpt-4o"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no replay traces found")
}

func TestEngine_Run_ContextCancellation(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
		makeBaselineStep(1, defaultPrompt(), 150),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	engine := NewEngine(store, &mockCompleter{})
	_, err := engine.Run(ctx, "baseline-trace-123", VariantConfig{Model: "gpt-4o"})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEngine_Run_StepFailure_CleanupStillMarksExperimentFailedWhenRunUpdateFails(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
	}
	store.updateExperimentRunHook = func(_ *storage.ExperimentRun, _ int) error {
		return errors.New("update run failed")
	}

	completer := &mockCompleter{
		errors: []error{errors.New("model overloaded")},
	}

	engine := NewEngine(store, completer)
	result, err := engine.Run(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.Contains(t, err.Error(), "cleanup")

	exp := store.experiments[result.ExperimentID]
	require.NotNil(t, exp)
	assert.Equal(t, storage.StatusFailed, exp.Status)
	assert.NotNil(t, exp.CompletedAt)
	assert.GreaterOrEqual(t, store.updateRunCalls, 1)
	assert.GreaterOrEqual(t, store.updateExperimentCalls, 1)
}

func TestEngine_Run_FinalizeRunUpdateFailureTriggersCleanup(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
	}
	store.updateExperimentRunHook = func(_ *storage.ExperimentRun, call int) error {
		if call == 1 {
			return errors.New("finalize run update failed")
		}
		return nil
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			{
				ID:      "ok-1",
				Model:   "gpt-4o",
				Choices: []agwclient.Choice{{Message: agwclient.ChatMessage{Role: "assistant", Content: "ok"}}},
				Usage:   agwclient.Usage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20},
			},
		},
	}

	engine := NewEngine(store, completer)
	result, err := engine.Run(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.Contains(t, err.Error(), "finalize lifecycle")

	exp := store.experiments[result.ExperimentID]
	require.NotNil(t, exp)
	assert.Equal(t, storage.StatusFailed, exp.Status)
	assert.NotNil(t, exp.CompletedAt)

	run := store.experimentRuns[result.VariantRunID]
	require.NotNil(t, run)
	assert.Equal(t, storage.StatusFailed, run.Status)
	assert.NotNil(t, run.CompletedAt)
}

func TestEngine_Run_FinalizeExperimentUpdateFailureTriggersCleanup(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
	}
	store.updateExperimentHook = func(_ *storage.Experiment, call int) error {
		// call 1: progress update, call 2: finalize experiment update
		if call == 2 {
			return errors.New("finalize experiment update failed")
		}
		return nil
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			{
				ID:      "ok-1",
				Model:   "gpt-4o",
				Choices: []agwclient.Choice{{Message: agwclient.ChatMessage{Role: "assistant", Content: "ok"}}},
				Usage:   agwclient.Usage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20},
			},
		},
	}

	engine := NewEngine(store, completer)
	result, err := engine.Run(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.Contains(t, err.Error(), "finalize lifecycle")

	exp := store.experiments[result.ExperimentID]
	require.NotNil(t, exp)
	assert.Equal(t, storage.StatusFailed, exp.Status)
	assert.NotNil(t, exp.CompletedAt)

	run := store.experimentRuns[result.VariantRunID]
	require.NotNil(t, run)
	assert.Equal(t, storage.StatusFailed, run.Status)
	assert.NotNil(t, run.CompletedAt)
}

func TestEngine_Run_VariantRunCreationFailureMarksExperimentFailed(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
	}
	store.createExperimentRunHook = func(run *storage.ExperimentRun) error {
		if run.RunType == storage.RunTypeVariant {
			return errors.New("variant run create failed")
		}
		return nil
	}

	engine := NewEngine(store, &mockCompleter{})
	_, err := engine.Run(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create variant run")

	// There should be exactly one experiment, and it should be failed.
	require.Len(t, store.experiments, 1)
	for _, exp := range store.experiments {
		assert.Equal(t, storage.StatusFailed, exp.Status)
		assert.NotNil(t, exp.CompletedAt)
	}
}

func TestExtractMessages(t *testing.T) {
	tests := []struct {
		name    string
		prompt  storage.JSONB
		want    int
		wantErr bool
	}{
		{
			name: "valid messages",
			prompt: storage.JSONB{
				"messages": []interface{}{
					map[string]interface{}{"role": "user", "content": "Hello"},
				},
			},
			want: 1,
		},
		{
			name: "multiple messages",
			prompt: storage.JSONB{
				"messages": []interface{}{
					map[string]interface{}{"role": "system", "content": "Be helpful"},
					map[string]interface{}{"role": "user", "content": "Hello"},
				},
			},
			want: 2,
		},
		{
			name:    "missing messages key",
			prompt:  storage.JSONB{"other": "data"},
			wantErr: true,
		},
		{
			name:    "empty prompt",
			prompt:  storage.JSONB{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs, err := extractMessages(tt.prompt)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, msgs, tt.want)
			}
		})
	}
}
