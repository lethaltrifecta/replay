package diff

import (
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

// ToAnalysisResult maps a Report into a storage.AnalysisResult for persistence.
func ToAnalysisResult(report *Report, experimentID, baselineRunID, candidateRunID uuid.UUID) *storage.AnalysisResult {
	behaviorDiff := storage.JSONB{
		"step_count":       report.StepCount,
		"step_diffs":       report.StepDiffs,
		"verdict":          report.Verdict,
		"similarity_score": report.SimilarityScore,
	}

	return &storage.AnalysisResult{
		ExperimentID:    experimentID,
		BaselineRunID:   baselineRunID,
		CandidateRunID:  candidateRunID,
		BehaviorDiff:    behaviorDiff,
		FirstDivergence: storage.JSONB{},
		SafetyDiff:      storage.JSONB{},
		SimilarityScore: report.SimilarityScore,
		QualityMetrics:  storage.JSONB{},
		TokenDelta:      report.TokenDelta,
		CostDelta:       0,
		LatencyDelta:    report.LatencyDelta,
		CreatedAt:       time.Now(),
	}
}
