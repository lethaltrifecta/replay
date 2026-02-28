package drift

// CompareConfig holds weights and thresholds for drift comparison.
type CompareConfig struct {
	ToolOrderWeight     float64
	ToolFrequencyWeight float64
	RiskShiftWeight     float64
	TokenDeltaWeight    float64
	PassThreshold       float64 // <= this → "pass"
	WarnThreshold       float64 // <= this → "warn", else "fail"
}

// DefaultConfig returns the default comparison configuration.
func DefaultConfig() CompareConfig {
	return CompareConfig{
		ToolOrderWeight:     0.35,
		ToolFrequencyWeight: 0.25,
		RiskShiftWeight:     0.25,
		TokenDeltaWeight:    0.15,
		PassThreshold:       0.2,
		WarnThreshold:       0.5,
	}
}
