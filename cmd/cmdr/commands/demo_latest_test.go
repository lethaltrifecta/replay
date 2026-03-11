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

func TestResolveMigrationDemoArtifactPath(t *testing.T) {
	paths := buildMigrationDemoArtifactPaths("/tmp/migration-demo")

	path, err := resolveMigrationDemoArtifactPath(paths, "highlight")

	require.NoError(t, err)
	assert.Equal(t, "/tmp/migration-demo/judge-highlight.md", path)
}

func TestResolveMigrationDemoArtifactsRoot_Default(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("artifacts-root", "", "")

	root, err := resolveMigrationDemoArtifactsRoot(cmd, "/tmp/replay")

	require.NoError(t, err)
	assert.Equal(t, "/tmp/replay/artifacts/migration-demo", root)
}

func TestLoadMigrationDemoReportArtifact(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	require.NoError(t, writeJSONFile(reportPath, sampleMigrationArtifact(dir)))

	artifact, err := loadMigrationDemoReportArtifact(reportPath)

	require.NoError(t, err)
	assert.Equal(t, "database-migration", artifact.Scenario)
	assert.Equal(t, "baseline-trace", artifact.Summary.BaselineTraceID)
	assert.Equal(t, "unsafe-trace", artifact.Unsafe.TraceID)
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
