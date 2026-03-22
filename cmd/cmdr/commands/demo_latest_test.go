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
)

func TestFindLatestMigrationDemoReportDir(t *testing.T) {
	root := t.TempDir()
	older := filepath.Join(root, "20260308-120000")
	newer := filepath.Join(root, "20260308-130000")

	writeDemoArtifactDir(t, older, time.Date(2026, time.March, 8, 12, 0, 0, 0, time.UTC))
	writeDemoArtifactDir(t, newer, time.Date(2026, time.March, 8, 13, 0, 0, 0, time.UTC))

	latest, err := findLatestMigrationDemoReportDir(root)

	require.NoError(t, err)
	assert.Equal(t, newer, latest)
}

func TestResolveMigrationDemoPaths(t *testing.T) {
	t.Run("artifact path", func(t *testing.T) {
		paths := buildMigrationDemoArtifactPaths("/tmp/migration-demo")

		tests := []struct {
			name       string
			artifact   string
			wantPath   string
			wantErrMsg string
		}{
			{name: "highlight", artifact: "highlight", wantPath: "/tmp/migration-demo/judge-highlight.md"},
			{name: "invalid", artifact: "unknown", wantErrMsg: `invalid --artifact "unknown"`},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				path, err := resolveMigrationDemoArtifactPath(paths, tt.artifact)
				if tt.wantErrMsg != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.wantErrMsg)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, tt.wantPath, path)
			})
		}
	})

	t.Run("artifacts root", func(t *testing.T) {
		tests := []struct {
			name      string
			flagValue string
			repoRoot  string
			want      string
		}{
			{name: "default", repoRoot: "/tmp/replay", want: "/tmp/replay/artifacts/migration-demo"},
			{name: "relative override", flagValue: "tmp/demo", repoRoot: "/repo", want: "/repo/tmp/demo"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cmd := &cobra.Command{}
				cmd.Flags().String("artifacts-root", "", "")
				require.NoError(t, cmd.Flags().Set("artifacts-root", tt.flagValue))

				root, err := resolveMigrationDemoArtifactsRoot(cmd, tt.repoRoot)
				require.NoError(t, err)
				assert.Equal(t, tt.want, root)
			})
		}
	})
}

func TestLoadMigrationDemoReportArtifact(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		reportPath := filepath.Join(dir, "report.json")
		require.NoError(t, writeJSONFile(reportPath, sampleMigrationArtifact(dir)))

		artifact, err := loadMigrationDemoReportArtifact(reportPath)

		require.NoError(t, err)
		assert.Equal(t, "database-migration", artifact.Scenario)
		assert.Equal(t, "baseline-trace", artifact.Summary.BaselineTraceID)
		assert.Equal(t, "unsafe-trace", artifact.Unsafe.TraceID)
	})

	t.Run("missing sections", func(t *testing.T) {
		dir := t.TempDir()
		reportPath := filepath.Join(dir, "report.json")
		require.NoError(t, writeJSONFile(reportPath, map[string]any{
			"generatedAt": time.Date(2026, time.March, 8, 12, 34, 56, 0, time.UTC),
			"scenario":    "database-migration",
		}))

		artifact, err := loadMigrationDemoReportArtifact(reportPath)
		require.Error(t, err)
		assert.Nil(t, artifact)
		assert.Contains(t, err.Error(), "missing required sections")
	})
}

func TestPrintMigrationLatest(t *testing.T) {
	out := &strings.Builder{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)

	printMigrationLatest(cmd, &migrationDemoLatestOutput{
		Paths:    buildMigrationDemoArtifactPaths("/tmp/migration-demo"),
		Artifact: sampleMigrationArtifact("/tmp/migration-demo"),
	})

	output := out.String()
	assert.Contains(t, output, "Latest Migration Demo Artifact")
	assert.Contains(t, output, "PASS")
	assert.Contains(t, output, "FAIL")
	assert.Contains(t, output, "judge-highlight.md")
	assert.Contains(t, output, "tool #0 changed")
}

func writeDemoArtifactDir(t *testing.T, dir string, modTime time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, writeJSONFile(filepath.Join(dir, "report.json"), sampleMigrationArtifact(dir)))
	require.NoError(t, os.Chtimes(dir, modTime, modTime))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "report.json"), modTime, modTime))
}

func sampleMigrationArtifact(dir string) *migrationDemoReportArtifact {
	return &migrationDemoReportArtifact{
		GeneratedAt:    time.Date(2026, time.March, 8, 12, 34, 56, 0, time.UTC),
		ReportDir:      dir,
		Scenario:       "database-migration",
		JudgeHighlight: "CMDR blocked an unsafe replay after it diverged from the approved baseline.",
		Summary: &migrationDemoRunSummary{
			BaselineTraceID:     "baseline-trace",
			SafeReplayTraceID:   "safe-trace",
			UnsafeReplayTraceID: "unsafe-trace",
			Logs: migrationDemoLogPaths{
				RunLog: "/tmp/migration-demo/run.log",
			},
		},
		Safe: &migrationDemoVerdictInfo{
			Label:   "safe-replay",
			TraceID: "safe-trace",
			Comparison: &traceComparisonReport{
				Report: &diff.Report{
					Verdict:         "pass",
					SimilarityScore: 1.0,
				},
			},
		},
		Unsafe: &migrationDemoVerdictInfo{
			Label:           "unsafe-replay",
			TraceID:         "unsafe-trace",
			FirstDivergence: `tool #0 changed: baseline="inspect_schema" variant="drop_table"`,
			Comparison: &traceComparisonReport{
				Report: &diff.Report{
					Verdict:         "fail",
					SimilarityScore: 0.14,
				},
			},
		},
	}
}
