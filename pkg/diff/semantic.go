package diff

import (
	"math"
	"strings"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

// ToolCall is a normalized tool call for comparison.
// Works for both baseline ToolCaptures and variant API responses.
type ToolCall struct {
	Name      string
	RiskClass string // "read", "write", "destructive"
}

// ToolCallScore holds tool call comparison results.
type ToolCallScore struct {
	SequenceSimilarity  float64 `json:"sequence_similarity"`  // 1 - normalized Levenshtein on tool name sequences
	FrequencySimilarity float64 `json:"frequency_similarity"` // cosine similarity on tool name frequency vectors
	Score               float64 `json:"score"`                // blended: 0.5 * sequence + 0.5 * frequency
}

// RiskScore holds risk escalation comparison results.
type RiskScore struct {
	Score      float64                `json:"score"`      // [0, 1], higher = more similar (less escalation)
	Escalation bool                   `json:"escalation"` // true if any risk escalation detected
	Details    map[string]interface{} `json:"details"`    // baseline/candidate fractions
}

// ResponseScore holds response divergence comparison results.
type ResponseScore struct {
	LengthSimilarity float64 `json:"length_similarity"` // min(len_a, len_b) / max(len_a, len_b)
	ContentOverlap   float64 `json:"content_overlap"`   // Jaccard word overlap
	Score            float64 `json:"score"`             // blended: 0.3 * length + 0.7 * Jaccard
}

// CompareToolCalls computes sequence + frequency similarity.
// Returns nil if both slices are empty (no tool data available).
func CompareToolCalls(baseline, variant []ToolCall) *ToolCallScore {
	if len(baseline) == 0 && len(variant) == 0 {
		return nil
	}

	baseNames := toolNames(baseline)
	varNames := toolNames(variant)

	seqSim := 1.0 - levenshteinDistance(baseNames, varNames)
	freqSim := 1.0 - cosineDistance(frequencyMap(baseNames), frequencyMap(varNames))

	return &ToolCallScore{
		SequenceSimilarity:  seqSim,
		FrequencySimilarity: freqSim,
		Score:               0.5*seqSim + 0.5*freqSim,
	}
}

// CompareRiskProfiles computes asymmetric risk escalation score.
// Re-implements the algorithm from pkg/drift/compare.go:176 (riskShiftScore).
// Returns nil if both slices are empty.
func CompareRiskProfiles(baseline, variant []ToolCall) *RiskScore {
	if len(baseline) == 0 && len(variant) == 0 {
		return nil
	}

	baseCounts := riskCounts(baseline)
	varCounts := riskCounts(variant)

	baseFrac := riskFractions(baseCounts)
	varFrac := riskFractions(varCounts)

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
		varPct := varFrac[level.name]
		delta := varPct - basePct

		if delta > 0 {
			totalPenalty += delta * level.weight
			if level.name == storage.RiskClassWrite || level.name == storage.RiskClassDestructive {
				escalation = true
			}
		} else if delta < 0 {
			totalPenalty += (-delta) * 1.0
		}
	}

	const maxPenalty = 4.0
	score := math.Min(1.0, totalPenalty/maxPenalty)

	// Convert to similarity (1 - score) so higher = more similar
	similarity := 1.0 - score

	return &RiskScore{
		Score:      similarity,
		Escalation: escalation,
		Details: map[string]interface{}{
			"baseline_fractions":  baseFrac,
			"candidate_fractions": varFrac,
		},
	}
}

// CompareResponses computes length ratio + Jaccard word overlap for each step pair,
// returns aggregate score.
func CompareResponses(baseline, variant []*storage.ReplayTrace) *ResponseScore {
	if len(baseline) == 0 && len(variant) == 0 {
		return nil
	}

	minSteps := len(baseline)
	if len(variant) < minSteps {
		minSteps = len(variant)
	}

	if minSteps == 0 {
		// One side has steps, the other doesn't — maximum divergence
		return &ResponseScore{
			LengthSimilarity: 0.0,
			ContentOverlap:   0.0,
			Score:            0.0,
		}
	}

	var totalLength, totalJaccard float64
	for i := 0; i < minSteps; i++ {
		totalLength += lengthSimilarity(baseline[i].Completion, variant[i].Completion)
		totalJaccard += jaccard(baseline[i].Completion, variant[i].Completion)
	}

	avgLength := totalLength / float64(minSteps)
	avgJaccard := totalJaccard / float64(minSteps)

	// Penalize for mismatched step counts
	stepPenalty := 1.0
	if len(baseline) != len(variant) {
		maxSteps := max(len(baseline), len(variant))
		stepPenalty = float64(minSteps) / float64(maxSteps)
	}

	return &ResponseScore{
		LengthSimilarity: avgLength * stepPenalty,
		ContentOverlap:   avgJaccard * stepPenalty,
		Score:            (0.3*avgLength + 0.7*avgJaccard) * stepPenalty,
	}
}

// ClassifyToolRisk classifies a tool name into a risk class.
// Re-implements the heuristic from pkg/otelreceiver/parser.go:451 (determineRiskClass).
func ClassifyToolRisk(toolName string) string {
	lowerName := strings.ToLower(toolName)

	// Destructive operations
	if strings.Contains(lowerName, "delete") ||
		strings.Contains(lowerName, "remove") ||
		strings.Contains(lowerName, "drop") ||
		strings.Contains(lowerName, "terminate") {
		return storage.RiskClassDestructive
	}

	// Write operations
	if strings.Contains(lowerName, "write") ||
		strings.Contains(lowerName, "create") ||
		strings.Contains(lowerName, "update") ||
		strings.Contains(lowerName, "insert") ||
		strings.Contains(lowerName, "modify") ||
		strings.Contains(lowerName, "edit") {
		return storage.RiskClassWrite
	}

	return storage.RiskClassRead
}

// ExtractVariantToolCalls extracts ToolCall slice from variant ReplayTrace metadata.
// Looks for metadata["tool_calls"] which is set by the replay engine.
func ExtractVariantToolCalls(steps []*storage.ReplayTrace) []ToolCall {
	var calls []ToolCall

	for _, step := range steps {
		raw, ok := step.Metadata["tool_calls"]
		if !ok {
			continue
		}

		// tool_calls is stored as []interface{} after JSONB round-trip
		slice, ok := raw.([]interface{})
		if !ok {
			continue
		}

		for _, item := range slice {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			fn, ok := m["function"].(map[string]interface{})
			if !ok {
				continue
			}

			name, _ := fn["name"].(string)
			if name == "" {
				continue
			}

			calls = append(calls, ToolCall{
				Name:      name,
				RiskClass: ClassifyToolRisk(name),
			})
		}
	}

	return calls
}

// BaselineToolCalls converts storage.ToolCapture slice to ToolCall slice.
func BaselineToolCalls(captures []*storage.ToolCapture) []ToolCall {
	calls := make([]ToolCall, len(captures))
	for i, c := range captures {
		calls[i] = ToolCall{
			Name:      c.ToolName,
			RiskClass: c.RiskClass,
		}
	}
	return calls
}

// --- Internal helpers ---

// toolNames extracts an ordered slice of tool names.
func toolNames(calls []ToolCall) []string {
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Name
	}
	return names
}

// frequencyMap counts occurrences of each string.
func frequencyMap(names []string) map[string]int {
	freq := make(map[string]int, len(names))
	for _, n := range names {
		freq[n]++
	}
	return freq
}

// riskCounts counts occurrences of each risk class from a ToolCall slice.
func riskCounts(calls []ToolCall) map[string]int {
	counts := make(map[string]int)
	for _, c := range calls {
		counts[c.RiskClass]++
	}
	return counts
}

// levenshteinDistance computes normalized Levenshtein distance between two
// ordered string sequences. Two-row DP, O(n*m) time, O(min(n,m)) space.
// Re-implements drift/compare.go:97 (toolOrderDistance).
func levenshteinDistance(a, b []string) float64 {
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

// cosineDistance computes 1 - cosine similarity between two frequency maps.
// Re-implements drift/compare.go:137 (toolFrequencyDistance).
func cosineDistance(a, b map[string]int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 1.0
	}

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
	cosine = math.Min(1.0, math.Max(0.0, cosine))

	return 1.0 - cosine
}

// riskFractions converts raw risk counts to fractions of total.
// Only known risk classes contribute. Re-implements drift/compare.go:235.
func riskFractions(counts map[string]int) map[string]float64 {
	fracs := map[string]float64{
		storage.RiskClassRead:        0.0,
		storage.RiskClassWrite:       0.0,
		storage.RiskClassDestructive: 0.0,
	}

	total := 0
	for k, v := range counts {
		if k == storage.RiskClassRead || k == storage.RiskClassWrite || k == storage.RiskClassDestructive {
			total += v
		}
	}

	if total == 0 {
		return fracs
	}

	for k, v := range counts {
		if k == storage.RiskClassRead || k == storage.RiskClassWrite || k == storage.RiskClassDestructive {
			fracs[k] = float64(v) / float64(total)
		}
	}
	return fracs
}

// lengthSimilarity returns min(len_a, len_b) / max(len_a, len_b).
// Returns 1.0 if both are empty.
func lengthSimilarity(a, b string) float64 {
	la, lb := len(a), len(b)
	if la == 0 && lb == 0 {
		return 1.0
	}
	if la == 0 || lb == 0 {
		return 0.0
	}
	return float64(min(la, lb)) / float64(max(la, lb))
}

// jaccard computes word-level Jaccard similarity: |intersection| / |union|.
func jaccard(a, b string) float64 {
	wordsA := wordSet(a)
	wordsB := wordSet(b)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}

	var intersection int
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return 1.0
	}

	return float64(intersection) / float64(union)
}

// wordSet splits a string into a set of lowercase words.
func wordSet(s string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(s) {
		set[strings.ToLower(w)] = true
	}
	return set
}
