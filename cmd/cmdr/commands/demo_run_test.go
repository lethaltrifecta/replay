package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/diff"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

func TestFindRepoRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "scripts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "scripts", "test-migration-demo-full-loop.sh"), []byte("#!/bin/sh\n"), 0o755))
	nested := filepath.Join(root, "cmd", "cmdr", "commands")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	found, err := findRepoRoot(nested)

	require.NoError(t, err)
	assert.Equal(t, root, found)
}

func TestLoadMigrationDemoRunSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "scenario": "database-migration",
  "baseline_trace_id": "baseline",
  "safe_replay_trace_id": "safe",
  "unsafe_replay_trace_id": "unsafe",
  "logs": {
    "cmdr": "/tmp/cmdr.log"
  }
}`), 0o644))

	summary, err := loadMigrationDemoRunSummary(path)

	require.NoError(t, err)
	assert.Equal(t, "baseline", summary.BaselineTraceID)
	assert.Equal(t, "safe", summary.SafeReplayTraceID)
	assert.Equal(t, "unsafe", summary.UnsafeReplayTraceID)
	assert.Equal(t, "/tmp/cmdr.log", summary.Logs.CMDR)
}

func TestRenderMigrationDemoReportMarkdown(t *testing.T) {
	report := renderMigrationDemoReportMarkdown(&migrationDemoReportArtifact{
		GeneratedAt:    time.Date(2026, time.March, 8, 12, 34, 56, 0, time.UTC),
		ReportDir:      "/tmp/migration-demo",
		Scenario:       "database-migration",
		JudgeHighlight: "CMDR blocked an unsafe replay after it diverged from the approved baseline.",
		Summary: &migrationDemoRunSummary{
			BaselineTraceID:     "baseline-trace",
			SafeReplayTraceID:   "safe-trace",
			UnsafeReplayTraceID: "unsafe-trace",
			Logs: migrationDemoLogPaths{
				RunLog:              "/tmp/migration-demo/run.log",
				CMDR:                "/tmp/migration-demo/cmdr.log",
				FreezeMCP:           "/tmp/migration-demo/freeze.log",
				MigrationMCP:        "/tmp/migration-demo/mcp.log",
				MockLLM:             "/tmp/migration-demo/llm.log",
				CaptureAgentgateway: "/tmp/migration-demo/capture.log",
				ReplayAgentgateway:  "/tmp/migration-demo/replay.log",
				SafeVerdict:         "/tmp/migration-demo/safe-verdict.log",
				UnsafeVerdict:       "/tmp/migration-demo/unsafe-verdict.log",
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
	assert.Contains(t, report, "## Judge Highlight")
	assert.Contains(t, report, "CMDR blocked an unsafe replay")
	assert.Contains(t, report, "## Outcome Summary")
	assert.Contains(t, report, "## Demo Flow")
	assert.Contains(t, report, "`baseline-trace`")
	assert.Contains(t, report, "### safe-replay")
	assert.Contains(t, report, "### unsafe-replay")
	assert.Contains(t, report, "`PASS`")
	assert.Contains(t, report, "`FAIL`")
	assert.Contains(t, report, `baseline="inspect_schema" variant="drop_table"`)
	assert.Contains(t, report, "/tmp/migration-demo/run.log")
}

func TestRenderMigrationJudgeHighlightMarkdown(t *testing.T) {
	report := renderMigrationJudgeHighlightMarkdown(&migrationDemoReportArtifact{
		JudgeHighlight: "CMDR replayed a frozen migration and blocked the unsafe candidate.",
		Summary: &migrationDemoRunSummary{
			BaselineTraceID: "baseline-trace",
		},
		Safe: &migrationDemoVerdictInfo{
			Comparison: &traceComparisonReport{
				Report: &diff.Report{Verdict: "pass"},
			},
		},
		Unsafe: &migrationDemoVerdictInfo{
			FirstDivergence: `tool #0 changed: baseline="inspect_schema" variant="drop_table"`,
			Comparison: &traceComparisonReport{
				Report: &diff.Report{Verdict: "fail"},
			},
		},
	})

	assert.Contains(t, report, "# Judge Highlight")
	assert.Contains(t, report, "CMDR replayed a frozen migration")
	assert.Contains(t, report, "`PASS`")
	assert.Contains(t, report, "`FAIL`")
	assert.Contains(t, report, `baseline="inspect_schema" variant="drop_table"`)
}

func TestRenderMigrationDemoScriptMarkdown(t *testing.T) {
	script := renderMigrationDemoScriptMarkdown(&migrationDemoReportArtifact{
		ReportDir: "/tmp/migration-demo",
		Summary: &migrationDemoRunSummary{
			Logs: migrationDemoLogPaths{
				RunLog:        "/tmp/migration-demo/run.log",
				SafeVerdict:   "/tmp/migration-demo/safe-verdict.log",
				UnsafeVerdict: "/tmp/migration-demo/unsafe-verdict.log",
			},
		},
	})

	assert.Contains(t, script, "# Migration Demo Script")
	assert.Contains(t, script, "## Setup")
	assert.Contains(t, script, "`cmdr demo migration run --report-dir /tmp/migration-demo`")
	assert.Contains(t, script, "## Talking Points")
	assert.Contains(t, script, "`freeze-mcp` keeps the tool environment fixed")
	assert.Contains(t, script, "/tmp/migration-demo/safe-verdict.log")
}

func TestMigrationJudgeHighlight(t *testing.T) {
	highlight := migrationJudgeHighlight(&migrationDemoRunSummary{
		BaselineTraceID: "baseline-trace",
	}, &traceComparisonReport{
		Report: &diff.Report{
			FirstDivergence: storage.JSONB{
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

func TestResolveMigrationDemoPostgresURL_Default(t *testing.T) {
	previous, hadValue := os.LookupEnv("CMDR_POSTGRES_URL")
	_ = os.Unsetenv("CMDR_POSTGRES_URL")
	defer restoreEnv("CMDR_POSTGRES_URL", previous, hadValue)

	assert.Equal(t, defaultMigrationDemoPostgresURL, resolveMigrationDemoPostgresURL())
}

func TestResolveMigrationDemoReportDir_Default(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("report-dir", "", "")
	dir, err := resolveMigrationDemoReportDir(cmd, "/tmp/replay")

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(dir, filepath.Join("/tmp/replay", "artifacts", "migration-demo")))
}

func TestSetEnvValue_ReplacesExistingEntry(t *testing.T) {
	env := []string{"FOO=old", "BAR=value"}

	updated := setEnvValue(env, "FOO", "new")

	assert.Equal(t, []string{"FOO=new", "BAR=value"}, updated)
}
