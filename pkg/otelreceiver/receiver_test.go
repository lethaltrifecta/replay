package otelreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/lethaltrifecta/replay/pkg/utils/logger"
)

// Mock storage for testing
type mockStorage struct {
	otelTraces    []*storage.OTELTrace
	replayTraces  []*storage.ReplayTrace
	toolCaptures  []*storage.ToolCapture
}

func (m *mockStorage) CreateOTELTrace(ctx context.Context, trace *storage.OTELTrace) error {
	m.otelTraces = append(m.otelTraces, trace)
	return nil
}

func (m *mockStorage) CreateReplayTrace(ctx context.Context, trace *storage.ReplayTrace) error {
	m.replayTraces = append(m.replayTraces, trace)
	return nil
}

func (m *mockStorage) CreateToolCapture(ctx context.Context, capture *storage.ToolCapture) error {
	m.toolCaptures = append(m.toolCaptures, capture)
	return nil
}

// Implement other required methods (stubs)
func (m *mockStorage) Close() error                                                                        { return nil }
func (m *mockStorage) Ping(ctx context.Context) error                                                      { return nil }
func (m *mockStorage) Migrate(ctx context.Context) error                                                   { return nil }
func (m *mockStorage) GetOTELTraceSpans(ctx context.Context, traceID string) ([]*storage.OTELTrace, error) { return nil, nil }
func (m *mockStorage) GetReplayTraceSpans(ctx context.Context, traceID string) ([]*storage.ReplayTrace, error) { return nil, nil }
func (m *mockStorage) ListReplayTraces(ctx context.Context, filters storage.TraceFilters) ([]*storage.ReplayTrace, error) { return nil, nil }
func (m *mockStorage) GetToolCapturesByTrace(ctx context.Context, traceID string) ([]*storage.ToolCapture, error) { return nil, nil }
func (m *mockStorage) GetToolCaptureByArgs(ctx context.Context, toolName string, argsHash string) (*storage.ToolCapture, error) { return nil, nil }
func (m *mockStorage) CreateExperiment(ctx context.Context, exp *storage.Experiment) error                { return nil }
func (m *mockStorage) GetExperiment(ctx context.Context, id uuid.UUID) (*storage.Experiment, error)       { return nil, nil }
func (m *mockStorage) UpdateExperiment(ctx context.Context, exp *storage.Experiment) error                { return nil }
func (m *mockStorage) ListExperiments(ctx context.Context, filters storage.ExperimentFilters) ([]*storage.Experiment, error) { return nil, nil }
func (m *mockStorage) CreateExperimentRun(ctx context.Context, run *storage.ExperimentRun) error          { return nil }
func (m *mockStorage) GetExperimentRun(ctx context.Context, id uuid.UUID) (*storage.ExperimentRun, error) { return nil, nil }
func (m *mockStorage) UpdateExperimentRun(ctx context.Context, run *storage.ExperimentRun) error          { return nil }
func (m *mockStorage) ListExperimentRuns(ctx context.Context, experimentID uuid.UUID) ([]*storage.ExperimentRun, error) { return nil, nil }
func (m *mockStorage) CreateAnalysisResult(ctx context.Context, result *storage.AnalysisResult) error     { return nil }
func (m *mockStorage) GetAnalysisResults(ctx context.Context, experimentID uuid.UUID) ([]*storage.AnalysisResult, error) { return nil, nil }
func (m *mockStorage) CreateEvaluator(ctx context.Context, evaluator *storage.Evaluator) error            { return nil }
func (m *mockStorage) GetEvaluator(ctx context.Context, id int) (*storage.Evaluator, error)               { return nil, nil }
func (m *mockStorage) GetEvaluatorByName(ctx context.Context, name string) (*storage.Evaluator, error)    { return nil, nil }
func (m *mockStorage) UpdateEvaluator(ctx context.Context, evaluator *storage.Evaluator) error            { return nil }
func (m *mockStorage) ListEvaluators(ctx context.Context, enabledOnly bool) ([]*storage.Evaluator, error) { return nil, nil }
func (m *mockStorage) DeleteEvaluator(ctx context.Context, id int) error                                  { return nil }
func (m *mockStorage) CreateEvaluationRun(ctx context.Context, run *storage.EvaluationRun) error          { return nil }
func (m *mockStorage) GetEvaluationRun(ctx context.Context, id uuid.UUID) (*storage.EvaluationRun, error) { return nil, nil }
func (m *mockStorage) UpdateEvaluationRun(ctx context.Context, run *storage.EvaluationRun) error          { return nil }
func (m *mockStorage) CreateEvaluatorResult(ctx context.Context, result *storage.EvaluatorResult) error   { return nil }
func (m *mockStorage) GetEvaluatorResults(ctx context.Context, evaluationRunID uuid.UUID) ([]*storage.EvaluatorResult, error) { return nil, nil }
func (m *mockStorage) CreateHumanEvaluation(ctx context.Context, eval *storage.HumanEvaluation) error     { return nil }
func (m *mockStorage) GetHumanEvaluation(ctx context.Context, id uuid.UUID) (*storage.HumanEvaluation, error) { return nil, nil }
func (m *mockStorage) UpdateHumanEvaluation(ctx context.Context, eval *storage.HumanEvaluation) error     { return nil }
func (m *mockStorage) ListPendingHumanEvaluations(ctx context.Context, assignedTo *string) ([]*storage.HumanEvaluation, error) { return nil, nil }
func (m *mockStorage) CreateGroundTruth(ctx context.Context, gt *storage.GroundTruth) error               { return nil }
func (m *mockStorage) GetGroundTruth(ctx context.Context, taskID string) (*storage.GroundTruth, error)    { return nil, nil }
func (m *mockStorage) UpdateGroundTruth(ctx context.Context, gt *storage.GroundTruth) error               { return nil }
func (m *mockStorage) ListGroundTruth(ctx context.Context, taskType *string) ([]*storage.GroundTruth, error) { return nil, nil }
func (m *mockStorage) DeleteGroundTruth(ctx context.Context, taskID string) error                         { return nil }
func (m *mockStorage) CreateEvaluationSummary(ctx context.Context, summary *storage.EvaluationSummary) error { return nil }
func (m *mockStorage) GetEvaluationSummary(ctx context.Context, experimentID uuid.UUID) ([]*storage.EvaluationSummary, error) { return nil, nil }
func (m *mockStorage) MarkTraceAsBaseline(ctx context.Context, baseline *storage.Baseline) error   { return nil }
func (m *mockStorage) GetBaseline(ctx context.Context, traceID string) (*storage.Baseline, error)  { return nil, nil }
func (m *mockStorage) ListBaselines(ctx context.Context) ([]*storage.Baseline, error)              { return nil, nil }
func (m *mockStorage) UnmarkBaseline(ctx context.Context, traceID string) error                    { return nil }
func (m *mockStorage) CreateDriftResult(ctx context.Context, result *storage.DriftResult) error     { return nil }
func (m *mockStorage) GetDriftResults(ctx context.Context, traceID string) ([]*storage.DriftResult, error) { return nil, nil }
func (m *mockStorage) GetDriftResultsByBaseline(ctx context.Context, baselineTraceID string) ([]*storage.DriftResult, error) { return nil, nil }
func (m *mockStorage) GetLatestDriftResult(ctx context.Context, traceID string) (*storage.DriftResult, error) { return nil, nil }
func (m *mockStorage) ListDriftResults(ctx context.Context, limit int) ([]*storage.DriftResult, error) { return nil, nil }
func (m *mockStorage) HasDriftResultForBaseline(ctx context.Context, traceID string, baselineTraceID string) (bool, error) { return false, nil }

func TestParser_IsLLMSpan(t *testing.T) {
	log, _ := logger.New("debug")
	parser := NewParser(log)

	t.Run("span with gen_ai attributes", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.Attributes().PutStr("gen_ai.request.model", "claude-3-5-sonnet")
		span.Attributes().PutStr("gen_ai.system", "anthropic")

		assert.True(t, parser.IsLLMSpan(span))
	})

	t.Run("span without gen_ai attributes", func(t *testing.T) {
		span := ptrace.NewSpan()
		span.Attributes().PutStr("http.method", "GET")
		span.Attributes().PutStr("http.url", "https://example.com")

		assert.False(t, parser.IsLLMSpan(span))
	})
}

func TestParser_ParseLLMSpan(t *testing.T) {
	log, _ := logger.New("debug")
	parser := NewParser(log)

	span := ptrace.NewSpan()
	span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Second)))

	// Add LLM attributes
	span.Attributes().PutStr("gen_ai.request.model", "claude-3-5-sonnet-20241022")
	span.Attributes().PutStr("gen_ai.system", "anthropic")
	span.Attributes().PutStr("gen_ai.prompt.0.role", "user")
	span.Attributes().PutStr("gen_ai.prompt.0.content", "Hello, how are you?")
	span.Attributes().PutStr("gen_ai.completion.0.content", "I'm doing great!")
	span.Attributes().PutDouble("gen_ai.request.temperature", 0.7)
	span.Attributes().PutInt("gen_ai.usage.input_tokens", 10)
	span.Attributes().PutInt("gen_ai.usage.output_tokens", 5)

	resource := pcommon.NewResource()
	resource.Attributes().PutStr("service.name", "test-service")

	trace := parser.ParseLLMSpan(span, resource)

	require.NotNil(t, trace)
	assert.Equal(t, "anthropic", trace.Provider)
	assert.Equal(t, "claude-3-5-sonnet-20241022", trace.Model)
	assert.Equal(t, "0102030405060708", trace.SpanID)
	assert.Equal(t, 10, trace.PromptTokens)
	assert.Equal(t, 5, trace.CompletionTokens)
	assert.Equal(t, 15, trace.TotalTokens)
	assert.Equal(t, "I'm doing great!", trace.Completion)
}

func TestReceiver_New(t *testing.T) {
	log, _ := logger.New("debug")
	mock := &mockStorage{}

	receiver, err := NewReceiver(Config{}, mock, log)

	require.NoError(t, err)
	assert.NotNil(t, receiver)
	assert.NotNil(t, receiver.parser)
}

func TestCalculateArgsHash_Deterministic(t *testing.T) {
	// Same logical content with different Go types should produce the same hash
	t.Run("int vs float64 for whole numbers", func(t *testing.T) {
		args1 := storage.JSONB{"count": int(5), "name": "test"}
		args2 := storage.JSONB{"count": float64(5), "name": "test"}

		hash1 := calculateArgsHash(args1)
		hash2 := calculateArgsHash(args2)

		assert.Equal(t, hash1, hash2, "int(5) and float64(5) should produce the same hash")
		assert.NotEmpty(t, hash1)
	})

	t.Run("int64 vs float64", func(t *testing.T) {
		args1 := storage.JSONB{"value": int64(42)}
		args2 := storage.JSONB{"value": float64(42)}

		assert.Equal(t, calculateArgsHash(args1), calculateArgsHash(args2))
	})

	t.Run("nested maps produce consistent hashes", func(t *testing.T) {
		args1 := storage.JSONB{
			"outer": map[string]interface{}{
				"b": "second",
				"a": "first",
			},
		}
		args2 := storage.JSONB{
			"outer": map[string]interface{}{
				"a": "first",
				"b": "second",
			},
		}

		assert.Equal(t, calculateArgsHash(args1), calculateArgsHash(args2))
	})

	t.Run("different values produce different hashes", func(t *testing.T) {
		args1 := storage.JSONB{"query": "hello"}
		args2 := storage.JSONB{"query": "world"}

		assert.NotEqual(t, calculateArgsHash(args1), calculateArgsHash(args2))
	})

	t.Run("nested JSONB type is normalized", func(t *testing.T) {
		args1 := storage.JSONB{
			"config": storage.JSONB{"key": int(1)},
		}
		args2 := storage.JSONB{
			"config": map[string]interface{}{"key": float64(1)},
		}

		assert.Equal(t, calculateArgsHash(args1), calculateArgsHash(args2))
	})

	t.Run("empty args produce consistent hash", func(t *testing.T) {
		args1 := storage.JSONB{}
		args2 := storage.JSONB{}

		hash := calculateArgsHash(args1)
		assert.Equal(t, hash, calculateArgsHash(args2))
		assert.NotEmpty(t, hash)
	})
}
