package otelreceiver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/lethaltrifecta/replay/pkg/utils/logger"
)

// Mock storage for testing
type mockStorage struct {
	otelTraces   []*storage.OTELTrace
	replayTraces []*storage.ReplayTrace
	toolCaptures []*storage.ToolCapture
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
func (m *mockStorage) Close() error                      { return nil }
func (m *mockStorage) Ping(ctx context.Context) error    { return nil }
func (m *mockStorage) Migrate(ctx context.Context) error { return nil }
func (m *mockStorage) GetOTELTraceSpans(ctx context.Context, traceID string) ([]*storage.OTELTrace, error) {
	return nil, nil
}
func (m *mockStorage) GetReplayTraceSpans(ctx context.Context, traceID string) ([]*storage.ReplayTrace, error) {
	return nil, nil
}
func (m *mockStorage) ListReplayTraces(ctx context.Context, filters storage.TraceFilters) ([]*storage.ReplayTrace, error) {
	return nil, nil
}
func (m *mockStorage) ListUniqueTraces(ctx context.Context, filters storage.TraceFilters) ([]*storage.TraceSummary, error) {
	return nil, nil
}
func (m *mockStorage) GetToolCapturesByTrace(ctx context.Context, traceID string) ([]*storage.ToolCapture, error) {
	return nil, nil
}
func (m *mockStorage) GetToolCaptureByArgs(ctx context.Context, toolName string, argsHash string) (*storage.ToolCapture, error) {
	return nil, nil
}
func (m *mockStorage) CreateExperiment(ctx context.Context, exp *storage.Experiment) error {
	return nil
}
func (m *mockStorage) GetExperiment(ctx context.Context, id uuid.UUID) (*storage.Experiment, error) {
	return nil, nil
}
func (m *mockStorage) UpdateExperiment(ctx context.Context, exp *storage.Experiment) error {
	return nil
}
func (m *mockStorage) ListExperiments(ctx context.Context, filters storage.ExperimentFilters) ([]*storage.Experiment, error) {
	return nil, nil
}
func (m *mockStorage) CreateExperimentRun(ctx context.Context, run *storage.ExperimentRun) error {
	return nil
}
func (m *mockStorage) GetExperimentRun(ctx context.Context, id uuid.UUID) (*storage.ExperimentRun, error) {
	return nil, nil
}
func (m *mockStorage) UpdateExperimentRun(ctx context.Context, run *storage.ExperimentRun) error {
	return nil
}
func (m *mockStorage) ListExperimentRuns(ctx context.Context, experimentID uuid.UUID) ([]*storage.ExperimentRun, error) {
	return nil, nil
}
func (m *mockStorage) CreateAnalysisResult(ctx context.Context, result *storage.AnalysisResult) error {
	return nil
}
func (m *mockStorage) GetAnalysisResults(ctx context.Context, experimentID uuid.UUID) ([]*storage.AnalysisResult, error) {
	return nil, nil
}
func (m *mockStorage) CreateEvaluator(ctx context.Context, evaluator *storage.Evaluator) error {
	return nil
}
func (m *mockStorage) GetEvaluator(ctx context.Context, id int) (*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStorage) GetEvaluatorByName(ctx context.Context, name string) (*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStorage) UpdateEvaluator(ctx context.Context, evaluator *storage.Evaluator) error {
	return nil
}
func (m *mockStorage) ListEvaluators(ctx context.Context, enabledOnly bool) ([]*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStorage) DeleteEvaluator(ctx context.Context, id int) error { return nil }
func (m *mockStorage) CreateEvaluationRun(ctx context.Context, run *storage.EvaluationRun) error {
	return nil
}
func (m *mockStorage) GetEvaluationRun(ctx context.Context, id uuid.UUID) (*storage.EvaluationRun, error) {
	return nil, nil
}
func (m *mockStorage) UpdateEvaluationRun(ctx context.Context, run *storage.EvaluationRun) error {
	return nil
}
func (m *mockStorage) CreateEvaluatorResult(ctx context.Context, result *storage.EvaluatorResult) error {
	return nil
}
func (m *mockStorage) GetEvaluatorResults(ctx context.Context, evaluationRunID uuid.UUID) ([]*storage.EvaluatorResult, error) {
	return nil, nil
}
func (m *mockStorage) CreateHumanEvaluation(ctx context.Context, eval *storage.HumanEvaluation) error {
	return nil
}
func (m *mockStorage) GetHumanEvaluation(ctx context.Context, id uuid.UUID) (*storage.HumanEvaluation, error) {
	return nil, nil
}
func (m *mockStorage) UpdateHumanEvaluation(ctx context.Context, eval *storage.HumanEvaluation) error {
	return nil
}
func (m *mockStorage) ListPendingHumanEvaluations(ctx context.Context, assignedTo *string) ([]*storage.HumanEvaluation, error) {
	return nil, nil
}
func (m *mockStorage) CreateGroundTruth(ctx context.Context, gt *storage.GroundTruth) error {
	return nil
}
func (m *mockStorage) GetGroundTruth(ctx context.Context, taskID string) (*storage.GroundTruth, error) {
	return nil, nil
}
func (m *mockStorage) UpdateGroundTruth(ctx context.Context, gt *storage.GroundTruth) error {
	return nil
}
func (m *mockStorage) ListGroundTruth(ctx context.Context, taskType *string) ([]*storage.GroundTruth, error) {
	return nil, nil
}
func (m *mockStorage) DeleteGroundTruth(ctx context.Context, taskID string) error { return nil }
func (m *mockStorage) CreateEvaluationSummary(ctx context.Context, summary *storage.EvaluationSummary) error {
	return nil
}
func (m *mockStorage) GetEvaluationSummary(ctx context.Context, experimentID uuid.UUID) ([]*storage.EvaluationSummary, error) {
	return nil, nil
}
func (m *mockStorage) MarkTraceAsBaseline(ctx context.Context, baseline *storage.Baseline) error {
	return nil
}
func (m *mockStorage) GetBaseline(ctx context.Context, traceID string) (*storage.Baseline, error) {
	return nil, nil
}
func (m *mockStorage) ListBaselines(ctx context.Context) ([]*storage.Baseline, error) {
	return nil, nil
}
func (m *mockStorage) UnmarkBaseline(ctx context.Context, traceID string) error { return nil }
func (m *mockStorage) CreateDriftResult(ctx context.Context, result *storage.DriftResult) error {
	return nil
}
func (m *mockStorage) GetDriftResults(ctx context.Context, traceID string) ([]*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStorage) GetDriftResultsByBaseline(ctx context.Context, baselineTraceID string, limit int) ([]*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStorage) GetLatestDriftResult(ctx context.Context, traceID string) (*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStorage) GetDriftResultForPair(ctx context.Context, traceID string, baselineTraceID string) (*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStorage) ListDriftResults(ctx context.Context, limit int, offset int) ([]*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStorage) HasDriftResultForBaseline(ctx context.Context, traceID string, baselineTraceID string) (bool, error) {
	return false, nil
}

// Batch operations
func (m *mockStorage) CreateIngestionBatch(ctx context.Context, otels []*storage.OTELTrace, replays []*storage.ReplayTrace, tools []*storage.ToolCapture) (storage.IngestCounts, error) {
	m.otelTraces = append(m.otelTraces, otels...)
	m.replayTraces = append(m.replayTraces, replays...)
	m.toolCaptures = append(m.toolCaptures, tools...)
	return storage.IngestCounts{
		OTELTraces:   int64(len(otels)),
		ReplayTraces: int64(len(replays)),
		ToolCaptures: int64(len(tools)),
	}, nil
}

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

func TestParser_ParseLLMSpan_AgentgatewayNativeAttributes(t *testing.T) {
	log, _ := logger.New("debug")
	parser := NewParser(log)

	span := ptrace.NewSpan()
	span.SetTraceID(pcommon.TraceID([16]byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4}))
	span.SetSpanID(pcommon.SpanID([8]byte{8, 7, 6, 5, 4, 3, 2, 1}))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Second)))

	span.Attributes().PutStr("gen_ai.request.model", "gpt-4o-mini")
	span.Attributes().PutStr("gen_ai.provider.name", "openai")
	span.Attributes().PutStr("gen_ai.prompt.0.role", "user")
	span.Attributes().PutStr("gen_ai.prompt.0.content", "Summarize the outage.")
	span.Attributes().PutStr("gen_ai.completion.0.content", "The outage appears to be database-related.")
	span.Attributes().PutInt("gen_ai.usage.input_tokens", 12)
	span.Attributes().PutInt("gen_ai.usage.output_tokens", 9)

	trace := parser.ParseLLMSpan(span, pcommon.NewResource())

	require.NotNil(t, trace)
	assert.Equal(t, "openai", trace.Provider)
	assert.Equal(t, "gpt-4o-mini", trace.Model)
	assert.Equal(t, 12, trace.PromptTokens)
	assert.Equal(t, 9, trace.CompletionTokens)
	assert.Equal(t, 21, trace.TotalTokens)
}

func TestParser_ParseLLMSpan_AgentgatewayTokenAliases(t *testing.T) {
	log, _ := logger.New("debug")
	parser := NewParser(log)

	span := ptrace.NewSpan()
	span.SetTraceID(pcommon.TraceID([16]byte{4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}))
	span.SetSpanID(pcommon.SpanID([8]byte{1, 3, 5, 7, 9, 2, 4, 6}))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Second)))

	span.Attributes().PutStr("gen_ai.request.model", "gemini-1.5-flash")
	span.Attributes().PutStr("gen_ai.system", "gemini")
	span.Attributes().PutStr("gen_ai.prompt.0.role", "user")
	span.Attributes().PutStr("gen_ai.prompt.0.content", "Check deployment health.")
	span.Attributes().PutStr("gen_ai.completion.0.content", "I will inspect the deployment.")
	span.Attributes().PutInt("gen_ai.usage.prompt_tokens", 14)
	span.Attributes().PutInt("gen_ai.usage.completion_tokens", 6)

	trace := parser.ParseLLMSpan(span, pcommon.NewResource())

	require.NotNil(t, trace)
	assert.Equal(t, "gemini", trace.Provider)
	assert.Equal(t, 14, trace.PromptTokens)
	assert.Equal(t, 6, trace.CompletionTokens)
	assert.Equal(t, 20, trace.TotalTokens)
}

func TestParser_ParseLLMSpan_PreservesReplayToolMetadata(t *testing.T) {
	log, _ := logger.New("debug")
	parser := NewParser(log)

	span := ptrace.NewSpan()
	span.SetTraceID(pcommon.TraceID([16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}))
	span.SetSpanID(pcommon.SpanID([8]byte{2, 2, 2, 2, 2, 2, 2, 2}))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Second)))

	span.Attributes().PutStr("gen_ai.request.model", "gpt-4o")
	span.Attributes().PutStr("gen_ai.system", "openai")
	span.Attributes().PutStr("gen_ai.prompt.0.role", "tool")
	span.Attributes().PutStr("gen_ai.prompt.0.content", `{"result":4}`)
	span.Attributes().PutStr("gen_ai.prompt.0.name", "calculator")
	span.Attributes().PutStr("gen_ai.prompt.0.tool_call_id", "call-123")
	span.Attributes().PutStr("gen_ai.prompt.0.tool_calls", `[{"id":"call-123","type":"function","function":{"name":"calculator","arguments":"{\"a\":2,\"b\":2}"}}]`)
	span.Attributes().PutStr("gen_ai.request.tools", `[{"type":"function","function":{"name":"calculator","description":"Add numbers","parameters":{"type":"object"}}}]`)
	span.Attributes().PutStr("gen_ai.request.tool_choice", `{"type":"function","function":{"name":"calculator"}}`)
	span.Attributes().PutStr("gen_ai.completion.0.content", "Using calculator")

	trace := parser.ParseLLMSpan(span, pcommon.NewResource())

	require.NotNil(t, trace)
	messages, ok := trace.Prompt["messages"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, messages, 1)
	assert.Equal(t, "calculator", messages[0]["name"])
	assert.Equal(t, "call-123", messages[0]["tool_call_id"])

	tools, ok := trace.Prompt["tools"].([]interface{})
	require.True(t, ok)
	require.Len(t, tools, 1)

	toolChoice, ok := trace.Prompt["tool_choice"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "function", toolChoice["type"])
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

	t.Run("large ints remain distinct", func(t *testing.T) {
		args1 := storage.JSONB{"n": int64(1 << 53)}
		args2 := storage.JSONB{"n": int64(1<<53 + 1)}

		assert.NotEqual(t, calculateArgsHash(args1), calculateArgsHash(args2))
	})
}

func TestReceiverHandleHTTPTraces_JSON(t *testing.T) {
	log, _ := logger.New("debug")
	mock := &mockStorage{}
	receiver, err := NewReceiver(Config{}, mock, log)
	require.NoError(t, err)

	traces := buildHTTPTestTraces()
	body, err := (&ptrace.JSONMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	receiver.handleHTTPTraces(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Len(t, mock.otelTraces, 1)

	resp := ptraceotlp.NewExportResponse()
	require.NoError(t, resp.UnmarshalJSON(rr.Body.Bytes()))
}

func TestReceiverHandleHTTPTraces_Protobuf(t *testing.T) {
	log, _ := logger.New("debug")
	mock := &mockStorage{}
	receiver, err := NewReceiver(Config{}, mock, log)
	require.NoError(t, err)

	traces := buildHTTPTestTraces()
	body, err := (&ptrace.ProtoMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	rr := httptest.NewRecorder()

	receiver.handleHTTPTraces(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-protobuf", rr.Header().Get("Content-Type"))
	assert.Len(t, mock.otelTraces, 1)

	resp := ptraceotlp.NewExportResponse()
	require.NoError(t, resp.UnmarshalProto(rr.Body.Bytes()))
}

func TestReceiverHandleHTTPTraces_UnsupportedContentType(t *testing.T) {
	log, _ := logger.New("debug")
	mock := &mockStorage{}
	receiver, err := NewReceiver(Config{}, mock, log)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()

	receiver.handleHTTPTraces(rr, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rr.Code)
	assert.Len(t, mock.otelTraces, 0)
}

func TestReceiverHandleHTTPTraces_JSONWithCharset(t *testing.T) {
	log, _ := logger.New("debug")
	mock := &mockStorage{}
	receiver, err := NewReceiver(Config{}, mock, log)
	require.NoError(t, err)

	traces := buildHTTPTestTraces()
	body, err := (&ptrace.JSONMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rr := httptest.NewRecorder()

	receiver.handleHTTPTraces(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Len(t, mock.otelTraces, 1)
}

func TestReceiverHandleHTTPTraces_NoContentType_ProtobufFallback(t *testing.T) {
	log, _ := logger.New("debug")
	mock := &mockStorage{}
	receiver, err := NewReceiver(Config{}, mock, log)
	require.NoError(t, err)

	traces := buildHTTPTestTraces()
	body, err := (&ptrace.ProtoMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	receiver.handleHTTPTraces(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-protobuf", rr.Header().Get("Content-Type"))
	assert.Len(t, mock.otelTraces, 1)

	resp := ptraceotlp.NewExportResponse()
	require.NoError(t, resp.UnmarshalProto(rr.Body.Bytes()))
}

func TestReceiverHandleHTTPTraces_NoContentType_JSONFallback(t *testing.T) {
	log, _ := logger.New("debug")
	mock := &mockStorage{}
	receiver, err := NewReceiver(Config{}, mock, log)
	require.NoError(t, err)

	traces := buildHTTPTestTraces()
	body, err := (&ptrace.JSONMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	receiver.handleHTTPTraces(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Len(t, mock.otelTraces, 1)

	resp := ptraceotlp.NewExportResponse()
	require.NoError(t, resp.UnmarshalJSON(rr.Body.Bytes()))
}

func buildHTTPTestTraces() ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "receiver-http-test")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	span.SetTraceID(pcommon.TraceID([16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}))
	span.SetSpanID(pcommon.SpanID([8]byte{2, 2, 2, 2, 2, 2, 2, 2}))
	span.SetName("test-span")
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(10 * time.Millisecond)))

	return traces
}
