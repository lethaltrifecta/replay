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
