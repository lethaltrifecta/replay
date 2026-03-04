package diff

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

func makeTrace(stepIndex, totalTokens, latencyMS int) *storage.ReplayTrace {
	return &storage.ReplayTrace{
		StepIndex:   stepIndex,
		TotalTokens: totalTokens,
		LatencyMS:   latencyMS,
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		name            string
		baseline        []*storage.ReplayTrace
		variant         []*storage.ReplayTrace
		threshold       float64
		wantVerdict     string
		wantScoreMin    float64
		wantScoreMax    float64
		wantStepCount   int
		wantTokenDelta  int
		wantLatencyDelta int
	}{
		{
			name: "identical traces",
			baseline: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
				makeTrace(1, 150, 300),
				makeTrace(2, 120, 250),
			},
			variant: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
				makeTrace(1, 150, 300),
				makeTrace(2, 120, 250),
			},
			threshold:        0.8,
			wantVerdict:      "pass",
			wantScoreMin:     1.0,
			wantScoreMax:     1.0,
			wantStepCount:    3,
			wantTokenDelta:   0,
			wantLatencyDelta: 0,
		},
		{
			name: "different step counts - fewer variant steps",
			baseline: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
				makeTrace(1, 150, 300),
				makeTrace(2, 120, 250),
				makeTrace(3, 130, 280),
			},
			variant: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
				makeTrace(1, 150, 300),
			},
			threshold:   0.8,
			wantVerdict: "fail",
			// stepSim = 1 - 2/4 = 0.5, tokenSim and latencySim < 1 due to missing steps
			wantScoreMin:     0.0,
			wantScoreMax:     0.79,
			wantStepCount:    2,
			wantTokenDelta:   -250, // (100+150) - (100+150+120+130)
			wantLatencyDelta: -530, // (200+300) - (200+300+250+280)
		},
		{
			name: "different step counts - more variant steps",
			baseline: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
			},
			variant: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
				makeTrace(1, 150, 300),
				makeTrace(2, 120, 250),
			},
			threshold:        0.8,
			wantVerdict:      "fail",
			wantScoreMin:     0.0,
			wantScoreMax:     0.79,
			wantStepCount:    3,
			wantTokenDelta:   270,
			wantLatencyDelta: 550,
		},
		{
			name: "large token delta",
			baseline: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
			},
			variant: []*storage.ReplayTrace{
				makeTrace(0, 10000, 200),
			},
			threshold:        0.8,
			wantVerdict:      "fail",
			wantScoreMin:     0.0,
			wantScoreMax:     0.79,
			wantStepCount:    1,
			wantTokenDelta:   9900,
			wantLatencyDelta: 0,
		},
		{
			name: "threshold edge - just at threshold",
			baseline: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
				makeTrace(1, 100, 200),
			},
			variant: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
				makeTrace(1, 100, 200),
			},
			threshold:   1.0, // exact match required
			wantVerdict: "pass",
			wantScoreMin: 1.0,
			wantScoreMax: 1.0,
			wantStepCount:    2,
			wantTokenDelta:   0,
			wantLatencyDelta: 0,
		},
		{
			name: "threshold edge - just below threshold",
			baseline: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
				makeTrace(1, 150, 300),
			},
			variant: []*storage.ReplayTrace{
				makeTrace(0, 110, 210),
				makeTrace(1, 160, 310),
			},
			threshold:   1.0, // exact match required
			wantVerdict: "fail",
			wantScoreMin: 0.8,
			wantScoreMax: 0.99,
			wantStepCount:    2,
			wantTokenDelta:   20,
			wantLatencyDelta: 20,
		},
		{
			name:     "empty baseline and variant",
			baseline: []*storage.ReplayTrace{},
			variant:  []*storage.ReplayTrace{},
			threshold:        0.8,
			wantVerdict:      "pass",
			wantScoreMin:     1.0,
			wantScoreMax:     1.0,
			wantStepCount:    0,
			wantTokenDelta:   0,
			wantLatencyDelta: 0,
		},
		{
			name: "low threshold passes moderate diff",
			baseline: []*storage.ReplayTrace{
				makeTrace(0, 100, 200),
				makeTrace(1, 200, 400),
			},
			variant: []*storage.ReplayTrace{
				makeTrace(0, 150, 300),
				makeTrace(1, 250, 500),
			},
			threshold:        0.5,
			wantVerdict:      "pass",
			wantScoreMin:     0.5,
			wantScoreMax:     1.0,
			wantStepCount:    2,
			wantTokenDelta:   100,
			wantLatencyDelta: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{SimilarityThreshold: tt.threshold}
			report := Compare(tt.baseline, tt.variant, cfg)

			assert.Equal(t, tt.wantVerdict, report.Verdict)
			assert.GreaterOrEqual(t, report.SimilarityScore, tt.wantScoreMin,
				"score %.4f should be >= %.4f", report.SimilarityScore, tt.wantScoreMin)
			assert.LessOrEqual(t, report.SimilarityScore, tt.wantScoreMax,
				"score %.4f should be <= %.4f", report.SimilarityScore, tt.wantScoreMax)
			assert.Equal(t, tt.wantStepCount, report.StepCount)
			assert.Equal(t, tt.wantTokenDelta, report.TokenDelta)
			assert.Equal(t, tt.wantLatencyDelta, report.LatencyDelta)
		})
	}
}

func TestToAnalysisResult(t *testing.T) {
	report := &Report{
		SimilarityScore: 0.92,
		Verdict:         "pass",
		StepCount:       3,
		StepDiffs: []StepDiff{
			{StepIndex: 0, TokenDelta: 10},
			{StepIndex: 1, TokenDelta: -5},
			{StepIndex: 2, TokenDelta: 20},
		},
		TokenDelta:   25,
		LatencyDelta: 340,
	}

	experimentID := uuid.New()
	baselineRunID := uuid.New()
	candidateRunID := uuid.New()

	result := ToAnalysisResult(report, experimentID, baselineRunID, candidateRunID)

	require.NotNil(t, result)
	assert.Equal(t, experimentID, result.ExperimentID)
	assert.Equal(t, baselineRunID, result.BaselineRunID)
	assert.Equal(t, candidateRunID, result.CandidateRunID)
	assert.InDelta(t, 0.92, result.SimilarityScore, 0.001)
	assert.Equal(t, 25, result.TokenDelta)
	assert.Equal(t, 340, result.LatencyDelta)
	assert.Equal(t, float64(0), result.CostDelta)
	assert.NotNil(t, result.BehaviorDiff)
	assert.Equal(t, "pass", result.BehaviorDiff["verdict"])
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.InDelta(t, 0.8, cfg.SimilarityThreshold, 0.001)
}

func makeTraceWithCompletion(stepIndex, totalTokens, latencyMS int, completion string) *storage.ReplayTrace {
	return &storage.ReplayTrace{
		StepIndex:   stepIndex,
		TotalTokens: totalTokens,
		LatencyMS:   latencyMS,
		Completion:  completion,
	}
}

func TestCompareAll(t *testing.T) {
	baseline := []*storage.ReplayTrace{
		makeTraceWithCompletion(0, 100, 200, "Get the user profile"),
		makeTraceWithCompletion(1, 150, 300, "Update the record"),
	}
	variant := []*storage.ReplayTrace{
		makeTraceWithCompletion(0, 110, 210, "Get the user profile"),
		makeTraceWithCompletion(1, 140, 290, "Update the record"),
	}

	baselineCaptures := []*storage.ToolCapture{
		{ToolName: "get_user", RiskClass: "read"},
		{ToolName: "update_record", RiskClass: "write"},
	}
	variantTools := []ToolCall{
		{Name: "get_user", RiskClass: "read"},
		{Name: "update_record", RiskClass: "write"},
	}

	cfg := Config{SimilarityThreshold: 0.8}
	report := CompareAll(CompareInput{
		Baseline:      baseline,
		Variant:       variant,
		BaselineTools: baselineCaptures,
		VariantTools:  variantTools,
	}, cfg)

	require.NotNil(t, report)
	assert.Equal(t, "pass", report.Verdict)
	assert.GreaterOrEqual(t, report.SimilarityScore, 0.8)
	assert.LessOrEqual(t, report.SimilarityScore, 1.0)

	// All semantic dimensions should be populated
	require.NotNil(t, report.ToolCallScore)
	assert.InDelta(t, 1.0, report.ToolCallScore.Score, 0.01)

	require.NotNil(t, report.RiskScore)
	assert.InDelta(t, 1.0, report.RiskScore.Score, 0.01)
	assert.False(t, report.RiskScore.Escalation)

	require.NotNil(t, report.ResponseScore)
	assert.InDelta(t, 1.0, report.ResponseScore.Score, 0.01)
}

func TestCompareAll_NoTools(t *testing.T) {
	baseline := []*storage.ReplayTrace{
		makeTraceWithCompletion(0, 100, 200, "The quick brown fox jumps over the lazy dog"),
		makeTraceWithCompletion(1, 150, 300, "Hello world this is a test response"),
	}
	variant := []*storage.ReplayTrace{
		makeTraceWithCompletion(0, 110, 210, "The quick brown fox jumps over the lazy dog"),
		makeTraceWithCompletion(1, 140, 290, "Hello world this is a test response"),
	}

	cfg := Config{SimilarityThreshold: 0.8}
	report := CompareAll(CompareInput{
		Baseline:      baseline,
		Variant:       variant,
		BaselineTools: nil,
		VariantTools:  nil,
	}, cfg)

	require.NotNil(t, report)
	assert.Equal(t, "pass", report.Verdict)
	assert.GreaterOrEqual(t, report.SimilarityScore, 0.8)

	// Tool dimensions should be nil (no tool data)
	assert.Nil(t, report.ToolCallScore)
	assert.Nil(t, report.RiskScore)

	// Response score should still be populated
	require.NotNil(t, report.ResponseScore)
	assert.GreaterOrEqual(t, report.ResponseScore.Score, 0.9)
}

func TestCompareAll_WithEscalation(t *testing.T) {
	baseline := []*storage.ReplayTrace{
		makeTraceWithCompletion(0, 100, 200, "Read the file"),
	}
	variant := []*storage.ReplayTrace{
		makeTraceWithCompletion(0, 100, 200, "Delete the file"),
	}

	baselineCaptures := []*storage.ToolCapture{
		{ToolName: "read_file", RiskClass: "read"},
	}
	variantTools := []ToolCall{
		{Name: "delete_file", RiskClass: "destructive"},
	}

	cfg := Config{SimilarityThreshold: 0.8}
	report := CompareAll(CompareInput{
		Baseline:      baseline,
		Variant:       variant,
		BaselineTools: baselineCaptures,
		VariantTools:  variantTools,
	}, cfg)

	require.NotNil(t, report)
	require.NotNil(t, report.RiskScore)
	assert.True(t, report.RiskScore.Escalation)
	assert.Less(t, report.RiskScore.Score, 1.0)
}

func TestToAnalysisResult_WithSemanticData(t *testing.T) {
	report := &Report{
		SimilarityScore: 0.85,
		Verdict:         "pass",
		StepCount:       2,
		TokenDelta:      20,
		LatencyDelta:    30,
		ToolCallScore: &ToolCallScore{
			SequenceSimilarity:  0.9,
			FrequencySimilarity: 0.95,
			Score:               0.925,
		},
		RiskScore: &RiskScore{
			Score:      0.8,
			Escalation: true,
			Details: map[string]interface{}{
				"baseline_fractions":  map[string]float64{"read": 1.0},
				"candidate_fractions": map[string]float64{"write": 1.0},
			},
		},
		ResponseScore: &ResponseScore{
			LengthSimilarity: 0.9,
			ContentOverlap:   0.75,
			Score:            0.795,
		},
	}

	experimentID := uuid.New()
	baselineRunID := uuid.New()
	candidateRunID := uuid.New()

	result := ToAnalysisResult(report, experimentID, baselineRunID, candidateRunID)

	require.NotNil(t, result)
	assert.InDelta(t, 0.85, result.SimilarityScore, 0.001)

	// SafetyDiff should be populated
	assert.NotEmpty(t, result.SafetyDiff)
	assert.Equal(t, true, result.SafetyDiff["escalation"])

	// QualityMetrics should be populated
	assert.NotEmpty(t, result.QualityMetrics)
	assert.InDelta(t, 0.75, result.QualityMetrics["content_overlap"], 0.01)
	assert.InDelta(t, 0.925, result.QualityMetrics["tool_call_score"], 0.01)
}
