package drift

import (
	"math"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

// maxToolSequenceLen caps the tool sequence length for Levenshtein distance.
// For traces with more tool calls than this, only the first N are compared.
// This bounds worst-case time to O(maxToolSequenceLen^2).
const maxToolSequenceLen = 1000

// DriftReport is the result of comparing two fingerprints.
// Details contains dimension scores, risk breakdown, model/provider info,
// and the weights used. See Compare for the full key set.
type DriftReport struct {
	Score   float64
	Verdict string
	Details map[string]interface{}
}

// Compare produces a DriftReport from a baseline and candidate fingerprint.
// Both fingerprints must be non-nil (caller is responsible for validation).
func Compare(baseline, candidate *Fingerprint, cfg CompareConfig) *DriftReport {
	baseSeq := truncateSequence(baseline.ToolSequence, maxToolSequenceLen)
	candSeq := truncateSequence(candidate.ToolSequence, maxToolSequenceLen)

	toolOrder := toolOrderDistance(baseSeq, candSeq)
	toolFreq := toolFrequencyDistance(baseline.ToolFrequency, candidate.ToolFrequency)
	riskScore, riskDetails := riskShiftScore(baseline.RiskCounts, candidate.RiskCounts)
	tokenScore := tokenDeltaScore(baseline.TotalTokens, candidate.TotalTokens)

	score := cfg.ToolOrderWeight*toolOrder +
		cfg.ToolFrequencyWeight*toolFreq +
		cfg.RiskShiftWeight*riskScore +
		cfg.TokenDeltaWeight*tokenScore

	verdict := storage.DriftVerdictFail
	if score <= cfg.PassThreshold {
		verdict = storage.DriftVerdictPass
	} else if score <= cfg.WarnThreshold {
		verdict = storage.DriftVerdictWarn
	}

	// Check model and provider changes (informational only, not scored)
	modelChanged := !stringSlicesEqual(baseline.Models, candidate.Models)
	providerChanged := !stringSlicesEqual(baseline.Providers, candidate.Providers)

	details := map[string]interface{}{
		"tool_order_score":     toolOrder,
		"tool_frequency_score": toolFreq,
		"risk_shift_score":     riskScore,
		"token_delta_score":    tokenScore,
		"risk_details":         riskDetails,
		"model_changed":        modelChanged,
		"provider_changed":     providerChanged,
		"baseline_models":      baseline.Models,
		"candidate_models":     candidate.Models,
		"baseline_providers":   baseline.Providers,
		"candidate_providers":  candidate.Providers,
		"baseline_step_count":  baseline.StepCount,
		"candidate_step_count": candidate.StepCount,
		"baseline_tool_count":  baseline.TotalTools,
		"candidate_tool_count": candidate.TotalTools,
		"baseline_total_tokens":       baseline.TotalTokens,
		"candidate_total_tokens":      candidate.TotalTokens,
		"baseline_prompt_tokens":      baseline.PromptTokens,
		"candidate_prompt_tokens":     candidate.PromptTokens,
		"baseline_completion_tokens":  baseline.CompletionTokens,
		"candidate_completion_tokens": candidate.CompletionTokens,
		"weights": map[string]interface{}{
			"tool_order":     cfg.ToolOrderWeight,
			"tool_frequency": cfg.ToolFrequencyWeight,
			"risk_shift":     cfg.RiskShiftWeight,
			"token_delta":    cfg.TokenDeltaWeight,
		},
	}

	return &DriftReport{
		Score:   score,
		Verdict: verdict,
		Details: details,
	}
}

// truncateSequence returns at most maxLen elements from the sequence.
func truncateSequence(seq []string, maxLen int) []string {
	if len(seq) <= maxLen {
		return seq
	}
	return seq[:maxLen]
}

// toolOrderDistance computes normalized Levenshtein distance between two
// ordered tool sequences. Two-row DP, O(n*m) time, O(min(n,m)) space.
func toolOrderDistance(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 1.0
	}

	// Ensure a is the shorter sequence for space optimization
	if len(a) > len(b) {
		a, b = b, a
	}

	prev := make([]int, len(a)+1)
	curr := make([]int, len(a)+1)

	for i := range prev {
		prev[i] = i
	}

	for j := 1; j <= len(b); j++ {
		curr[0] = j
		for i := 1; i <= len(a); i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[i] = min(
				curr[i-1]+1,    // insert
				prev[i]+1,      // delete
				prev[i-1]+cost, // substitute
			)
		}
		prev, curr = curr, prev
	}

	maxLen := max(len(a), len(b))
	return float64(prev[len(a)]) / float64(maxLen)
}

// toolFrequencyDistance computes 1 - cosine similarity between two
// tool frequency maps. Scale-invariant: proportional distributions score 0.
func toolFrequencyDistance(a, b map[string]int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 1.0
	}

	// Collect all keys
	keys := make(map[string]struct{})
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}

	var dotProduct, magA, magB float64
	for k := range keys {
		va := float64(a[k])
		vb := float64(b[k])
		dotProduct += va * vb
		magA += va * va
		magB += vb * vb
	}

	if magA == 0 || magB == 0 {
		return 1.0
	}

	cosine := dotProduct / (math.Sqrt(magA) * math.Sqrt(magB))
	// Clamp to [0, 1] to handle floating point imprecision
	cosine = math.Min(1.0, math.Max(0.0, cosine))

	return 1.0 - cosine
}

// riskShiftScore computes an asymmetric weighted delta on risk fractions.
// Escalation (read→write, read→destructive) is penalized more heavily than de-escalation.
// Only known risk classes (read/write/destructive) are scored; "unknown" is excluded.
// Returns score in [0,1] and a details map.
func riskShiftScore(base, cand map[string]int) (float64, map[string]interface{}) {
	baseFrac := riskFractions(base)
	candFrac := riskFractions(cand)

	// Escalation weights: how much to penalize risk level increases
	// read→write: 2x, read→destructive: 3x, write→destructive: 2x
	// De-escalation: 1x (still counts, but less)
	riskLevels := []struct {
		name   string
		weight float64
	}{
		{storage.RiskClassRead, 1.0},
		{storage.RiskClassWrite, 2.0},
		{storage.RiskClassDestructive, 3.0},
	}

	var totalPenalty float64
	escalation := false

	for _, level := range riskLevels {
		basePct := baseFrac[level.name]
		candPct := candFrac[level.name]
		delta := candPct - basePct

		if delta > 0 {
			// Escalation: candidate has more of this risk level
			totalPenalty += delta * level.weight
			if level.name == storage.RiskClassWrite || level.name == storage.RiskClassDestructive {
				escalation = true
			}
		} else if delta < 0 {
			// De-escalation: 1x weight
			totalPenalty += (-delta) * 1.0
		}
	}

	// Normalize by max theoretical penalty sum (4.0):
	// max escalation: read goes from 1.0→0.0 (delta -1.0, weight 1.0 = 1.0)
	// and destructive goes from 0.0→1.0 (delta 1.0, weight 3.0 = 3.0) = 4.0
	const maxPenalty = 4.0
	score := math.Min(1.0, totalPenalty/maxPenalty)

	details := map[string]interface{}{
		"baseline_fractions":  baseFrac,
		"candidate_fractions": candFrac,
		"escalation":          escalation,
	}

	return score, details
}

// riskFractions converts raw risk counts to fractions of total.
// Only known risk classes (read/write/destructive) contribute to the total
// and the fractions. Unknown classes are excluded from the denominator
// to avoid diluting the scoring of known risk behaviors.
func riskFractions(counts map[string]int) map[string]float64 {
	fracs := map[string]float64{
		storage.RiskClassRead:        0.0,
		storage.RiskClassWrite:       0.0,
		storage.RiskClassDestructive: 0.0,
	}

	total := 0
	for k, v := range counts {
		if knownRiskClasses[k] {
			total += v
		}
	}

	if total == 0 {
		return fracs
	}

	for k, v := range counts {
		if knownRiskClasses[k] {
			fracs[k] = float64(v) / float64(total)
		}
	}
	return fracs
}

// tokenDeltaScore computes a log-dampened score for token count divergence.
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

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
