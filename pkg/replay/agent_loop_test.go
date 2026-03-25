package replay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
	"github.com/lethaltrifecta/replay/pkg/otelreceiver"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// --- Mock ToolExecutor ---

type mockToolExecutor struct {
	results []ToolResult
	errors  []error
	calls   int
	closed  bool
}

func (m *mockToolExecutor) CallTool(_ context.Context, _ string, _ map[string]any) (ToolResult, error) {
	idx := m.calls
	m.calls++
	if idx < len(m.errors) && m.errors[idx] != nil {
		return ToolResult{}, m.errors[idx]
	}
	if idx < len(m.results) {
		return m.results[idx], nil
	}
	return ToolResult{Content: "{}", LatencyMS: 10}, nil
}

func (m *mockToolExecutor) Close() error {
	m.closed = true
	return nil
}

// --- Helper ---

func toolCallResponse(id, name, args string) agwclient.ToolCallResponse {
	return agwclient.ToolCallResponse{
		ID:   id,
		Type: "function",
		Function: agwclient.FunctionCall{
			Name:      name,
			Arguments: args,
		},
	}
}

func completionWithToolCalls(model string, content string, toolCalls []agwclient.ToolCallResponse, tokens int) *agwclient.CompletionResponse {
	return &agwclient.CompletionResponse{
		ID:    "resp-" + model,
		Model: model,
		Choices: []agwclient.Choice{{
			Message: agwclient.ChatMessage{
				Role:      "assistant",
				Content:   content,
				ToolCalls: toolCalls,
			},
		}},
		Usage: agwclient.Usage{PromptTokens: tokens / 2, CompletionTokens: tokens / 2, TotalTokens: tokens},
	}
}

func completionNoTools(model, content string, tokens int) *agwclient.CompletionResponse {
	return completionWithToolCalls(model, content, nil, tokens)
}

// --- Tests ---

func TestAgentLoop_HappyPath_NoToolCalls(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			completionNoTools("gpt-4o", "Hello!", 80),
		},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{}
	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})

	require.NoError(t, err)
	assert.Equal(t, 1, result.TurnsExecuted)
	assert.Equal(t, "no_tool_calls", result.StopReason)
	assert.Len(t, result.Divergences, 0)
	assert.Len(t, result.Steps, 1)
	assert.Equal(t, "Hello!", result.Steps[0].Completion)
	assert.Equal(t, "agent_loop", result.Steps[0].Metadata["source"])
	assert.Equal(t, 0, toolExec.calls)

	// Experiment should be completed
	exp := store.experiments[result.ExperimentID]
	require.NotNil(t, exp)
	assert.Equal(t, storage.StatusCompleted, exp.Status)
}

func TestAgentLoop_MultiTurnWithTools(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, storage.JSONB{
			"messages": []any{
				map[string]any{"role": "system", "content": "Use tools."},
				map[string]any{"role": "user", "content": "Check weather"},
			},
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name":        "get_weather",
						"description": "Get weather",
						"parameters":  map[string]any{"type": "object"},
					},
				},
			},
		}, 100),
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			// Turn 0: calls tool
			completionWithToolCalls("gpt-4o", "", []agwclient.ToolCallResponse{
				toolCallResponse("call-1", "get_weather", `{"city":"SF"}`),
			}, 100),
			// Turn 1: final answer
			completionNoTools("gpt-4o", "It's sunny in SF!", 80),
		},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{
		results: []ToolResult{
			{Content: `{"temp":72,"condition":"sunny"}`, LatencyMS: 50},
		},
	}

	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})

	require.NoError(t, err)
	assert.Equal(t, 2, result.TurnsExecuted)
	assert.Equal(t, "no_tool_calls", result.StopReason)
	assert.Len(t, result.Steps, 2)
	assert.Equal(t, 1, toolExec.calls)

	// Verify messages accumulate: completer should have seen tool result in turn 1
	require.Len(t, completer.requests, 2)
	turn1Msgs := completer.requests[1].Messages
	// Should have: system, user, assistant (with tool_calls), tool result
	assert.Len(t, turn1Msgs, 4)
	assert.Equal(t, "tool", turn1Msgs[3].Role)
	assert.Equal(t, "call-1", turn1Msgs[3].ToolCallID)

	// Verify tool_calls in metadata of turn 0
	require.NotNil(t, result.Steps[0].Metadata["tool_calls"])

	// Experiment completed
	exp := store.experiments[result.ExperimentID]
	assert.Equal(t, storage.StatusCompleted, exp.Status)
}

func TestAgentLoop_Divergence_ToolNotCaptured(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, storage.JSONB{
			"messages": []any{
				map[string]any{"role": "user", "content": "Do something"},
			},
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "some_tool",
					},
				},
			},
		}, 100),
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			completionWithToolCalls("gpt-4o", "", []agwclient.ToolCallResponse{
				toolCallResponse("call-1", "some_tool", `{"x":1}`),
			}, 100),
		},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{
		results: []ToolResult{
			{
				Content:   `{"error":{"type":"tool_not_captured","message":"no capture found"}}`,
				IsError:   true,
				ErrorType: "tool_not_captured",
				LatencyMS: 5,
			},
		},
	}

	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})

	// Divergence does NOT fail the experiment
	require.NoError(t, err)
	assert.Equal(t, "divergence", result.StopReason)
	assert.Equal(t, 1, result.TurnsExecuted)
	require.Len(t, result.Divergences, 1)
	assert.Equal(t, "some_tool", result.Divergences[0].ToolName)
	assert.Equal(t, "tool_not_captured", result.Divergences[0].ErrorType)

	// Experiment should be completed (not failed)
	exp := store.experiments[result.ExperimentID]
	assert.Equal(t, storage.StatusCompleted, exp.Status)
}

func TestAgentLoop_CapturedToolError_ContinuesLoop(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, storage.JSONB{
			"messages": []any{
				map[string]any{"role": "user", "content": "Do something"},
			},
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "failing_tool",
					},
				},
			},
		}, 100),
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			// Turn 0: calls tool that returns captured_tool_error
			completionWithToolCalls("gpt-4o", "", []agwclient.ToolCallResponse{
				toolCallResponse("call-1", "failing_tool", `{}`),
			}, 100),
			// Turn 1: LLM handles the error and gives final answer
			completionNoTools("gpt-4o", "The tool failed but I handled it.", 60),
		},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{
		results: []ToolResult{
			{
				Content:   `{"error":{"type":"captured_tool_error","message":"original error"}}`,
				IsError:   true,
				ErrorType: "captured_tool_error",
				LatencyMS: 5,
			},
		},
	}

	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})

	require.NoError(t, err)
	assert.Equal(t, 2, result.TurnsExecuted)
	assert.Equal(t, "no_tool_calls", result.StopReason)
	assert.Len(t, result.Divergences, 0) // captured_tool_error is not a divergence
}

func TestAgentLoop_MaxTurnsExhaustion(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, storage.JSONB{
			"messages": []any{
				map[string]any{"role": "user", "content": "Loop forever"},
			},
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "loop_tool",
					},
				},
			},
		}, 100),
	}

	// Every turn calls a tool, never stops
	responses := make([]*agwclient.CompletionResponse, 3)
	for i := range responses {
		responses[i] = completionWithToolCalls("gpt-4o", "", []agwclient.ToolCallResponse{
			toolCallResponse(fmt.Sprintf("call-%d", i+1), "loop_tool", `{}`),
		}, 80)
	}

	completer := &mockCompleter{responses: responses}
	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{}
	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 3})

	require.NoError(t, err)
	assert.Equal(t, 3, result.TurnsExecuted)
	assert.Equal(t, "max_turns", result.StopReason)
}

func TestAgentLoop_LLMError_FailsExperiment(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
	}

	completer := &mockCompleter{
		errors: []error{errors.New("model overloaded")},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{}
	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm call turn 0")

	exp := store.experiments[result.ExperimentID]
	assert.Equal(t, storage.StatusFailed, exp.Status)
}

func TestAgentLoop_ToolTransportError_FailsExperiment(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, storage.JSONB{
			"messages": []any{
				map[string]any{"role": "user", "content": "Do something"},
			},
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "some_tool",
					},
				},
			},
		}, 100),
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			completionWithToolCalls("gpt-4o", "", []agwclient.ToolCallResponse{
				toolCallResponse("call-1", "some_tool", `{}`),
			}, 100),
		},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{
		errors: []error{errors.New("connection refused")},
	}

	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool call some_tool turn 0")

	exp := store.experiments[result.ExperimentID]
	assert.Equal(t, storage.StatusFailed, exp.Status)
}

func TestAgentLoop_ContextCancellation(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, defaultPrompt(), 100),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	engine := NewEngine(store, &mockCompleter{})
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{}
	_, err = engine.ExecuteAgentLoop(ctx, prepared, toolExec, AgentLoopConfig{MaxTurns: 8})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestAgentLoop_EmptyBaseline(t *testing.T) {
	store := newMockStorage()
	// No traces

	engine := NewEngine(store, &mockCompleter{})
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.Error(t, err)
	assert.ErrorIs(t, err, storage.ErrTraceNotFound)
	assert.Nil(t, prepared)
}

func TestAgentLoop_MetadataFormat(t *testing.T) {
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, storage.JSONB{
			"messages": []any{
				map[string]any{"role": "user", "content": "Hello"},
			},
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "my_tool",
					},
				},
			},
		}, 100),
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			completionWithToolCalls("gpt-4o", "Calling tool", []agwclient.ToolCallResponse{
				toolCallResponse("call-1", "my_tool", `{"a":1}`),
			}, 100),
			completionNoTools("gpt-4o", "Done", 60),
		},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{
		results: []ToolResult{{Content: `{"result":"ok"}`, LatencyMS: 10}},
	}

	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})
	require.NoError(t, err)

	// Verify metadata on turn 0
	step0Meta := result.Steps[0].Metadata
	assert.Equal(t, "agent_loop", step0Meta["source"])
	assert.Equal(t, "baseline-trace-123", step0Meta["baseline_trace_id"])
	assert.NotNil(t, step0Meta["tool_calls"])
	assert.NotNil(t, step0Meta["replay_tool_names"])

	// Turn 0 should have turn=0 in metadata
	turnVal, ok := step0Meta["turn"]
	require.True(t, ok)

	// Accept both int and json.Number (depends on serialization path)
	switch v := turnVal.(type) {
	case int:
		assert.Equal(t, 0, v)
	case json.Number:
		n, _ := v.Int64()
		assert.Equal(t, int64(0), n)
	case float64:
		assert.Equal(t, float64(0), v)
	default:
		t.Fatalf("unexpected turn type: %T", turnVal)
	}
}

// --- locatorMockToolExecutor ---

// locatorCall records a single SetLocator invocation.
type locatorCall struct {
	SpanID    string
	StepIndex int
}

// locatorMockToolExecutor is a mockToolExecutor that also implements ToolLocator.
type locatorMockToolExecutor struct {
	mockToolExecutor
	setLocatorCalls   []locatorCall
	clearLocatorCalls int
}

func (m *locatorMockToolExecutor) SetLocator(spanID string, stepIndex int) {
	m.setLocatorCalls = append(m.setLocatorCalls, locatorCall{SpanID: spanID, StepIndex: stepIndex})
}

func (m *locatorMockToolExecutor) ClearLocator() {
	m.clearLocatorCalls++
}

func TestAgentLoop_LocatorMatchesBaselineCaptures(t *testing.T) {
	// Baseline calls get_weather twice with the same args but different results.
	// The agent loop should set the correct locator (span_id, step_index) for each.
	argsJSON := storage.JSONB{"city": "SF"}
	argsHash := otelreceiver.CalculateCaptureArgsHash(argsJSON)

	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, storage.JSONB{
			"messages": []any{
				map[string]any{"role": "user", "content": "Check weather twice"},
			},
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "get_weather",
					},
				},
			},
		}, 100),
	}

	// Seed two baseline captures for the same tool+args
	store.toolCaptures = map[string][]*storage.ToolCapture{
		"baseline-trace-123": {
			{
				TraceID:   "baseline-trace-123",
				SpanID:    "span-aaa",
				StepIndex: 0,
				ToolName:  "get_weather",
				ArgsHash:  argsHash,
				Result:    storage.JSONB{"temp": 72},
			},
			{
				TraceID:   "baseline-trace-123",
				SpanID:    "span-bbb",
				StepIndex: 1,
				ToolName:  "get_weather",
				ArgsHash:  argsHash,
				Result:    storage.JSONB{"temp": 68},
			},
		},
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			// Turn 0: calls get_weather twice
			completionWithToolCalls("gpt-4o", "", []agwclient.ToolCallResponse{
				toolCallResponse("call-1", "get_weather", `{"city":"SF"}`),
				toolCallResponse("call-2", "get_weather", `{"city":"SF"}`),
			}, 100),
			// Turn 1: final answer
			completionNoTools("gpt-4o", "Weather checked!", 60),
		},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &locatorMockToolExecutor{
		mockToolExecutor: mockToolExecutor{
			results: []ToolResult{
				{Content: `{"temp":72}`, LatencyMS: 10},
				{Content: `{"temp":68}`, LatencyMS: 10},
			},
		},
	}

	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})
	require.NoError(t, err)
	assert.Equal(t, 2, result.TurnsExecuted)
	assert.Equal(t, "no_tool_calls", result.StopReason)

	// Verify SetLocator was called twice with the correct span/step
	require.Len(t, toolExec.setLocatorCalls, 2)
	assert.Equal(t, locatorCall{SpanID: "span-aaa", StepIndex: 0}, toolExec.setLocatorCalls[0])
	assert.Equal(t, locatorCall{SpanID: "span-bbb", StepIndex: 1}, toolExec.setLocatorCalls[1])

	// Verify ClearLocator was called after each tool call
	assert.Equal(t, 2, toolExec.clearLocatorCalls)
}

func TestAgentLoop_BaselineExhausted_Diverges(t *testing.T) {
	// Baseline has 1 capture for get_weather({"city":"SF"}).
	// Variant calls it twice → second call should trigger baseline_exhausted divergence.
	argsJSON := storage.JSONB{"city": "SF"}
	argsHash := otelreceiver.CalculateCaptureArgsHash(argsJSON)

	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, storage.JSONB{
			"messages": []any{
				map[string]any{"role": "user", "content": "Check weather"},
			},
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "get_weather",
					},
				},
			},
		}, 100),
	}

	// Only 1 baseline capture
	store.toolCaptures = map[string][]*storage.ToolCapture{
		"baseline-trace-123": {
			{
				TraceID:  "baseline-trace-123",
				SpanID:   "span-aaa",
				ToolName: "get_weather",
				ArgsHash: argsHash,
			},
		},
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			// Turn 0: calls get_weather twice in one turn
			completionWithToolCalls("gpt-4o", "", []agwclient.ToolCallResponse{
				toolCallResponse("call-1", "get_weather", `{"city":"SF"}`),
				toolCallResponse("call-2", "get_weather", `{"city":"SF"}`),
			}, 100),
		},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{
		results: []ToolResult{
			{Content: `{"temp":72}`, LatencyMS: 10},
		},
	}

	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})
	require.NoError(t, err)
	assert.Equal(t, "divergence", result.StopReason)
	require.Len(t, result.Divergences, 1)
	assert.Equal(t, "get_weather", result.Divergences[0].ToolName)
	assert.Equal(t, "baseline_exhausted", result.Divergences[0].ErrorType)
	// Only the first tool call should have been executed
	assert.Equal(t, 1, toolExec.calls)
}

func TestAgentLoop_MultiToolTurn_DistinctCaptures(t *testing.T) {
	// A single turn with 2 tool calls should persist both captures
	// with distinct SpanIDs (no unique constraint collision).
	store := newMockStorage()
	store.replayTraces["baseline-trace-123"] = []*storage.ReplayTrace{
		makeBaselineStep(0, storage.JSONB{
			"messages": []any{
				map[string]any{"role": "user", "content": "Do two things"},
			},
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "tool_a",
					},
				},
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "tool_b",
					},
				},
			},
		}, 100),
	}

	completer := &mockCompleter{
		responses: []*agwclient.CompletionResponse{
			completionWithToolCalls("gpt-4o", "", []agwclient.ToolCallResponse{
				toolCallResponse("call-1", "tool_a", `{"x":1}`),
				toolCallResponse("call-2", "tool_b", `{"y":2}`),
			}, 100),
			completionNoTools("gpt-4o", "Done", 60),
		},
	}

	engine := NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-trace-123", VariantConfig{Model: "gpt-4o"}, 0.8)
	require.NoError(t, err)

	toolExec := &mockToolExecutor{
		results: []ToolResult{
			{Content: `{"a":"ok"}`, LatencyMS: 10},
			{Content: `{"b":"ok"}`, LatencyMS: 10},
		},
	}

	result, err := engine.ExecuteAgentLoop(context.Background(), prepared, toolExec, AgentLoopConfig{MaxTurns: 8})
	require.NoError(t, err)
	assert.Equal(t, 2, result.TurnsExecuted)
	assert.Equal(t, 2, toolExec.calls)

	// Verify both captures were stored with distinct SpanIDs
	var captureSpanIDs []string
	for _, cap := range store.createdCaptures {
		captureSpanIDs = append(captureSpanIDs, cap.SpanID)
	}
	require.Len(t, captureSpanIDs, 2, "both tool captures should be persisted")
	assert.NotEqual(t, captureSpanIDs[0], captureSpanIDs[1], "captures should have distinct SpanIDs")
	// Both should reference the same turn
	assert.Equal(t, 0, store.createdCaptures[0].StepIndex)
	assert.Equal(t, 0, store.createdCaptures[1].StepIndex)
}

func TestCaptureQueue_Exhausted(t *testing.T) {
	q := &captureQueue{
		captures: []*storage.ToolCapture{
			{ToolName: "foo", ArgsHash: "hash1"},
			{ToolName: "foo", ArgsHash: "hash1"},
			{ToolName: "bar", ArgsHash: "hash2"},
		},
		used: make([]bool, 3),
	}

	// Not exhausted before any matches
	assert.False(t, q.exhausted("foo", "hash1"))

	// Consume first
	assert.NotNil(t, q.match("foo", "hash1"))
	assert.False(t, q.exhausted("foo", "hash1")) // still one left

	// Consume second
	assert.NotNil(t, q.match("foo", "hash1"))
	assert.True(t, q.exhausted("foo", "hash1")) // all consumed

	// Different tool is not exhausted
	assert.False(t, q.exhausted("bar", "hash2"))

	// Tool that was never in queue
	assert.False(t, q.exhausted("baz", "hash3"))

	// Nil receiver
	var nilQ *captureQueue
	assert.False(t, nilQ.exhausted("foo", "hash1"))
}
