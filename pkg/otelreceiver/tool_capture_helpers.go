package otelreceiver

import "github.com/lethaltrifecta/replay/pkg/storage"

// CalculateCaptureArgsHash exposes the production args hashing logic for tests
// and tooling that need to seed captures with matcher-compatible hashes.
func CalculateCaptureArgsHash(args storage.JSONB) string {
	return calculateArgsHash(args)
}

// DetermineCaptureRiskClass exposes production risk classification to keep
// seeded captures aligned with receiver behavior.
func DetermineCaptureRiskClass(toolName string, args storage.JSONB) string {
	return determineRiskClass(toolName, args)
}
