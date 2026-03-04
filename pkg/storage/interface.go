package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage defines the interface for all storage operations
type Storage interface {
	// Lifecycle
	Close() error
	Ping(ctx context.Context) error
	Migrate(ctx context.Context) error

	// Traces
	CreateOTELTrace(ctx context.Context, trace *OTELTrace) error
	GetOTELTraceSpans(ctx context.Context, traceID string) ([]*OTELTrace, error)
	CreateReplayTrace(ctx context.Context, trace *ReplayTrace) error
	GetReplayTraceSpans(ctx context.Context, traceID string) ([]*ReplayTrace, error)
	ListReplayTraces(ctx context.Context, filters TraceFilters) ([]*ReplayTrace, error)

	// Atomic ingestion batch — all three tables in a single transaction
	CreateIngestionBatch(ctx context.Context, otels []*OTELTrace, replays []*ReplayTrace, tools []*ToolCapture) (IngestCounts, error)

	// Tool Captures (for Freeze-Tools)
	CreateToolCapture(ctx context.Context, capture *ToolCapture) error
	GetToolCapturesByTrace(ctx context.Context, traceID string) ([]*ToolCapture, error)
	GetToolCaptureByArgs(ctx context.Context, toolName string, argsHash string) (*ToolCapture, error)

	// Experiments
	CreateExperiment(ctx context.Context, exp *Experiment) error
	GetExperiment(ctx context.Context, id uuid.UUID) (*Experiment, error)
	UpdateExperiment(ctx context.Context, exp *Experiment) error
	ListExperiments(ctx context.Context, filters ExperimentFilters) ([]*Experiment, error)

	// Experiment Runs
	CreateExperimentRun(ctx context.Context, run *ExperimentRun) error
	GetExperimentRun(ctx context.Context, id uuid.UUID) (*ExperimentRun, error)
	UpdateExperimentRun(ctx context.Context, run *ExperimentRun) error
	ListExperimentRuns(ctx context.Context, experimentID uuid.UUID) ([]*ExperimentRun, error)

	// Analysis Results
	CreateAnalysisResult(ctx context.Context, result *AnalysisResult) error
	GetAnalysisResults(ctx context.Context, experimentID uuid.UUID) ([]*AnalysisResult, error)

	// Evaluators
	CreateEvaluator(ctx context.Context, evaluator *Evaluator) error
	GetEvaluator(ctx context.Context, id int) (*Evaluator, error)
	GetEvaluatorByName(ctx context.Context, name string) (*Evaluator, error)
	UpdateEvaluator(ctx context.Context, evaluator *Evaluator) error
	ListEvaluators(ctx context.Context, enabledOnly bool) ([]*Evaluator, error)
	DeleteEvaluator(ctx context.Context, id int) error

	// Evaluation Runs
	CreateEvaluationRun(ctx context.Context, run *EvaluationRun) error
	GetEvaluationRun(ctx context.Context, id uuid.UUID) (*EvaluationRun, error)
	UpdateEvaluationRun(ctx context.Context, run *EvaluationRun) error

	// Evaluator Results
	CreateEvaluatorResult(ctx context.Context, result *EvaluatorResult) error
	GetEvaluatorResults(ctx context.Context, evaluationRunID uuid.UUID) ([]*EvaluatorResult, error)

	// Human Evaluation Queue
	CreateHumanEvaluation(ctx context.Context, eval *HumanEvaluation) error
	GetHumanEvaluation(ctx context.Context, id uuid.UUID) (*HumanEvaluation, error)
	UpdateHumanEvaluation(ctx context.Context, eval *HumanEvaluation) error
	ListPendingHumanEvaluations(ctx context.Context, assignedTo *string) ([]*HumanEvaluation, error)

	// Ground Truth
	CreateGroundTruth(ctx context.Context, gt *GroundTruth) error
	GetGroundTruth(ctx context.Context, taskID string) (*GroundTruth, error)
	UpdateGroundTruth(ctx context.Context, gt *GroundTruth) error
	ListGroundTruth(ctx context.Context, taskType *string) ([]*GroundTruth, error)
	DeleteGroundTruth(ctx context.Context, taskID string) error

	// Evaluation Summary
	CreateEvaluationSummary(ctx context.Context, summary *EvaluationSummary) error
	GetEvaluationSummary(ctx context.Context, experimentID uuid.UUID) ([]*EvaluationSummary, error)

	// Baselines
	MarkTraceAsBaseline(ctx context.Context, baseline *Baseline) error
	GetBaseline(ctx context.Context, traceID string) (*Baseline, error)
	ListBaselines(ctx context.Context) ([]*Baseline, error)
	UnmarkBaseline(ctx context.Context, traceID string) error

	// Drift Results
	CreateDriftResult(ctx context.Context, result *DriftResult) error
	GetDriftResults(ctx context.Context, traceID string) ([]*DriftResult, error)
	GetDriftResultsByBaseline(ctx context.Context, baselineTraceID string, limit int) ([]*DriftResult, error)
	GetLatestDriftResult(ctx context.Context, traceID string) (*DriftResult, error)
	ListDriftResults(ctx context.Context, limit int) ([]*DriftResult, error)
	HasDriftResultForBaseline(ctx context.Context, traceID string, baselineTraceID string) (bool, error)
}

// TraceFilters for filtering trace queries
type TraceFilters struct {
	Model     *string
	Provider  *string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
	SortAsc   bool // when true, ORDER BY created_at ASC instead of DESC
}

// ExperimentFilters for filtering experiment queries
type ExperimentFilters struct {
	Status    *string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}
