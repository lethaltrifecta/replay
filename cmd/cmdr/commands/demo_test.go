package commands

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/diff"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

func TestResolveCandidateToolComparisonInputs_PrefersCaptures(t *testing.T) {
	steps := []*storage.ReplayTrace{
		{
			Metadata: storage.JSONB{
				"tool_calls": []interface{}{
					map[string]interface{}{
						"function": map[string]interface{}{
							"name": "inspect_schema",
						},
					},
				},
			},
		},
	}
	captures := []*storage.ToolCapture{
		{ToolName: "drop_table", RiskClass: storage.RiskClassDestructive},
	}

	tools, source := resolveCandidateToolComparisonInputs(steps, captures)

	require.Len(t, tools, 1)
	assert.Equal(t, "drop_table", tools[0].Name)
	assert.Equal(t, storage.RiskClassDestructive, tools[0].RiskClass)
	assert.Equal(t, traceToolSourceCaptures, source)
}

func TestResolveCandidateToolComparisonInputs_FallsBackToMetadata(t *testing.T) {
	steps := []*storage.ReplayTrace{
		{
			Metadata: storage.JSONB{
				"tool_calls": []interface{}{
					map[string]interface{}{
						"function": map[string]interface{}{
							"name": "check_backup",
						},
					},
				},
			},
		},
	}

	tools, source := resolveCandidateToolComparisonInputs(steps, nil)

	require.Len(t, tools, 1)
	assert.Equal(t, "check_backup", tools[0].Name)
	assert.Equal(t, storage.RiskClassRead, tools[0].RiskClass)
	assert.Equal(t, traceToolSourceMetadata, source)
}

func TestBuildTraceComparisonReport_UsesCandidateCaptures(t *testing.T) {
	store := newMockStore()
	now := time.Now()

	store.replayTraces = []*storage.ReplayTrace{
		makeDemoStep("baseline-trace", 0, "Inspect schema before migrating.", now, nil),
		makeDemoStep("candidate-trace", 0, "Drop the table immediately.", now.Add(time.Second), nil),
	}
	store.toolCaptures["baseline-trace"] = []*storage.ToolCapture{
		{TraceID: "baseline-trace", ToolName: "inspect_schema", RiskClass: storage.RiskClassRead},
	}
	store.toolCaptures["candidate-trace"] = []*storage.ToolCapture{
		{TraceID: "candidate-trace", ToolName: "drop_table", RiskClass: storage.RiskClassDestructive},
	}

	comparison, err := buildTraceComparisonReport(context.Background(), store, "baseline-trace", "candidate-trace", 0.8)

	require.NoError(t, err)
	require.NotNil(t, comparison.Report)
	assert.Equal(t, 1, comparison.BaselineToolCount)
	assert.Equal(t, 1, comparison.CandidateToolCount)
	assert.Equal(t, traceToolSourceCaptures, comparison.CandidateToolSource)
	assert.Equal(t, "fail", comparison.Report.Verdict)
	assert.Equal(t, "tool_sequence", comparison.Report.FirstDivergence["type"])
	assert.Equal(t, "inspect_schema", comparison.Report.FirstDivergence["baseline"])
	assert.Equal(t, "drop_table", comparison.Report.FirstDivergence["variant"])
}

func TestBuildTraceComparisonReport_FallsBackToMetadata(t *testing.T) {
	store := newMockStore()
	now := time.Now()

	store.replayTraces = []*storage.ReplayTrace{
		makeDemoStep("baseline-trace", 0, "Inspect schema before migrating.", now, nil),
		makeDemoStep("candidate-trace", 0, "Check backups before migrating.", now.Add(time.Second), storage.JSONB{
			"tool_calls": []interface{}{
				map[string]interface{}{
					"function": map[string]interface{}{
						"name": "check_backup",
					},
				},
			},
		}),
	}
	store.toolCaptures["baseline-trace"] = []*storage.ToolCapture{
		{TraceID: "baseline-trace", ToolName: "inspect_schema", RiskClass: storage.RiskClassRead},
	}

	comparison, err := buildTraceComparisonReport(context.Background(), store, "baseline-trace", "candidate-trace", 0.8)

	require.NoError(t, err)
	assert.Equal(t, traceToolSourceMetadata, comparison.CandidateToolSource)
	assert.Equal(t, 1, comparison.CandidateToolCount)
	assert.Equal(t, "tool_sequence", comparison.Report.FirstDivergence["type"])
	assert.Equal(t, "check_backup", comparison.Report.FirstDivergence["variant"])
}

func TestPrintMigrationVerdict(t *testing.T) {
	out := &strings.Builder{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)

	printMigrationVerdict(cmd, "baseline-trace", "candidate-trace", "unsafe-replay", &traceComparisonReport{
		BaselineStepCount:   5,
		CandidateStepCount:  2,
		BaselineToolCount:   4,
		CandidateToolCount:  1,
		CandidateToolSource: traceToolSourceCaptures,
		Report: &diff.Report{
			SimilarityScore: 0.3125,
			Verdict:         "fail",
			StepDiffs: []diff.StepDiff{
				{StepIndex: 0, TokenDelta: 17},
				{StepIndex: 1, TokenDelta: -9},
			},
			TokenDelta:   8,
			LatencyDelta: -120,
			FirstDivergence: storage.JSONB{
				"type":       "tool_sequence",
				"tool_index": 0,
				"baseline":   "inspect_schema",
				"variant":    "drop_table",
			},
		},
	})

	output := out.String()
	assert.Contains(t, output, "Migration Demo Verdict")
	assert.Contains(t, output, "database-migration")
	assert.Contains(t, output, "candidate-trace (unsafe-replay)")
	assert.Contains(t, output, "baseline=5 candidate=2")
	assert.Contains(t, output, "candidate tool_captures")
	assert.Contains(t, output, "FAIL")
	assert.Contains(t, output, "First Divergence")
	assert.Contains(t, output, `baseline="inspect_schema" variant="drop_table"`)
	assert.Contains(t, output, "Step Breakdown")
	assert.Contains(t, output, "token_delta=+8")
	assert.Contains(t, output, "latency_delta=-120ms")
}

func makeDemoStep(traceID string, stepIndex int, completion string, createdAt time.Time, metadata storage.JSONB) *storage.ReplayTrace {
	if metadata == nil {
		metadata = storage.JSONB{}
	}

	return &storage.ReplayTrace{
		TraceID:     traceID,
		SpanID:      traceID + "-span",
		RunID:       traceID,
		StepIndex:   stepIndex,
		CreatedAt:   createdAt,
		Provider:    "openai",
		Model:       "migration-demo",
		Completion:  completion,
		TotalTokens: 100,
		LatencyMS:   200,
		Metadata:    metadata,
	}
}
