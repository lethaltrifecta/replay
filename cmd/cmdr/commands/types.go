package commands

import (
	"time"
)

type migrationDemoLogPaths struct {
	RunLog        string `json:"run_log"`
	SafeVerdict   string `json:"safe_verdict"`
	UnsafeVerdict string `json:"unsafe_verdict"`
}

type migrationDemoRunSummary struct {
	BaselineTraceID     string                `json:"baseline_trace_id"`
	SafeReplayTraceID   string                `json:"safe_replay_trace_id"`
	UnsafeReplayTraceID string                `json:"unsafe_replay_trace_id"`
	ReportDir           string                `json:"report_dir"`
	Logs                migrationDemoLogPaths `json:"logs"`
}

type migrationDemoVerdictInfo struct {
	Label           string                 `json:"label"`
	TraceID         string                 `json:"trace_id"`
	FirstDivergence string                 `json:"first_divergence,omitempty"`
	Comparison      *traceComparisonReport `json:"comparison"`
}

type migrationDemoReportArtifact struct {
	GeneratedAt    time.Time                 `json:"generated_at"`
	ReportDir      string                    `json:"report_dir"`
	Scenario       string                    `json:"scenario"`
	JudgeHighlight string                    `json:"judge_highlight"`
	Summary        *migrationDemoRunSummary  `json:"summary"`
	Safe           *migrationDemoVerdictInfo `json:"safe"`
	Unsafe         *migrationDemoVerdictInfo `json:"unsafe"`
}

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
