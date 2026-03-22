package commands

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

func TestBuildTraceComparisonReport(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	// Setup traces
	store.replayTraces = []*storage.ReplayTrace{
		{TraceID: "baseline-1", StepIndex: 0, Model: "gpt-4", Provider: "openai"},
		{TraceID: "candidate-1", StepIndex: 0, Model: "gpt-4o", Provider: "openai"},
	}
	store.toolCaptures["baseline-1"] = []*storage.ToolCapture{
		{TraceID: "baseline-1", ToolName: "test_tool", Args: storage.JSONB{"foo": "bar"}},
	}
	store.toolCaptures["candidate-1"] = []*storage.ToolCapture{
		{TraceID: "candidate-1", ToolName: "test_tool", Args: storage.JSONB{"foo": "bar"}},
	}

	report, err := buildTraceComparisonReport(ctx, store, "baseline-1", "candidate-1", 0.8)

	require.NoError(t, err)
	assert.Equal(t, 1, report.BaselineStepCount)
	assert.Equal(t, 1, report.CandidateStepCount)
	assert.Equal(t, 1, report.BaselineToolCount)
	assert.Equal(t, 1, report.CandidateToolCount)
	assert.Equal(t, traceToolSourceCaptures, report.CandidateToolSource)
}

func TestResolveCandidateToolComparisonInputs(t *testing.T) {
	t.Run("from captures", func(t *testing.T) {
		captures := []*storage.ToolCapture{
			{ToolName: "t1"},
		}
		tools, source := resolveCandidateToolComparisonInputs(nil, captures)
		assert.Len(t, tools, 1)
		assert.Equal(t, traceToolSourceCaptures, source)
	})

	t.Run("from metadata", func(t *testing.T) {
		steps := []*storage.ReplayTrace{
			{Metadata: storage.JSONB{"tool_calls": []interface{}{
				map[string]interface{}{
					"function": map[string]interface{}{
						"name": "t1",
					},
				},
			}}},
		}
		tools, source := resolveCandidateToolComparisonInputs(steps, nil)
		assert.Len(t, tools, 1)
		assert.Equal(t, traceToolSourceMetadata, source)
	})

	t.Run("none", func(t *testing.T) {
		tools, source := resolveCandidateToolComparisonInputs(nil, nil)
		assert.Nil(t, tools)
		assert.Equal(t, traceToolSourceNone, source)
	})
}
