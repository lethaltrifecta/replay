package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/otelreceiver"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

func TestBuildBaselineTrace_ChangeContext(t *testing.T) {
	replays, tools := buildBaselineTrace()

	require.NotEmpty(t, replays)
	require.NotEmpty(t, tools)

	// Every step carries change_context metadata.
	for i, r := range replays {
		raw, ok := r.Metadata["change_context"]
		require.True(t, ok, "step %d: missing change_context", i)

		ctx, ok := raw.(map[string]any)
		require.True(t, ok, "step %d: change_context is not a map", i)

		assert.Equal(t, "instruction_file", ctx["kind"])
		assert.Equal(t, "role.md", ctx["target"])
		assert.Equal(t, "safe (v1.2)", ctx["baseline_label"])
		assert.Equal(t, "aggressive (v1.3)", ctx["candidate_label"])
		assert.NotEmpty(t, ctx["summary"])
	}
}

func TestBuildDriftedTrace_ChangeContext(t *testing.T) {
	replays, _ := buildDriftedTrace()

	require.NotEmpty(t, replays)

	ctx, ok := replays[0].Metadata["change_context"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "instruction_file", ctx["kind"])
	assert.Equal(t, "role.md", ctx["target"])
}

func TestDemoTraces_DifferentPromptsSameModel(t *testing.T) {
	baselineReplays, _ := buildBaselineTrace()
	driftedReplays, _ := buildDriftedTrace()

	require.NotEmpty(t, baselineReplays)
	require.NotEmpty(t, driftedReplays)

	// Same model on both — the story is "different instructions, not different model."
	assert.Equal(t, baselineReplays[0].Model, driftedReplays[0].Model)
	assert.Equal(t, baselineReplays[0].Provider, driftedReplays[0].Provider)

	// Different system prompts.
	baselineMessages := baselineReplays[0].Prompt["messages"].([]map[string]string)
	driftedMessages := driftedReplays[0].Prompt["messages"].([]map[string]string)

	assert.NotEqual(t, baselineMessages[0]["content"], driftedMessages[0]["content"],
		"system prompts must differ between baseline and drifted traces")

	// Same user message.
	assert.Equal(t, baselineMessages[1]["content"], driftedMessages[1]["content"],
		"user message must be identical across both traces")
}

func TestDemoArgsHash_MatchesProduction(t *testing.T) {
	args := storage.JSONB{
		"path":    "src/auth/module.ts",
		"retries": 3,
		"limits":  storage.JSONB{"timeout_ms": 1500},
	}

	// Compare against the production helper to lock the demo onto the same normalization path.
	expected := otelreceiver.CalculateCaptureArgsHash(args)
	actual := demoArgsHash(args)

	assert.NotEmpty(t, actual)
	assert.Equal(t, expected, actual, "demo args hash must match production capture hashing")
}
