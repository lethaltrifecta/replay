package commands

import (
	"fmt"
	"math"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

func formatFirstDivergence(divergence storage.FirstDivergence) string {
	if divergence.Type == "" {
		return ""
	}

	switch divergence.Type {
	case "tool_sequence":
		return fmt.Sprintf("tool #%d changed: baseline=%q variant=%q", divergence.StepIndex, divergence.Baseline, divergence.Variant)
	case "tool_count":
		return fmt.Sprintf("tool count diverged at tool #%d: baseline=%s variant=%s", divergence.StepIndex, divergence.Baseline, divergence.Variant)
	case "response_content":
		return fmt.Sprintf("step %d response changed: baseline=%q variant=%q", divergence.StepIndex, divergence.Baseline, divergence.Variant)
	case "step_count":
		return fmt.Sprintf("step count diverged at step %d: baseline=%s variant=%s", divergence.StepIndex, divergence.Baseline, divergence.Variant)
	default:
		return fmt.Sprintf("type=%s step=%d baseline=%q variant=%q", divergence.Type, divergence.StepIndex, divergence.Baseline, divergence.Variant)
	}
}

// toStructuredDivergence helper to convert legacy JSONB from diff.Report to structured type.
func toStructuredDivergence(m storage.JSONB) storage.FirstDivergence {
	if len(m) == 0 {
		return storage.FirstDivergence{}
	}
	var d storage.FirstDivergence
	if idx, ok := intValue(m["step_index"]); ok {
		d.StepIndex = idx
	} else if idx, ok := intValue(m["tool_index"]); ok {
		d.StepIndex = idx
	}
	d.Type, _ = m["type"].(string)
	d.Baseline, _ = m["baseline"].(string)
	if d.Baseline == "" {
		d.Baseline, _ = m["baseline_count"].(string)
	}
	if d.Baseline == "" {
		d.Baseline, _ = m["baseline_excerpt"].(string)
	}
	if d.Baseline == "" {
		d.Baseline, _ = m["baseline_steps"].(string)
	}
	d.Variant, _ = m["variant"].(string)
	if d.Variant == "" {
		d.Variant, _ = m["variant_count"].(string)
	}
	if d.Variant == "" {
		d.Variant, _ = m["variant_excerpt"].(string)
	}
	if d.Variant == "" {
		d.Variant, _ = m["variant_steps"].(string)
	}
	return d
}

func intValue(v interface{}) (int, bool) {
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

func floatValue(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func verdictDisplay(verdict string) string {
	switch verdict {
	case storage.DriftVerdictPass:
		return "PASS"
	case storage.DriftVerdictWarn:
		return "WARN"
	case storage.DriftVerdictFail:
		return "FAIL"
	default:
		return verdict
	}
}
