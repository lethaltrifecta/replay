package commands

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/diff"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

func TestGateCheck_ThresholdValidation(t *testing.T) {
	tests := []struct {
		name      string
		threshold string
		wantErr   string
	}{
		{"negative", "-0.5", "--threshold must be between 0.0 and 1.0"},
		{"above_one", "1.5", "--threshold must be between 0.0 and 1.0"},
		{"way_above", "2.0", "--threshold must be between 0.0 and 1.0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fresh command tree so flag state doesn't leak
			root := &cobra.Command{Use: "test"}
			checkCmd := &cobra.Command{
				Use:  "check",
				RunE: runGateCheck,
			}
			checkCmd.Flags().String("baseline", "", "")
			checkCmd.Flags().String("model", "", "")
			checkCmd.Flags().String("provider", "", "")
			checkCmd.Flags().Float64("threshold", 0.8, "")
			root.AddCommand(checkCmd)

			out := &strings.Builder{}
			root.SetOut(out)
			root.SetErr(out)

			root.SetArgs([]string{"check", "--baseline", "test-trace", "--model", "gpt-4", "--threshold", tc.threshold})
			err := root.Execute()

			// Threshold validation runs before config.Load/connectDB,
			// so the error should be about the threshold, not a DB failure.
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestGateCheck_ValidThresholds(t *testing.T) {
	// Valid threshold values should NOT trigger validation error.
	// They will fail later (config.Load/connectDB), but not on threshold.
	tests := []struct {
		name      string
		threshold string
	}{
		{"zero", "0.0"},
		{"one", "1.0"},
		{"typical", "0.8"},
		{"half", "0.5"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{Use: "test"}
			checkCmd := &cobra.Command{
				Use:  "check",
				RunE: runGateCheck,
			}
			checkCmd.Flags().String("baseline", "", "")
			checkCmd.Flags().String("model", "", "")
			checkCmd.Flags().String("provider", "", "")
			checkCmd.Flags().Float64("threshold", 0.8, "")
			root.AddCommand(checkCmd)

			out := &strings.Builder{}
			root.SetOut(out)
			root.SetErr(out)

			root.SetArgs([]string{"check", "--baseline", "test-trace", "--model", "gpt-4", "--threshold", tc.threshold})
			err := root.Execute()

			// Should fail on config.Load or connectDB, NOT threshold validation
			if err != nil {
				assert.NotContains(t, err.Error(), "--threshold must be between")
			}
		})
	}
}

func TestPrintGateReport(t *testing.T) {
	out := &strings.Builder{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)

	report := &diff.Report{
		SimilarityScore: 0.9200,
		Verdict:         "pass",
		StepCount:       3,
		FirstDivergence: storage.JSONB{
			"type":       "tool_sequence",
			"tool_index": 1,
			"baseline":   "describe_pod",
			"variant":    "delete_pod",
		},
		StepDiffs: []diff.StepDiff{
			{StepIndex: 0, TokenDelta: 42},
			{StepIndex: 1, TokenDelta: -18},
			{StepIndex: 2, TokenDelta: 10},
		},
		TokenDelta:   34,
		LatencyDelta: 150,
	}

	expID := uuid.New()
	printGateReport(cmd, "baseline-trace-123", "gpt-4o", expID, report)

	output := out.String()
	assert.Contains(t, output, "Gate Check Result")
	assert.Contains(t, output, "baseline-trace-123")
	assert.Contains(t, output, "gpt-4o")
	assert.Contains(t, output, "3 replayed")
	assert.Contains(t, output, "0.9200")
	assert.Contains(t, output, "PASS")
	assert.Contains(t, output, "First Divergence")
	assert.Contains(t, output, `baseline="describe_pod" variant="delete_pod"`)
	assert.Contains(t, output, "Step Breakdown")
	assert.Contains(t, output, "+42")
	assert.Contains(t, output, "-18")
	assert.Contains(t, output, "token_delta=+34")
	assert.Contains(t, output, "latency_delta=+150ms")
	assert.Contains(t, output, expID.String())
}

func TestPrintGateReport_FailVerdict(t *testing.T) {
	out := &strings.Builder{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)

	report := &diff.Report{
		SimilarityScore: 0.4500,
		Verdict:         "fail",
		StepCount:       2,
		StepDiffs: []diff.StepDiff{
			{StepIndex: 0, TokenDelta: 500},
			{StepIndex: 1, TokenDelta: -300},
		},
		TokenDelta:   200,
		LatencyDelta: -50,
	}

	printGateReport(cmd, "baseline-456", "claude-3-5-sonnet", uuid.New(), report)

	output := out.String()
	assert.Contains(t, output, "FAIL")
	assert.Contains(t, output, "0.4500")
}

func TestPrintGateReport_NoStepDiffs(t *testing.T) {
	out := &strings.Builder{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)

	report := &diff.Report{
		SimilarityScore: 1.0,
		Verdict:         "pass",
		StepCount:       0,
	}

	printGateReport(cmd, "baseline-123", "gpt-4", uuid.New(), report)

	output := out.String()
	assert.Contains(t, output, "Gate Check Result")
	assert.NotContains(t, output, "Step Breakdown")
}

func TestErrGateFailed_IsSentinel(t *testing.T) {
	err := ErrGateFailed
	assert.True(t, errors.Is(err, ErrGateFailed))
	assert.Equal(t, "gate check failed", err.Error())

	// Wrapped errors should still match
	wrapped := errors.New("other error")
	assert.False(t, errors.Is(wrapped, ErrGateFailed))
}

func TestResolveToolComparisonInputs_FallbackOnCaptureError(t *testing.T) {
	cmd := &cobra.Command{}
	errOut := &strings.Builder{}
	cmd.SetErr(errOut)

	captures, tools := resolveToolComparisonInputs(
		cmd,
		nil,
		errors.New("db unavailable"),
		[]*storage.ReplayTrace{
			{
				Metadata: storage.JSONB{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"function": map[string]interface{}{
								"name": "delete_user",
							},
						},
					},
				},
			},
		},
	)

	assert.Nil(t, captures)
	assert.Nil(t, tools)
	assert.Contains(t, errOut.String(), "falling back to 4-dimension structural+response diff")
}

func TestResolveToolComparisonInputs_UsesSemanticWhenCapturesLoad(t *testing.T) {
	cmd := &cobra.Command{}
	errOut := &strings.Builder{}
	cmd.SetErr(errOut)

	inputCaptures := []*storage.ToolCapture{
		{ToolName: "get_user", RiskClass: "read"},
	}
	captures, tools := resolveToolComparisonInputs(
		cmd,
		inputCaptures,
		nil,
		[]*storage.ReplayTrace{
			{
				Metadata: storage.JSONB{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"function": map[string]interface{}{
								"name": "get_user",
							},
						},
					},
				},
			},
		},
	)

	require.Len(t, captures, 1)
	assert.Equal(t, "get_user", captures[0].ToolName)
	require.Len(t, tools, 1)
	assert.Equal(t, "get_user", tools[0].Name)
	assert.Empty(t, errOut.String())
}

func TestFormatFirstDivergence_ResponseContent(t *testing.T) {
	summary := formatFirstDivergence(storage.JSONB{
		"type":             "response_content",
		"step_index":       2,
		"jaccard":          0.33,
		"baseline_excerpt": "Investigate pod state first",
		"variant_excerpt":  "Delete the pod immediately",
	})

	assert.Contains(t, summary, "step 2 response changed")
	assert.Contains(t, summary, "Investigate pod state first")
	assert.Contains(t, summary, "Delete the pod immediately")
}

func TestBuildReplayHeaders(t *testing.T) {
	headers, err := buildReplayHeaders("baseline-123", []string{
		"X-Debug=1",
		"X-Scenario=incident-triage",
	})

	require.NoError(t, err)
	assert.Equal(t, "baseline-123", headers["X-Freeze-Trace-Id"])
	assert.Equal(t, "1", headers["X-Debug"])
	assert.Equal(t, "incident-triage", headers["X-Scenario"])
}

func TestBuildReplayHeaders_InvalidSpec(t *testing.T) {
	headers, err := buildReplayHeaders("", []string{"not-valid"})

	require.Error(t, err)
	assert.Nil(t, headers)
	assert.Contains(t, err.Error(), "expected KEY=VALUE")
}

func TestBuildReplayHeaders_CaseDuplicateRejected(t *testing.T) {
	headers, err := buildReplayHeaders("", []string{
		"X-Custom=one",
		"x-custom=two",
	})

	require.Error(t, err)
	assert.Nil(t, headers)
	assert.Contains(t, err.Error(), "case-insensitive collision")
}

func TestBuildReplayHeaders_FreezeTraceIDCollision(t *testing.T) {
	headers, err := buildReplayHeaders("baseline-123", []string{
		"x-freeze-trace-id=other-value",
	})

	require.Error(t, err)
	assert.Nil(t, headers)
	assert.Contains(t, err.Error(), "case-insensitive collision")
}
