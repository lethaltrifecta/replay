package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type migrationDemoArtifactPaths struct {
	Dir           string `json:"dir"`
	Summary       string `json:"summary"`
	JSON          string `json:"json"`
	Report        string `json:"report"`
	Highlight     string `json:"highlight"`
	Script        string `json:"script"`
	RunLog        string `json:"run_log"`
	SafeVerdict   string `json:"safe_verdict"`
	UnsafeVerdict string `json:"unsafe_verdict"`
}

type migrationDemoLatestOutput struct {
	Paths    migrationDemoArtifactPaths   `json:"paths"`
	Artifact *migrationDemoReportArtifact `json:"artifact"`
}

func runMigrationLatest(cmd *cobra.Command, args []string) error {
	repoRoot, err := findRepoRootForDemo()
	if err != nil {
		return err
	}

	artifactsRoot, err := resolveMigrationDemoArtifactsRoot(cmd, repoRoot)
	if err != nil {
		return err
	}

	latestDir, err := findLatestMigrationDemoReportDir(artifactsRoot)
	if err != nil {
		return err
	}

	paths := buildMigrationDemoArtifactPaths(latestDir)
	artifact, err := loadMigrationDemoReportArtifact(paths.JSON)
	if err != nil {
		return err
	}

	output := &migrationDemoLatestOutput{
		Paths:    paths,
		Artifact: artifact,
	}

	artifactKey, _ := cmd.Flags().GetString("artifact")
	if artifactKey != "" {
		path, err := resolveMigrationDemoArtifactPath(paths, artifactKey)
		if err != nil {
			return err
		}
		cmd.Println(path)
		return nil
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal latest artifact output: %w", err)
		}
		data = append(data, '\n')
		_, _ = cmd.OutOrStdout().Write(data)
		return nil
	}

	printMigrationLatest(cmd, output)
	return nil
}

func resolveMigrationDemoArtifactsRoot(cmd *cobra.Command, repoRoot string) (string, error) {
	artifactsRoot, _ := cmd.Flags().GetString("artifacts-root")
	if artifactsRoot == "" {
		artifactsRoot = filepath.Join(repoRoot, "artifacts", "migration-demo")
	}
	if !filepath.IsAbs(artifactsRoot) {
		artifactsRoot = filepath.Join(repoRoot, artifactsRoot)
	}
	return filepath.Clean(artifactsRoot), nil
}

func findLatestMigrationDemoReportDir(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no migration demo artifacts found under %s; run `cmdr demo migration run` first or pass --artifacts-root", root)
		}
		return "", fmt.Errorf("read migration demo artifacts root: %w", err)
	}

	type candidate struct {
		path    string
		modTime time.Time
	}
	var candidates []candidate

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return "", fmt.Errorf("read artifact directory info for %s: %w", path, err)
		}
		if !fileExists(filepath.Join(path, "report.json")) {
			continue
		}
		candidates = append(candidates, candidate{path: path, modTime: info.ModTime()})
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no completed migration demo reports found under %s; run `cmdr demo migration run` first or pass --artifacts-root", root)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})

	return candidates[0].path, nil
}

func buildMigrationDemoArtifactPaths(dir string) migrationDemoArtifactPaths {
	return migrationDemoArtifactPaths{
		Dir:           dir,
		Summary:       filepath.Join(dir, "run-summary.json"),
		JSON:          filepath.Join(dir, "report.json"),
		Report:        filepath.Join(dir, "report.md"),
		Highlight:     filepath.Join(dir, "judge-highlight.md"),
		Script:        filepath.Join(dir, "demo-script.md"),
		RunLog:        filepath.Join(dir, "run.log"),
		SafeVerdict:   filepath.Join(dir, "safe-verdict.log"),
		UnsafeVerdict: filepath.Join(dir, "unsafe-verdict.log"),
	}
}

func resolveMigrationDemoArtifactPath(paths migrationDemoArtifactPaths, artifact string) (string, error) {
	switch artifact {
	case "dir":
		return paths.Dir, nil
	case "summary":
		return paths.Summary, nil
	case "json":
		return paths.JSON, nil
	case "report":
		return paths.Report, nil
	case "highlight":
		return paths.Highlight, nil
	case "script":
		return paths.Script, nil
	case "run-log":
		return paths.RunLog, nil
	case "safe-verdict":
		return paths.SafeVerdict, nil
	case "unsafe-verdict":
		return paths.UnsafeVerdict, nil
	default:
		return "", fmt.Errorf("invalid --artifact %q: expected dir|summary|json|report|highlight|script|run-log|safe-verdict|unsafe-verdict", artifact)
	}
}

func loadMigrationDemoReportArtifact(path string) (*migrationDemoReportArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read report artifact: %w", err)
	}

	var artifact migrationDemoReportArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, fmt.Errorf("decode report artifact: %w", err)
	}

	if artifact.Summary == nil || artifact.Safe == nil || artifact.Unsafe == nil {
		return nil, fmt.Errorf("report artifact missing required sections")
	}

	return &artifact, nil
}

func printMigrationLatest(cmd *cobra.Command, output *migrationDemoLatestOutput) {
	artifact := output.Artifact
	cmd.Printf("Latest Migration Demo Artifact\n")
	cmd.Printf("============================\n")
	cmd.Printf("Directory:   %s\n", output.Paths.Dir)
	cmd.Printf("Generated:   %s\n", artifact.GeneratedAt.Format("2006-01-02 15:04:05"))
	cmd.Printf("Baseline:    %s\n", artifact.Summary.BaselineTraceID)
	cmd.Printf("Safe Trace:  %s\n", artifact.Summary.SafeReplayTraceID)
	cmd.Printf("Unsafe Trace: %s\n", artifact.Summary.UnsafeReplayTraceID)
	cmd.Printf("\n")
	cmd.Printf("Safe Verdict:   %s (%.4f)\n", verdictDisplay(artifact.Safe.Comparison.Report.Verdict), artifact.Safe.Comparison.Report.SimilarityScore)
	cmd.Printf("Unsafe Verdict: %s (%.4f)\n", verdictDisplay(artifact.Unsafe.Comparison.Report.Verdict), artifact.Unsafe.Comparison.Report.SimilarityScore)
	if artifact.Unsafe.FirstDivergence != "" {
		cmd.Printf("First Divergence: %s\n", artifact.Unsafe.FirstDivergence)
	}
	cmd.Printf("\nJudge Highlight:\n")
	cmd.Printf("  %s\n", artifact.JudgeHighlight)
	cmd.Printf("\nFiles:\n")
	cmd.Printf("  report.md          %s\n", output.Paths.Report)
	cmd.Printf("  judge-highlight.md %s\n", output.Paths.Highlight)
	cmd.Printf("  demo-script.md     %s\n", output.Paths.Script)
	cmd.Printf("  report.json        %s\n", output.Paths.JSON)
	cmd.Printf("  run-summary.json   %s\n", output.Paths.Summary)
	cmd.Printf("  run.log            %s\n", output.Paths.RunLog)
}
