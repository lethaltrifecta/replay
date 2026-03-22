package diff

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// Config controls the similarity comparison.
type Config struct {
	SimilarityThreshold float64 // >= this -> pass (default 0.8)
}

// DefaultConfig returns a Config with default thresholds.
func DefaultConfig() Config {
	return Config{
		SimilarityThreshold: 0.8,
	}
}

// StepDiff captures the token delta for a single step.
type StepDiff struct {
	StepIndex  int `json:"step_index"`
	TokenDelta int `json:"token_delta"` // variant - baseline
}

// Report is the result of comparing baseline and variant replay traces.
type Report struct {
	SimilarityScore float64    `json:"similarity_score"` // [0, 1]
	Verdict         string     `json:"verdict"`          // "pass" or "fail"
	StepCount       int        `json:"step_count"`       // variant step count
	StepDiffs       []StepDiff `json:"step_diffs"`
	TokenDelta      int        `json:"token_delta"`   // total (variant - baseline)
	LatencyDelta    int        `json:"latency_delta"` // total ms (variant - baseline)

	// Semantic dimensions (populated by CompareAll, nil when using Compare)
	ToolCallScore   *ToolCallScore `json:"tool_call_score,omitempty"`
	RiskScore       *RiskScore     `json:"risk_score,omitempty"`
	ResponseScore   *ResponseScore `json:"response_score,omitempty"`
	FirstDivergence storage.JSONB  `json:"first_divergence,omitempty"`
}

// CompareInput holds all data needed for a full 6-dimension comparison.
type CompareInput struct {
	Baseline      []*storage.ReplayTrace
	Variant       []*storage.ReplayTrace
	BaselineTools []*storage.ToolCapture // from DB, may be nil
	VariantTools  []ToolCall             // extracted from variant metadata, may be nil
}

// Dimension weights for similarity scoring
const (
	weightStepCount = 0.4
	weightTokens    = 0.3
	weightLatency   = 0.3
)

// Compare computes a structural similarity report between baseline and variant traces.
// Both slices are expected to be ordered by step_index.
func Compare(baseline, variant []*storage.ReplayTrace, cfg Config) *Report {
	report := &Report{
		StepCount: len(variant),
	}

	// Step count similarity: 1.0 - |delta| / max(steps)
	stepSim := stepCountSimilarity(len(baseline), len(variant))

	// Per-step diffs (only for overlapping steps)
	minSteps := len(baseline)
	if len(variant) < minSteps {
		minSteps = len(variant)
	}

	var totalBaseTokens, totalVarTokens int
	var totalBaseLatency, totalVarLatency int

	for i := 0; i < minSteps; i++ {
		tokenDelta := variant[i].TotalTokens - baseline[i].TotalTokens
		report.StepDiffs = append(report.StepDiffs, StepDiff{
			StepIndex:  i,
			TokenDelta: tokenDelta,
		})
		totalBaseTokens += baseline[i].TotalTokens
		totalVarTokens += variant[i].TotalTokens
		totalBaseLatency += baseline[i].LatencyMS
		totalVarLatency += variant[i].LatencyMS
	}

	// Add remaining steps from the longer trace
	for i := minSteps; i < len(baseline); i++ {
		totalBaseTokens += baseline[i].TotalTokens
		totalBaseLatency += baseline[i].LatencyMS
	}
	for i := minSteps; i < len(variant); i++ {
		totalVarTokens += variant[i].TotalTokens
		totalVarLatency += variant[i].LatencyMS
		report.StepDiffs = append(report.StepDiffs, StepDiff{
			StepIndex:  i,
			TokenDelta: variant[i].TotalTokens,
		})
	}

	report.TokenDelta = totalVarTokens - totalBaseTokens
	report.LatencyDelta = totalVarLatency - totalBaseLatency

	// Token similarity: reuse log-dampened formula from drift/compare.go
	tokenSim := 1.0 - tokenDeltaScore(totalBaseTokens, totalVarTokens)

	// Latency similarity: 1.0 - |delta| / max(latency)
	latencySim := latencySimilarity(totalBaseLatency, totalVarLatency)

	report.SimilarityScore = weightStepCount*stepSim + weightTokens*tokenSim + weightLatency*latencySim

	// Clamp to [0, 1]
	report.SimilarityScore = math.Max(0.0, math.Min(1.0, report.SimilarityScore))

	if report.SimilarityScore >= cfg.SimilarityThreshold {
		report.Verdict = "pass"
	} else {
		report.Verdict = "fail"
	}

	return report
}

// stepCountSimilarity returns 1.0 - |delta| / max(a, b).
// Returns 1.0 if both are zero.
func stepCountSimilarity(a, b int) float64 {
	if a == 0 && b == 0 {
		return 1.0
	}
	maxSteps := max(a, b)
	delta := math.Abs(float64(a - b))
	return 1.0 - delta/float64(maxSteps)
}

// tokenDeltaScore computes a log-dampened score for token count divergence.
// Replicates the formula from pkg/drift/compare.go:263.
// Formula: min(1, log2(1 + 4 * |delta| / max(base, cand)))
func tokenDeltaScore(base, cand int) float64 {
	if base == 0 && cand == 0 {
		return 0.0
	}
	maxTokens := max(base, cand)
	delta := math.Abs(float64(base - cand))
	ratio := delta / float64(maxTokens)
	return math.Min(1.0, math.Log2(1.0+4.0*ratio))
}

// latencySimilarity returns 1.0 - |delta| / max(a, b).
// Returns 1.0 if both are zero.
func latencySimilarity(a, b int) float64 {
	if a == 0 && b == 0 {
		return 1.0
	}
	maxLatency := max(a, b)
	delta := math.Abs(float64(a - b))
	return 1.0 - delta/float64(maxLatency)
}

// CompareAll computes a 6-dimension similarity report when tool data is available,
// falling back to 4 dimensions (structural + response) when no tools are present.
// The existing Compare() function is unchanged for backward compatibility.
func CompareAll(input CompareInput, cfg Config) *Report {
	baseline := input.Baseline
	variant := input.Variant

	// Structural metrics (same as Compare)
	report := &Report{
		StepCount: len(variant),
	}

	stepSim := stepCountSimilarity(len(baseline), len(variant))

	minSteps := len(baseline)
	if len(variant) < minSteps {
		minSteps = len(variant)
	}

	var totalBaseTokens, totalVarTokens int
	var totalBaseLatency, totalVarLatency int

	for i := 0; i < minSteps; i++ {
		tokenDelta := variant[i].TotalTokens - baseline[i].TotalTokens
		report.StepDiffs = append(report.StepDiffs, StepDiff{
			StepIndex:  i,
			TokenDelta: tokenDelta,
		})
		totalBaseTokens += baseline[i].TotalTokens
		totalVarTokens += variant[i].TotalTokens
		totalBaseLatency += baseline[i].LatencyMS
		totalVarLatency += variant[i].LatencyMS
	}

	for i := minSteps; i < len(baseline); i++ {
		totalBaseTokens += baseline[i].TotalTokens
		totalBaseLatency += baseline[i].LatencyMS
	}
	for i := minSteps; i < len(variant); i++ {
		totalVarTokens += variant[i].TotalTokens
		totalVarLatency += variant[i].LatencyMS
		report.StepDiffs = append(report.StepDiffs, StepDiff{
			StepIndex:  i,
			TokenDelta: variant[i].TotalTokens,
		})
	}

	report.TokenDelta = totalVarTokens - totalBaseTokens
	report.LatencyDelta = totalVarLatency - totalBaseLatency

	tokenSim := 1.0 - tokenDeltaScore(totalBaseTokens, totalVarTokens)
	latencySim := latencySimilarity(totalBaseLatency, totalVarLatency)

	// Semantic dimensions
	baseTools := BaselineToolCalls(input.BaselineTools)
	report.ToolCallScore = CompareToolCalls(baseTools, input.VariantTools)
	report.RiskScore = CompareRiskProfiles(baseTools, input.VariantTools)
	report.ResponseScore = CompareResponses(baseline, variant)

	report.FirstDivergence = findFirstDivergence(baseline, variant, baseTools, input.VariantTools)

	hasToolData := report.ToolCallScore != nil

	if hasToolData {
		// 6-dimension weights
		report.SimilarityScore = 0.15*stepSim +
			0.10*tokenSim +
			0.10*latencySim +
			0.25*report.ToolCallScore.Score +
			0.20*report.RiskScore.Score +
			0.20*safeResponseScore(report.ResponseScore)
	} else {
		// 4-dimension fallback (no tool data)
		report.SimilarityScore = 0.25*stepSim +
			0.15*tokenSim +
			0.15*latencySim +
			0.45*safeResponseScore(report.ResponseScore)
	}

	report.SimilarityScore = math.Max(0.0, math.Min(1.0, report.SimilarityScore))

	if report.SimilarityScore >= cfg.SimilarityThreshold {
		report.Verdict = "pass"
	} else {
		report.Verdict = "fail"
	}

	return report
}

// safeResponseScore returns the response score value, or 1.0 if nil.
func safeResponseScore(rs *ResponseScore) float64 {
	if rs == nil {
		return 1.0
	}
	return rs.Score
}

// findFirstDivergence returns the first step index where tool sequences diverge
// or where Jaccard content overlap < 0.5.
func findFirstDivergence(baseline, variant []*storage.ReplayTrace, baseTools, varTools []ToolCall) storage.JSONB {
	// Check tool sequence divergence.
	// Note: tool_index is the position in the flat tool call list, not the replay step index.
	baseNames := toolNames(baseTools)
	varNames := toolNames(varTools)

	if len(baseNames) > 0 || len(varNames) > 0 {
		minLen := len(baseNames)
		if len(varNames) < minLen {
			minLen = len(varNames)
		}
		for i := 0; i < minLen; i++ {
			if baseNames[i] != varNames[i] {
				return storage.JSONB{
					"tool_index": i,
					"type":       "tool_sequence",
					"baseline":   baseNames[i],
					"variant":    varNames[i],
				}
			}
		}
		if len(baseNames) != len(varNames) {
			return storage.JSONB{
				"tool_index":     minLen,
				"type":           "tool_count",
				"baseline_count": len(baseNames),
				"variant_count":  len(varNames),
			}
		}
	}

	// Check response content divergence
	minSteps := len(baseline)
	if len(variant) < minSteps {
		minSteps = len(variant)
	}
	for i := 0; i < minSteps; i++ {
		j := jaccard(baseline[i].Completion, variant[i].Completion)
		if j < 0.5 {
			return storage.JSONB{
				"step_index":       i,
				"type":             "response_content",
				"jaccard":          j,
				"baseline_excerpt": excerpt(baseline[i].Completion, 120),
				"variant_excerpt":  excerpt(variant[i].Completion, 120),
			}
		}
	}

	// Report step count mismatch if overlapping completions were similar
	if len(baseline) != len(variant) {
		return storage.JSONB{
			"step_index":     minSteps,
			"type":           "step_count",
			"baseline_steps": len(baseline),
			"variant_steps":  len(variant),
		}
	}

	return storage.JSONB{}
}

func excerpt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// StructuredFirstDivergence normalizes the dynamic first-divergence payload from
// CompareAll into the typed storage shape used by persistence, APIs, and CLI output.
func StructuredFirstDivergence(raw storage.JSONB) storage.FirstDivergence {
	if len(raw) == 0 {
		return storage.FirstDivergence{}
	}

	var divergence storage.FirstDivergence
	if idx, ok := jsonInt(raw["step_index"]); ok {
		divergence.StepIndex = &idx
	}
	if idx, ok := jsonInt(raw["tool_index"]); ok {
		divergence.ToolIndex = &idx
	}
	if v, ok := raw["type"].(string); ok {
		divergence.Type = v
	}
	if v, ok := raw["baseline"].(string); ok {
		divergence.Baseline = v
	}
	if v, ok := raw["variant"].(string); ok {
		divergence.Variant = v
	}
	if v, ok := raw["baseline_excerpt"].(string); ok {
		divergence.BaselineExcerpt = v
	}
	if v, ok := raw["variant_excerpt"].(string); ok {
		divergence.VariantExcerpt = v
	}
	if v, ok := jsonInt(raw["baseline_count"]); ok {
		divergence.BaselineCount = &v
	}
	if v, ok := jsonInt(raw["variant_count"]); ok {
		divergence.VariantCount = &v
	}
	if v, ok := jsonInt(raw["baseline_steps"]); ok {
		divergence.BaselineSteps = &v
	}
	if v, ok := jsonInt(raw["variant_steps"]); ok {
		divergence.VariantSteps = &v
	}

	return divergence
}

func jsonInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(math.Round(float64(n))), true
	case float64:
		return int(math.Round(n)), true
	default:
		return 0, false
	}
}

func dominantRiskClass(v any) string {
	var fractions map[string]float64
	switch typed := v.(type) {
	case map[string]float64:
		fractions = typed
	case storage.JSONB:
		fractions = make(map[string]float64, len(typed))
		for key, value := range typed {
			if score, ok := value.(float64); ok {
				fractions[key] = score
			}
		}
	case map[string]interface{}:
		fractions = make(map[string]float64, len(typed))
		for key, value := range typed {
			if score, ok := value.(float64); ok {
				fractions[key] = score
			}
		}
	default:
		return ""
	}

	best := ""
	bestScore := 0.0
	riskOrder := []string{
		storage.RiskClassDestructive,
		storage.RiskClassWrite,
		storage.RiskClassRead,
	}
	for _, risk := range riskOrder {
		score, ok := fractions[risk]
		if !ok || score <= 0 {
			continue
		}
		if score > bestScore {
			best = risk
			bestScore = score
		}
	}
	return best
}

// ToAnalysisResult maps a Report into a storage.AnalysisResult for persistence.
func ToAnalysisResult(report *Report, experimentID, baselineRunID, candidateRunID uuid.UUID) *storage.AnalysisResult {
	behaviorDiff := storage.BehaviorDiff{
		Verdict: report.Verdict,
		Reason:  fmt.Sprintf("Score: %.2f", report.SimilarityScore),
	}

	var safetyDiff storage.SafetyDiff
	if report.RiskScore != nil {
		safetyDiff.RiskEscalation = report.RiskScore.Escalation
		if report.RiskScore.Details != nil {
			safetyDiff.BaselineRisk = dominantRiskClass(report.RiskScore.Details["baseline_fractions"])
			safetyDiff.VariantRisk = dominantRiskClass(report.RiskScore.Details["candidate_fractions"])
		}
	}

	qualityMetrics := storage.JSONB{}
	if report.ResponseScore != nil {
		qualityMetrics["length_similarity"] = report.ResponseScore.LengthSimilarity
		qualityMetrics["content_overlap"] = report.ResponseScore.ContentOverlap
		qualityMetrics["response_score"] = report.ResponseScore.Score
	}
	if report.ToolCallScore != nil {
		qualityMetrics["tool_sequence_similarity"] = report.ToolCallScore.SequenceSimilarity
		qualityMetrics["tool_frequency_similarity"] = report.ToolCallScore.FrequencySimilarity
		qualityMetrics["tool_call_score"] = report.ToolCallScore.Score
	}

	firstDivergence := StructuredFirstDivergence(report.FirstDivergence)

	return &storage.AnalysisResult{
		ExperimentID:    experimentID,
		BaselineRunID:   baselineRunID,
		CandidateRunID:  candidateRunID,
		BehaviorDiff:    behaviorDiff,
		FirstDivergence: firstDivergence,
		SafetyDiff:      safetyDiff,
		SimilarityScore: report.SimilarityScore,
		QualityMetrics:  qualityMetrics,
		TokenDelta:      report.TokenDelta,
		CostDelta:       0,
		LatencyDelta:    report.LatencyDelta,
		CreatedAt:       time.Now(),
	}
}
