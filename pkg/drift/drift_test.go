package drift

import (
	"math"
	"testing"

	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Extract tests ---

func TestExtract_BasicTrace(t *testing.T) {
	spans := []*storage.ReplayTrace{
		{Provider: "anthropic", Model: "claude-3-opus", PromptTokens: 600, CompletionTokens: 400, TotalTokens: 1000},
		{Provider: "anthropic", Model: "claude-3-opus", PromptTokens: 900, CompletionTokens: 600, TotalTokens: 1500},
		{Provider: "anthropic", Model: "claude-3-opus", PromptTokens: 300, CompletionTokens: 200, TotalTokens: 500},
	}
	tools := []*storage.ToolCapture{
		{ToolName: "search", RiskClass: "read"},
		{ToolName: "search", RiskClass: "read"},
		{ToolName: "write_file", RiskClass: "write"},
	}

	fp, err := Extract("trace-1", spans, tools)
	require.NoError(t, err)

	assert.Equal(t, "trace-1", fp.TraceID)
	assert.Equal(t, 3, fp.StepCount)
	assert.Equal(t, []string{"claude-3-opus", "claude-3-opus", "claude-3-opus"}, fp.Models)
	assert.Equal(t, []string{"anthropic", "anthropic", "anthropic"}, fp.Providers)
	assert.Equal(t, []string{"search", "search", "write_file"}, fp.ToolSequence)
	assert.Equal(t, map[string]int{"search": 2, "write_file": 1}, fp.ToolFrequency)
	assert.Equal(t, map[string]int{"read": 2, "write": 1}, fp.RiskCounts)
	assert.Equal(t, 3, fp.TotalTools)
	assert.Equal(t, 3000, fp.TotalTokens)
	assert.Equal(t, 1800, fp.PromptTokens)
	assert.Equal(t, 1200, fp.CompletionTokens)
	assert.InDelta(t, 1000.0, fp.AvgTokens, 0.01)
	assert.Equal(t, 1500, fp.MaxTokens)
}

func TestExtract_NoTools(t *testing.T) {
	spans := []*storage.ReplayTrace{
		{Provider: "openai", Model: "gpt-4", TotalTokens: 500},
	}

	fp, err := Extract("trace-2", spans, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, fp.StepCount)
	assert.Equal(t, []string{"openai"}, fp.Providers)
	assert.Empty(t, fp.ToolSequence)
	assert.Empty(t, fp.ToolFrequency)
	assert.Empty(t, fp.RiskCounts)
	assert.Equal(t, 0, fp.TotalTools)
}

func TestExtract_EmptySpans(t *testing.T) {
	_, err := Extract("trace-3", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no spans provided")
}

func TestExtract_SingleStep(t *testing.T) {
	spans := []*storage.ReplayTrace{
		{Provider: "anthropic", Model: "claude-3-opus", PromptTokens: 1200, CompletionTokens: 800, TotalTokens: 2000},
	}
	tools := []*storage.ToolCapture{
		{ToolName: "delete_file", RiskClass: "destructive"},
	}

	fp, err := Extract("trace-4", spans, tools)
	require.NoError(t, err)

	assert.Equal(t, 1, fp.StepCount)
	assert.Equal(t, 2000, fp.TotalTokens)
	assert.Equal(t, 1200, fp.PromptTokens)
	assert.Equal(t, 800, fp.CompletionTokens)
	assert.InDelta(t, 2000.0, fp.AvgTokens, 0.01)
	assert.Equal(t, 2000, fp.MaxTokens)
	assert.Equal(t, map[string]int{"destructive": 1}, fp.RiskCounts)
}

func TestExtract_MixedModels(t *testing.T) {
	spans := []*storage.ReplayTrace{
		{Provider: "anthropic", Model: "claude-3-opus", TotalTokens: 1000},
		{Provider: "anthropic", Model: "claude-3.5-sonnet", TotalTokens: 800},
	}

	fp, err := Extract("trace-5", spans, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"claude-3-opus", "claude-3.5-sonnet"}, fp.Models)
	assert.Equal(t, []string{"anthropic", "anthropic"}, fp.Providers)
}

func TestExtract_MixedProviders(t *testing.T) {
	spans := []*storage.ReplayTrace{
		{Provider: "anthropic", Model: "claude-3-opus", TotalTokens: 1000},
		{Provider: "openai", Model: "gpt-4", TotalTokens: 800},
	}

	fp, err := Extract("trace-6", spans, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"anthropic", "openai"}, fp.Providers)
	assert.Equal(t, []string{"claude-3-opus", "gpt-4"}, fp.Models)
}

func TestExtract_EmptyRiskClass(t *testing.T) {
	spans := []*storage.ReplayTrace{
		{Model: "claude-3-opus", TotalTokens: 1000},
	}
	tools := []*storage.ToolCapture{
		{ToolName: "search", RiskClass: "read"},
		{ToolName: "custom_tool", RiskClass: ""},    // empty — excluded
		{ToolName: "write_file", RiskClass: "write"},
	}

	fp, err := Extract("trace-7", spans, tools)
	require.NoError(t, err)

	// Empty risk class excluded from counts
	assert.Equal(t, map[string]int{"read": 1, "write": 1}, fp.RiskCounts)
	assert.Equal(t, 3, fp.TotalTools) // all three tools counted
}

func TestExtract_UnknownRiskClass(t *testing.T) {
	spans := []*storage.ReplayTrace{
		{Model: "claude-3-opus", TotalTokens: 1000},
	}
	tools := []*storage.ToolCapture{
		{ToolName: "search", RiskClass: "read"},
		{ToolName: "admin_tool", RiskClass: "admin"},        // unknown → "unknown"
		{ToolName: "exec_tool", RiskClass: "DESTRUCTIVE"},   // normalized → "destructive"
		{ToolName: "list_tool", RiskClass: " Read "},         // normalized → "read"
	}

	fp, err := Extract("trace-8", spans, tools)
	require.NoError(t, err)

	assert.Equal(t, 2, fp.RiskCounts["read"])        // "read" + " Read "
	assert.Equal(t, 1, fp.RiskCounts["destructive"])  // "DESTRUCTIVE"
	assert.Equal(t, 1, fp.RiskCounts["unknown"])      // "admin"
}

// --- normalizeRiskClass tests ---

func TestNormalizeRiskClass(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"read", "read"},
		{"write", "write"},
		{"destructive", "destructive"},
		{"READ", "read"},
		{" Write ", "write"},
		{"DESTRUCTIVE", "destructive"},
		{"admin", "unknown"},
		{"", ""},
		{"  ", ""},
		{"Execute", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeRiskClass(tt.input))
		})
	}
}

// --- toolOrderDistance tests ---

func TestToolOrderDistance(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []string
		expected float64
		delta    float64
	}{
		{
			name:     "identical",
			a:        []string{"search", "write", "read"},
			b:        []string{"search", "write", "read"},
			expected: 0.0,
			delta:    0.01,
		},
		{
			name:     "completely different",
			a:        []string{"search", "write"},
			b:        []string{"delete", "create"},
			expected: 1.0,
			delta:    0.01,
		},
		{
			name:     "one insertion",
			a:        []string{"search", "write"},
			b:        []string{"search", "read", "write"},
			expected: 1.0 / 3.0, // edit distance 1, max len 3
			delta:    0.01,
		},
		{
			name:     "both empty",
			a:        []string{},
			b:        []string{},
			expected: 0.0,
			delta:    0.01,
		},
		{
			name:     "one empty",
			a:        []string{"search"},
			b:        []string{},
			expected: 1.0,
			delta:    0.01,
		},
		{
			name:     "reversed",
			a:        []string{"a", "b", "c"},
			b:        []string{"c", "b", "a"},
			expected: 2.0 / 3.0, // edit distance 2 (swap ends)
			delta:    0.01,
		},
		{
			name:     "a longer than b (exercises swap branch)",
			a:        []string{"a", "b", "c", "d"},
			b:        []string{"a", "b"},
			expected: 2.0 / 4.0, // 2 deletions, max len 4
			delta:    0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toolOrderDistance(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, tt.delta)
		})
	}
}

// --- toolFrequencyDistance tests ---

func TestToolFrequencyDistance(t *testing.T) {
	tests := []struct {
		name     string
		a, b     map[string]int
		expected float64
		delta    float64
	}{
		{
			name:     "identical",
			a:        map[string]int{"search": 3, "write": 1},
			b:        map[string]int{"search": 3, "write": 1},
			expected: 0.0,
			delta:    0.01,
		},
		{
			name:     "proportional (cosine ignores scale)",
			a:        map[string]int{"search": 3, "write": 1},
			b:        map[string]int{"search": 6, "write": 2},
			expected: 0.0,
			delta:    0.01,
		},
		{
			name:     "disjoint",
			a:        map[string]int{"search": 5},
			b:        map[string]int{"write": 5},
			expected: 1.0,
			delta:    0.01,
		},
		{
			name:     "both empty",
			a:        map[string]int{},
			b:        map[string]int{},
			expected: 0.0,
			delta:    0.01,
		},
		{
			name:     "one empty",
			a:        map[string]int{"search": 1},
			b:        map[string]int{},
			expected: 1.0,
			delta:    0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toolFrequencyDistance(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, tt.delta)
		})
	}
}

// --- riskShiftScore tests ---

func TestRiskShiftScore(t *testing.T) {
	tests := []struct {
		name            string
		base, cand      map[string]int
		expectHigh      bool // expect score > 0.5
		expectEsc       bool // expect escalation=true in details
		checkScore      *float64
		checkScoreDelta float64
	}{
		{
			name:            "no change",
			base:            map[string]int{"read": 5, "write": 2},
			cand:            map[string]int{"read": 5, "write": 2},
			checkScore:      ptrFloat(0.0),
			checkScoreDelta: 0.01,
		},
		{
			name:       "read to destructive",
			base:       map[string]int{"read": 10},
			cand:       map[string]int{"destructive": 10},
			expectHigh: true,
			expectEsc:  true,
		},
		{
			name: "write to read is less than read to write",
			base: map[string]int{"write": 10},
			cand: map[string]int{"read": 10},
		},
		{
			name:            "both empty",
			base:            map[string]int{},
			cand:            map[string]int{},
			checkScore:      ptrFloat(0.0),
			checkScoreDelta: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, details := riskShiftScore(tt.base, tt.cand)
			assert.GreaterOrEqual(t, score, 0.0)
			assert.LessOrEqual(t, score, 1.0)

			if tt.expectHigh {
				assert.Greater(t, score, 0.5, "expected high risk score")
			}
			if tt.expectEsc {
				assert.Equal(t, true, details["escalation"])
			}
			if tt.checkScore != nil {
				assert.InDelta(t, *tt.checkScore, score, tt.checkScoreDelta)
			}
		})
	}

	// Asymmetry check: read→write should penalize more than write→read
	t.Run("asymmetric penalty", func(t *testing.T) {
		scoreUp, _ := riskShiftScore(
			map[string]int{"read": 10},
			map[string]int{"write": 10},
		)
		scoreDown, _ := riskShiftScore(
			map[string]int{"write": 10},
			map[string]int{"read": 10},
		)
		assert.Greater(t, scoreUp, scoreDown, "escalation should be penalized more than de-escalation")
	})
}

func TestRiskShiftScore_UnknownClassIgnored(t *testing.T) {
	// Unknown classes should not affect the risk fractions or scoring
	base := map[string]int{"read": 5, "unknown": 3}
	cand := map[string]int{"read": 5, "unknown": 3}

	score, details := riskShiftScore(base, cand)
	assert.InDelta(t, 0.0, score, 0.01)

	// Fractions should only reflect known classes
	baseFrac := details["baseline_fractions"].(map[string]float64)
	assert.InDelta(t, 1.0, baseFrac["read"], 0.01)
	assert.InDelta(t, 0.0, baseFrac["write"], 0.01)
	assert.InDelta(t, 0.0, baseFrac["destructive"], 0.01)
}

func TestRiskShiftScore_UnknownToDestructive(t *testing.T) {
	// Baseline: only unknown tools. Candidate: destructive tools.
	// Since unknown is excluded from fractions, baseline fractions are all zero.
	// Candidate fractions: destructive=1.0
	// This should score as high escalation.
	base := map[string]int{"unknown": 10}
	cand := map[string]int{"destructive": 10}

	score, details := riskShiftScore(base, cand)
	assert.Greater(t, score, 0.5, "transition to destructive should score high")
	assert.Equal(t, true, details["escalation"])
}

// --- tokenDeltaScore tests ---

func TestTokenDeltaScore(t *testing.T) {
	tests := []struct {
		name     string
		base     int
		cand     int
		expected float64
		delta    float64
	}{
		{
			name:     "identical",
			base:     1000,
			cand:     1000,
			expected: 0.0,
			delta:    0.01,
		},
		{
			name:  "small delta (~10%)",
			base:  1000,
			cand:  1100,
			delta: 0.1, // just check it's low
		},
		{
			name:  "double",
			base:  1000,
			cand:  2000,
			delta: 0.1, // check it's high
		},
		{
			name:     "both zero",
			base:     0,
			cand:     0,
			expected: 0.0,
			delta:    0.01,
		},
		{
			name:     "one zero one nonzero",
			base:     0,
			cand:     1000,
			expected: 1.0, // 4 * 1000/1000 = 4, log2(5) > 1, clamped to 1.0
			delta:    0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenDeltaScore(tt.base, tt.cand)
			assert.GreaterOrEqual(t, result, 0.0)
			assert.LessOrEqual(t, result, 1.0)

			if tt.expected > 0 || tt.name == "identical" || tt.name == "both zero" || tt.name == "one zero one nonzero" {
				assert.InDelta(t, tt.expected, result, tt.delta)
			}
		})
	}

	// Small delta should be lower than large delta
	t.Run("monotonic", func(t *testing.T) {
		small := tokenDeltaScore(1000, 1100)
		large := tokenDeltaScore(1000, 2000)
		assert.Less(t, small, large)
	})
}

// --- End-to-end Compare tests ---

func TestCompare_IdenticalTraces(t *testing.T) {
	fp := &Fingerprint{
		TraceID:          "trace-a",
		StepCount:        3,
		Models:           []string{"claude-3-opus", "claude-3-opus", "claude-3-opus"},
		Providers:        []string{"anthropic", "anthropic", "anthropic"},
		ToolSequence:     []string{"search", "read", "write"},
		ToolFrequency:    map[string]int{"search": 1, "read": 1, "write": 1},
		RiskCounts:       map[string]int{"read": 2, "write": 1},
		TotalTools:       3,
		TotalTokens:      3000,
		PromptTokens:     1800,
		CompletionTokens: 1200,
	}

	report := Compare(fp, fp, DefaultConfig())
	assert.InDelta(t, 0.0, report.Score, 0.01)
	assert.Equal(t, storage.DriftVerdictPass, report.Verdict)
	assert.Equal(t, false, report.Details["model_changed"])
	assert.Equal(t, false, report.Details["provider_changed"])
}

func TestCompare_CompletelyDifferent(t *testing.T) {
	baseline := &Fingerprint{
		TraceID:       "base",
		StepCount:     2,
		Models:        []string{"claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic"},
		ToolSequence:  []string{"search", "read"},
		ToolFrequency: map[string]int{"search": 1, "read": 1},
		RiskCounts:    map[string]int{"read": 2},
		TotalTools:    2,
		TotalTokens:   2000,
	}
	candidate := &Fingerprint{
		TraceID:       "cand",
		StepCount:     3,
		Models:        []string{"gpt-4", "gpt-4", "gpt-4"},
		Providers:     []string{"openai", "openai", "openai"},
		ToolSequence:  []string{"delete", "create", "deploy"},
		ToolFrequency: map[string]int{"delete": 1, "create": 1, "deploy": 1},
		RiskCounts:    map[string]int{"destructive": 3},
		TotalTools:    3,
		TotalTokens:   8000,
	}

	report := Compare(baseline, candidate, DefaultConfig())
	assert.Greater(t, report.Score, 0.5, "completely different traces should score high")
	assert.Equal(t, storage.DriftVerdictFail, report.Verdict)
	assert.Equal(t, true, report.Details["model_changed"])
	assert.Equal(t, true, report.Details["provider_changed"])
}

func TestCompare_WarnVerdict(t *testing.T) {
	// Construct traces that produce a score between 0.2 and 0.5
	baseline := &Fingerprint{
		TraceID:       "base",
		StepCount:     3,
		Models:        []string{"claude-3-opus", "claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic", "anthropic"},
		ToolSequence:  []string{"search", "read", "write"},
		ToolFrequency: map[string]int{"search": 1, "read": 1, "write": 1},
		RiskCounts:    map[string]int{"read": 2, "write": 1},
		TotalTools:    3,
		TotalTokens:   3000,
	}
	candidate := &Fingerprint{
		TraceID:       "cand",
		StepCount:     3,
		Models:        []string{"claude-3-opus", "claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic", "anthropic"},
		ToolSequence:  []string{"read", "search", "write"}, // reorder → ~0.67 tool order
		ToolFrequency: map[string]int{"search": 1, "read": 1, "write": 1},
		RiskCounts:    map[string]int{"read": 2, "write": 1},
		TotalTools:    3,
		TotalTokens:   3000,
	}

	report := Compare(baseline, candidate, DefaultConfig())
	// Tool order: 2/3 * 0.35 = 0.233, everything else 0 → score ~0.233
	assert.Greater(t, report.Score, 0.2)
	assert.LessOrEqual(t, report.Score, 0.5)
	assert.Equal(t, storage.DriftVerdictWarn, report.Verdict)
}

func TestCompare_MinorReorder(t *testing.T) {
	baseline := &Fingerprint{
		TraceID:       "base",
		StepCount:     3,
		Models:        []string{"claude-3-opus", "claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic", "anthropic"},
		ToolSequence:  []string{"search", "read", "write"},
		ToolFrequency: map[string]int{"search": 1, "read": 1, "write": 1},
		RiskCounts:    map[string]int{"read": 2, "write": 1},
		TotalTools:    3,
		TotalTokens:   3000,
	}
	candidate := &Fingerprint{
		TraceID:       "cand",
		StepCount:     3,
		Models:        []string{"claude-3-opus", "claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic", "anthropic"},
		ToolSequence:  []string{"read", "search", "write"},
		ToolFrequency: map[string]int{"search": 1, "read": 1, "write": 1},
		RiskCounts:    map[string]int{"read": 2, "write": 1},
		TotalTools:    3,
		TotalTokens:   3100,
	}

	report := Compare(baseline, candidate, DefaultConfig())
	assert.LessOrEqual(t, report.Score, 0.5, "minor reorder should not fail")
	assert.NotEqual(t, storage.DriftVerdictFail, report.Verdict)
}

func TestCompare_RiskEscalation(t *testing.T) {
	baseline := &Fingerprint{
		TraceID:       "base",
		StepCount:     2,
		Models:        []string{"claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic"},
		ToolSequence:  []string{"search", "read"},
		ToolFrequency: map[string]int{"search": 1, "read": 1},
		RiskCounts:    map[string]int{"read": 2},
		TotalTools:    2,
		TotalTokens:   2000,
	}
	candidate := &Fingerprint{
		TraceID:       "cand",
		StepCount:     2,
		Models:        []string{"claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic"},
		ToolSequence:  []string{"search", "delete"},
		ToolFrequency: map[string]int{"search": 1, "delete": 1},
		RiskCounts:    map[string]int{"read": 1, "destructive": 1},
		TotalTools:    2,
		TotalTokens:   2000,
	}

	report := Compare(baseline, candidate, DefaultConfig())
	assert.Greater(t, report.Score, 0.2, "risk escalation should increase score")

	riskDetails, ok := report.Details["risk_details"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, riskDetails["escalation"])
}

func TestCompare_TokenJitterOnly(t *testing.T) {
	baseline := &Fingerprint{
		TraceID:       "base",
		StepCount:     2,
		Models:        []string{"claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic"},
		ToolSequence:  []string{"search", "write"},
		ToolFrequency: map[string]int{"search": 1, "write": 1},
		RiskCounts:    map[string]int{"read": 1, "write": 1},
		TotalTools:    2,
		TotalTokens:   2000,
	}
	candidate := &Fingerprint{
		TraceID:       "cand",
		StepCount:     2,
		Models:        []string{"claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic"},
		ToolSequence:  []string{"search", "write"},
		ToolFrequency: map[string]int{"search": 1, "write": 1},
		RiskCounts:    map[string]int{"read": 1, "write": 1},
		TotalTools:    2,
		TotalTokens:   2200, // 10% increase
	}

	report := Compare(baseline, candidate, DefaultConfig())
	assert.Equal(t, storage.DriftVerdictPass, report.Verdict, "token jitter alone should pass")
}

func TestCompare_ModelChanged(t *testing.T) {
	baseline := &Fingerprint{
		TraceID:       "base",
		StepCount:     1,
		Models:        []string{"claude-3-opus"},
		Providers:     []string{"anthropic"},
		ToolSequence:  []string{"search"},
		ToolFrequency: map[string]int{"search": 1},
		RiskCounts:    map[string]int{"read": 1},
		TotalTools:    1,
		TotalTokens:   1000,
	}
	candidate := &Fingerprint{
		TraceID:       "cand",
		StepCount:     1,
		Models:        []string{"claude-3.5-sonnet"},
		Providers:     []string{"anthropic"},
		ToolSequence:  []string{"search"},
		ToolFrequency: map[string]int{"search": 1},
		RiskCounts:    map[string]int{"read": 1},
		TotalTools:    1,
		TotalTokens:   1000,
	}

	report := Compare(baseline, candidate, DefaultConfig())
	assert.Equal(t, true, report.Details["model_changed"])
	assert.Equal(t, false, report.Details["provider_changed"])
	// Score should still be low — model change is informational
	assert.Equal(t, storage.DriftVerdictPass, report.Verdict)
}

func TestCompare_ProviderChanged(t *testing.T) {
	baseline := &Fingerprint{
		TraceID:       "base",
		StepCount:     1,
		Models:        []string{"claude-3-opus"},
		Providers:     []string{"anthropic"},
		ToolSequence:  []string{"search"},
		ToolFrequency: map[string]int{"search": 1},
		RiskCounts:    map[string]int{"read": 1},
		TotalTools:    1,
		TotalTokens:   1000,
	}
	candidate := &Fingerprint{
		TraceID:       "cand",
		StepCount:     1,
		Models:        []string{"gpt-4"},
		Providers:     []string{"openai"},
		ToolSequence:  []string{"search"},
		ToolFrequency: map[string]int{"search": 1},
		RiskCounts:    map[string]int{"read": 1},
		TotalTools:    1,
		TotalTokens:   1000,
	}

	report := Compare(baseline, candidate, DefaultConfig())
	assert.Equal(t, true, report.Details["model_changed"])
	assert.Equal(t, true, report.Details["provider_changed"])
	// Both model and provider change are informational
	assert.Equal(t, storage.DriftVerdictPass, report.Verdict)
}

func TestCompare_CustomWeights(t *testing.T) {
	baseline := &Fingerprint{
		TraceID:       "base",
		StepCount:     2,
		Models:        []string{"claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic"},
		ToolSequence:  []string{"search", "write"},
		ToolFrequency: map[string]int{"search": 1, "write": 1},
		RiskCounts:    map[string]int{"read": 1, "write": 1},
		TotalTools:    2,
		TotalTokens:   2000,
	}
	candidate := &Fingerprint{
		TraceID:       "cand",
		StepCount:     2,
		Models:        []string{"claude-3-opus", "claude-3-opus"},
		Providers:     []string{"anthropic", "anthropic"},
		ToolSequence:  []string{"write", "search"}, // reordered
		ToolFrequency: map[string]int{"search": 1, "write": 1},
		RiskCounts:    map[string]int{"read": 1, "write": 1},
		TotalTools:    2,
		TotalTokens:   2000,
	}

	// With default config (tool_order weight 0.35)
	defaultReport := Compare(baseline, candidate, DefaultConfig())

	// With zero tool_order weight
	noOrderCfg := DefaultConfig()
	noOrderCfg.ToolOrderWeight = 0.0
	noOrderReport := Compare(baseline, candidate, noOrderCfg)

	assert.Greater(t, defaultReport.Score, noOrderReport.Score,
		"zeroing tool_order weight should reduce score when only order differs")
	assert.InDelta(t, 0.0, noOrderReport.Score, 0.01,
		"with zero tool_order weight and no other diffs, score should be ~0")
}

func TestCompare_NoToolsEitherSide(t *testing.T) {
	baseline := &Fingerprint{
		TraceID:       "base",
		StepCount:     1,
		Models:        []string{"claude-3-opus"},
		Providers:     []string{"anthropic"},
		ToolSequence:  []string{},
		ToolFrequency: map[string]int{},
		RiskCounts:    map[string]int{},
		TotalTools:    0,
		TotalTokens:   1000,
	}
	candidate := &Fingerprint{
		TraceID:       "cand",
		StepCount:     1,
		Models:        []string{"claude-3-opus"},
		Providers:     []string{"anthropic"},
		ToolSequence:  []string{},
		ToolFrequency: map[string]int{},
		RiskCounts:    map[string]int{},
		TotalTools:    0,
		TotalTokens:   1000,
	}

	report := Compare(baseline, candidate, DefaultConfig())
	assert.InDelta(t, 0.0, report.Score, 0.01)
	assert.Equal(t, storage.DriftVerdictPass, report.Verdict)
}

func TestCompare_DetailsContainsTokenBreakdown(t *testing.T) {
	baseline := &Fingerprint{
		TraceID:          "base",
		StepCount:        1,
		Models:           []string{"claude-3-opus"},
		Providers:        []string{"anthropic"},
		ToolSequence:     []string{},
		ToolFrequency:    map[string]int{},
		RiskCounts:       map[string]int{},
		TotalTokens:      3000,
		PromptTokens:     2000,
		CompletionTokens: 1000,
	}
	candidate := &Fingerprint{
		TraceID:          "cand",
		StepCount:        1,
		Models:           []string{"claude-3-opus"},
		Providers:        []string{"anthropic"},
		ToolSequence:     []string{},
		ToolFrequency:    map[string]int{},
		RiskCounts:       map[string]int{},
		TotalTokens:      3000,
		PromptTokens:     1800,
		CompletionTokens: 1200,
	}

	report := Compare(baseline, candidate, DefaultConfig())
	assert.Equal(t, 2000, report.Details["baseline_prompt_tokens"])
	assert.Equal(t, 1000, report.Details["baseline_completion_tokens"])
	assert.Equal(t, 1800, report.Details["candidate_prompt_tokens"])
	assert.Equal(t, 1200, report.Details["candidate_completion_tokens"])
}

// --- DefaultConfig tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.InDelta(t, 0.35, cfg.ToolOrderWeight, 0.001)
	assert.InDelta(t, 0.25, cfg.ToolFrequencyWeight, 0.001)
	assert.InDelta(t, 0.25, cfg.RiskShiftWeight, 0.001)
	assert.InDelta(t, 0.15, cfg.TokenDeltaWeight, 0.001)
	assert.InDelta(t, 0.20, cfg.PassThreshold, 0.001)
	assert.InDelta(t, 0.50, cfg.WarnThreshold, 0.001)

	// Weights must sum to 1.0
	sum := cfg.ToolOrderWeight + cfg.ToolFrequencyWeight + cfg.RiskShiftWeight + cfg.TokenDeltaWeight
	assert.InDelta(t, 1.0, sum, 0.001)
}

// --- Helpers ---

func ptrFloat(f float64) *float64 {
	return &f
}

// Verify log-dampening curve properties
func TestTokenDeltaScore_CurveProperties(t *testing.T) {
	// 10% delta → ~0.45
	// base=1000, cand=1100: delta=100, max=1100, ratio≈0.091
	// log2(1 + 4*0.091) = log2(1.364) ≈ 0.45
	score10 := tokenDeltaScore(1000, 1100)
	assert.InDelta(t, 0.45, score10, 0.1)

	// The formula saturates to 1.0 quickly: when 4*ratio >= 1 → ratio >= 0.25
	// 50% delta (ratio=0.333) already saturates to 1.0
	score50 := tokenDeltaScore(1000, 1500)
	assert.InDelta(t, 1.0, score50, 0.01)

	// 100% delta → 1.0 (also saturated)
	score100 := tokenDeltaScore(1000, 2000)
	assert.InDelta(t, 1.0, score100, 0.01)

	// Monotonicity holds up to saturation: small < large
	assert.Less(t, score10, score50)
	// At saturation both equal 1.0
	assert.InDelta(t, score50, score100, 0.01)

	// Symmetric
	assert.InDelta(t, tokenDeltaScore(1000, 1100), tokenDeltaScore(1100, 1000), 0.01)
}

func TestRiskShiftScore_MaxTheoretical(t *testing.T) {
	// Maximum escalation: all read → all destructive
	score, details := riskShiftScore(
		map[string]int{"read": 100},
		map[string]int{"destructive": 100},
	)
	assert.Equal(t, true, details["escalation"])
	// Score should be very high (close to 1.0)
	assert.Greater(t, score, 0.9, "max escalation should produce score close to 1.0")
	assert.LessOrEqual(t, score, 1.0)
}

func TestToolOrderDistance_LongSequences(t *testing.T) {
	a := []string{"a", "b", "c", "d", "e"}
	b := []string{"a", "b", "c", "d", "e"}
	assert.InDelta(t, 0.0, toolOrderDistance(a, b), 0.01)

	// Single change in long sequence should be small
	b2 := []string{"a", "b", "x", "d", "e"}
	dist := toolOrderDistance(a, b2)
	assert.InDelta(t, 1.0/5.0, dist, 0.01)

	// Completely different
	c := []string{"v", "w", "x", "y", "z"}
	assert.InDelta(t, 1.0, toolOrderDistance(a, c), 0.01)
}

func TestToolFrequencyDistance_PartialOverlap(t *testing.T) {
	a := map[string]int{"search": 5, "write": 3}
	b := map[string]int{"search": 5, "delete": 3}

	dist := toolFrequencyDistance(a, b)
	// Not 0 (different tools), not 1 (some overlap)
	assert.Greater(t, dist, 0.0)
	assert.Less(t, dist, 1.0)

	// Verify specific value: dot=25, magA=sqrt(34), magB=sqrt(34)
	// cosine = 25/34 ≈ 0.735, distance ≈ 0.265
	expected := 1.0 - 25.0/math.Sqrt(34.0*34.0)
	assert.InDelta(t, expected, dist, 0.01)
}

// --- Sequence truncation tests ---

func TestTruncateSequence(t *testing.T) {
	short := []string{"a", "b", "c"}
	assert.Equal(t, short, truncateSequence(short, 1000))

	long := make([]string, 10)
	for i := range long {
		long[i] = "tool"
	}
	result := truncateSequence(long, 5)
	assert.Len(t, result, 5)
}
