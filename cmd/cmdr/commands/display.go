package commands

import (
	"fmt"

	"github.com/lethaltrifecta/replay/pkg/diff"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

func formatFirstDivergence(divergence storage.FirstDivergence) string {
	if divergence.Type == "" {
		return ""
	}

	index := divergenceIndex(divergence)
	baseline := divergenceValue(divergence.Baseline, divergence.BaselineExcerpt, divergence.BaselineCount, divergence.BaselineSteps)
	variant := divergenceValue(divergence.Variant, divergence.VariantExcerpt, divergence.VariantCount, divergence.VariantSteps)

	switch divergence.Type {
	case "tool_sequence":
		return fmt.Sprintf("tool #%d changed: baseline=%q variant=%q", index, baseline, variant)
	case "tool_count":
		return fmt.Sprintf("tool count diverged at tool #%d: baseline=%s variant=%s", index, baseline, variant)
	case "response_content":
		return fmt.Sprintf("step %d response changed: baseline=%q variant=%q", index, baseline, variant)
	case "step_count":
		return fmt.Sprintf("step count diverged at step %d: baseline=%s variant=%s", index, baseline, variant)
	default:
		return fmt.Sprintf("type=%s step=%d baseline=%q variant=%q", divergence.Type, index, baseline, variant)
	}
}

func toStructuredDivergence(m storage.JSONB) storage.FirstDivergence {
	return diff.StructuredFirstDivergence(m)
}

func divergenceIndex(divergence storage.FirstDivergence) int {
	if divergence.ToolIndex != nil {
		return *divergence.ToolIndex
	}
	if divergence.StepIndex != nil {
		return *divergence.StepIndex
	}
	return 0
}

func divergenceValue(primary string, excerpt string, count *int, steps *int) string {
	switch {
	case primary != "":
		return primary
	case excerpt != "":
		return excerpt
	case count != nil:
		return fmt.Sprintf("%d", *count)
	case steps != nil:
		return fmt.Sprintf("%d", *steps)
	default:
		return ""
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
