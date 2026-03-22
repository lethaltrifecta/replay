package drift

import "github.com/lethaltrifecta/replay/pkg/storage"

// StructuredDetails projects a drift report into the persisted summary shape
// exposed by storage/API consumers.
func StructuredDetails(report *DriftReport) storage.DriftDetails {
	if report == nil {
		return storage.DriftDetails{}
	}

	details := storage.DriftDetails{
		Reason:         summarizeReason(report.Details),
		RiskEscalation: driftRiskEscalation(report.Details),
	}

	return details
}

func summarizeReason(details map[string]interface{}) string {
	if driftRiskEscalation(details) {
		return "risk escalation detected"
	}
	if modelChanged(details) && providerChanged(details) {
		return "model and provider changed"
	}
	if modelChanged(details) {
		return "model changed"
	}
	if providerChanged(details) {
		return "provider changed"
	}

	bestKey := ""
	bestScore := 0.0
	scoreKeys := []struct {
		key    string
		reason string
	}{
		{key: "tool_order_score", reason: "tool order changed"},
		{key: "tool_frequency_score", reason: "tool usage frequency changed"},
		{key: "risk_shift_score", reason: "risk profile changed"},
		{key: "token_delta_score", reason: "token usage changed"},
	}
	for _, candidate := range scoreKeys {
		score, ok := float64Value(details[candidate.key])
		if !ok || score <= bestScore {
			continue
		}
		bestKey = candidate.reason
		bestScore = score
	}

	return bestKey
}

func driftRiskEscalation(details map[string]interface{}) bool {
	riskDetails, ok := details["risk_details"].(map[string]interface{})
	if !ok {
		return false
	}
	escalation, ok := riskDetails["escalation"].(bool)
	return ok && escalation
}

func modelChanged(details map[string]interface{}) bool {
	changed, ok := details["model_changed"].(bool)
	return ok && changed
}

func providerChanged(details map[string]interface{}) bool {
	changed, ok := details["provider_changed"].(bool)
	return ok && changed
}

func float64Value(v any) (float64, bool) {
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
