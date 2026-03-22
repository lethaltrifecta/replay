package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/diff"
)

func TestFindRepoRootForDemo(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o644))
	nested := filepath.Join(root, "cmd", "cmdr", "commands")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	// Change to nested dir to test finding root
	oldCwd, _ := os.Getwd()
	os.Chdir(nested)
	defer os.Chdir(oldCwd)

	found, err := findRepoRootForDemo()

	require.NoError(t, err)
	
	evalRoot, _ := filepath.EvalSymlinks(root)
	evalFound, _ := filepath.EvalSymlinks(found)
	assert.Equal(t, evalRoot, evalFound)
}

func TestRenderMigrationDemoReportMarkdown(t *testing.T) {
	report := renderMigrationDemoReportMarkdown(&migrationDemoReportArtifact{
		GeneratedAt:    time.Date(2026, time.March, 8, 12, 34, 56, 0, time.UTC),
		ReportDir:      "/tmp/migration-demo",
		Scenario:       "database-migration",
		JudgeHighlight: "CMDR blocked an unsafe replay after it diverged from the approved baseline. Baseline trace: `baseline-trace`.",
		Summary: &migrationDemoRunSummary{
			BaselineTraceID:     "baseline-trace",
			SafeReplayTraceID:   "safe-trace",
			UnsafeReplayTraceID: "unsafe-trace",
			Logs: migrationDemoLogPaths{
				RunLog:        "/tmp/migration-demo/run.log",
				SafeVerdict:   "/tmp/migration-demo/safe-verdict.log",
				UnsafeVerdict: "/tmp/migration-demo/unsafe-verdict.log",
			},
		},
		Safe: &migrationDemoVerdictInfo{
			Label:           "safe-replay",
			TraceID:         "safe-trace",
			FirstDivergence: "",
			Comparison: &traceComparisonReport{
				BaselineStepCount:   5,
				CandidateStepCount:  5,
				BaselineToolCount:   4,
				CandidateToolCount:  4,
				CandidateToolSource: traceToolSourceCaptures,
				Report: &diff.Report{
					SimilarityScore: 1.0,
					Verdict:         "pass",
				},
			},
		},
		Unsafe: &migrationDemoVerdictInfo{
			Label:           "unsafe-replay",
			TraceID:         "unsafe-trace",
			FirstDivergence: `tool #0 changed: baseline="inspect_schema" variant="drop_table"`,
			Comparison: &traceComparisonReport{
				BaselineStepCount:   5,
				CandidateStepCount:  2,
				BaselineToolCount:   4,
				CandidateToolCount:  1,
				CandidateToolSource: traceToolSourceCaptures,
				Report: &diff.Report{
					SimilarityScore: 0.14,
					Verdict:         "fail",
				},
			},
		},
	})

	assert.Contains(t, report, "# Migration Demo Report")
	assert.Contains(t, report, "## Executive Summary")
	assert.Contains(t, report, "CMDR blocked an unsafe replay")
	assert.Contains(t, report, "`baseline-trace`")
	assert.Contains(t, report, "### safe-replay")
	assert.Contains(t, report, "### unsafe-replay")
	assert.Contains(t, report, `baseline="inspect_schema" variant="drop_table"`)
	assert.Contains(t, report, "/tmp/migration-demo/run.log")
}

func TestMigrationJudgeHighlight(t *testing.T) {
	highlight := migrationJudgeHighlight(&migrationDemoRunSummary{
		BaselineTraceID: "baseline-trace",
	}, &traceComparisonReport{
		Report: &diff.Report{
			FirstDivergence: map[string]interface{}{
				"type":       "tool_sequence",
				"tool_index": 0,
				"baseline":   "inspect_schema",
				"variant":    "drop_table",
			},
		},
	})

	assert.Contains(t, highlight, "agentgateway")
	assert.Contains(t, highlight, "inspect_schema")
	assert.Contains(t, highlight, "baseline-trace")
}

func TestSetEnvValue_ReplacesExistingEntry(t *testing.T) {
	env := []string{"FOO=old", "BAR=value"}

	updated := setEnvValue(env, "FOO", "new")

	assert.Equal(t, []string{"FOO=new", "BAR=value"}, updated)
}
