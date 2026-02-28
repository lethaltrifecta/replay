package drift

import (
	"fmt"
	"strings"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

// knownRiskClasses defines the valid risk classes for normalization.
var knownRiskClasses = map[string]bool{
	storage.RiskClassRead:        true,
	storage.RiskClassWrite:       true,
	storage.RiskClassDestructive: true,
}

// Fingerprint is a behavioral signature of a trace, computed from
// replay spans and tool captures. It is not stored — always derived
// on-the-fly from raw trace data.
type Fingerprint struct {
	TraceID          string
	StepCount        int
	Models           []string       // model per step (ordered)
	Providers        []string       // provider per step (ordered)
	ToolSequence     []string       // ordered tool names
	ToolFrequency    map[string]int // tool name → count
	RiskCounts       map[string]int // "read"/"write"/"destructive"/"unknown" → count
	TotalTools       int
	TotalTokens      int
	PromptTokens     int
	CompletionTokens int
	AvgTokens        float64
	MaxTokens        int
}

// Extract builds a Fingerprint from pre-sorted replay spans and tool captures.
// Both inputs must be sorted by step_index ASC with deterministic tiebreakers
// (the storage layer guarantees step_index ASC, created_at ASC, id ASC).
// Returns an error if spans is empty.
func Extract(traceID string, spans []*storage.ReplayTrace, tools []*storage.ToolCapture) (*Fingerprint, error) {
	if len(spans) == 0 {
		return nil, fmt.Errorf("cannot extract fingerprint: no spans provided")
	}

	fp := &Fingerprint{
		TraceID:       traceID,
		StepCount:     len(spans),
		Models:        make([]string, 0, len(spans)),
		Providers:     make([]string, 0, len(spans)),
		ToolSequence:  make([]string, 0, len(tools)),
		ToolFrequency: make(map[string]int),
		RiskCounts:    make(map[string]int),
	}

	// Extract model, provider, and token stats from spans
	for _, s := range spans {
		fp.Models = append(fp.Models, s.Model)
		fp.Providers = append(fp.Providers, s.Provider)
		fp.TotalTokens += s.TotalTokens
		fp.PromptTokens += s.PromptTokens
		fp.CompletionTokens += s.CompletionTokens
		if s.TotalTokens > fp.MaxTokens {
			fp.MaxTokens = s.TotalTokens
		}
	}
	fp.AvgTokens = float64(fp.TotalTokens) / float64(fp.StepCount)

	// Extract tool sequence, frequency, and risk from tool captures
	for _, t := range tools {
		fp.ToolSequence = append(fp.ToolSequence, t.ToolName)
		fp.ToolFrequency[t.ToolName]++
		rc := normalizeRiskClass(t.RiskClass)
		if rc != "" {
			fp.RiskCounts[rc]++
		}
	}
	fp.TotalTools = len(tools)

	return fp, nil
}

// normalizeRiskClass lowercases, trims, and validates a risk class string.
// Unknown non-empty values are mapped to "unknown".
func normalizeRiskClass(rc string) string {
	rc = strings.ToLower(strings.TrimSpace(rc))
	if rc == "" {
		return ""
	}
	if knownRiskClasses[rc] {
		return rc
	}
	return "unknown"
}
