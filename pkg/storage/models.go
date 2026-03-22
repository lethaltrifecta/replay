package storage

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"maps"
	"time"

	"github.com/google/uuid"
)

// --- JSONB Support Helpers ---

func jsonValue(v any) (driver.Value, error) {
	return json.Marshal(v)
}

func jsonScan(src any, target any) error {
	if src == nil {
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, target)
}

// --- Structured Types with JSONB Support ---

// PromptMessage represents a single message in a model prompt.
type PromptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PromptContent represents the structured content of an LLM prompt.
type PromptContent struct {
	Messages []PromptMessage `json:"messages"`
}

func (p PromptContent) Value() (driver.Value, error) { return jsonValue(p) }
func (p *PromptContent) Scan(src interface{}) error  { return jsonScan(src, p) }

// DriftDetails represents the structured details of behavioral drift.
type DriftDetails struct {
	Reason         string `json:"reason,omitempty"`
	DivergenceStep *int   `json:"divergence_step,omitempty"`
	RiskEscalation bool   `json:"risk_escalation,omitempty"`
}

func (d DriftDetails) Value() (driver.Value, error) { return jsonValue(d) }
func (d *DriftDetails) Scan(src interface{}) error  { return jsonScan(src, d) }

// IsZero reports whether the drift details carry any meaningful summary data.
func (d DriftDetails) IsZero() bool {
	return d.Reason == "" && d.DivergenceStep == nil && !d.RiskEscalation
}

// BehaviorDiff represents the behavior comparison in an analysis result.
type BehaviorDiff struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

func (b BehaviorDiff) Value() (driver.Value, error) { return jsonValue(b) }
func (b *BehaviorDiff) Scan(src interface{}) error  { return jsonScan(src, b) }

// FirstDivergence represents where a candidate first deviated from a baseline.
type FirstDivergence struct {
	StepIndex       *int   `json:"step_index,omitempty"`
	ToolIndex       *int   `json:"tool_index,omitempty"`
	Type            string `json:"type,omitempty"`
	Baseline        string `json:"baseline,omitempty"`
	Variant         string `json:"variant,omitempty"`
	BaselineExcerpt string `json:"baseline_excerpt,omitempty"`
	VariantExcerpt  string `json:"variant_excerpt,omitempty"`
	BaselineCount   *int   `json:"baseline_count,omitempty"`
	VariantCount    *int   `json:"variant_count,omitempty"`
	BaselineSteps   *int   `json:"baseline_steps,omitempty"`
	VariantSteps    *int   `json:"variant_steps,omitempty"`
}

func (f FirstDivergence) Value() (driver.Value, error) { return jsonValue(f) }
func (f *FirstDivergence) Scan(src interface{}) error  { return jsonScan(src, f) }

// IsZero reports whether the divergence payload is empty.
func (f FirstDivergence) IsZero() bool {
	return f.StepIndex == nil &&
		f.ToolIndex == nil &&
		f.Type == "" &&
		f.Baseline == "" &&
		f.Variant == "" &&
		f.BaselineExcerpt == "" &&
		f.VariantExcerpt == "" &&
		f.BaselineCount == nil &&
		f.VariantCount == nil &&
		f.BaselineSteps == nil &&
		f.VariantSteps == nil
}

// SafetyDiff represents the risk changes between baseline and candidate.
type SafetyDiff struct {
	RiskEscalation bool   `json:"risk_escalation,omitempty"`
	BaselineRisk   string `json:"baseline_risk,omitempty"`
	VariantRisk    string `json:"variant_risk,omitempty"`
}

func (s SafetyDiff) Value() (driver.Value, error) { return jsonValue(s) }
func (s *SafetyDiff) Scan(src interface{}) error  { return jsonScan(src, s) }

// IsZero reports whether the safety diff carries any meaningful risk delta data.
func (s SafetyDiff) IsZero() bool {
	return !s.RiskEscalation && s.BaselineRisk == "" && s.VariantRisk == ""
}

// AnalysisResultData is the structured container for experiment analysis.
type AnalysisResultData struct {
	BehaviorDiff    BehaviorDiff    `json:"behavior_diff"`
	FirstDivergence FirstDivergence `json:"first_divergence"`
	SafetyDiff      SafetyDiff      `json:"safety_diff"`
}

func (a AnalysisResultData) Value() (driver.Value, error) { return jsonValue(a) }
func (a *AnalysisResultData) Scan(src interface{}) error  { return jsonScan(src, a) }

// VariantConfig represents the configuration used for a replay run.
type VariantConfig struct {
	Model          string            `json:"model"`
	Provider       string            `json:"provider"`
	Temperature    *float64          `json:"temperature,omitempty"`
	TopP           *float64          `json:"top_p,omitempty"`
	MaxTokens      *int              `json:"max_tokens,omitempty"`
	RequestHeaders map[string]string `json:"request_headers,omitempty"`
}

func (v VariantConfig) Value() (driver.Value, error) { return jsonValue(v) }
func (v *VariantConfig) Scan(src interface{}) error  { return jsonScan(src, v) }

// IsZero reports whether the variant config contains any meaningful values.
func (v VariantConfig) IsZero() bool {
	return v.Model == "" &&
		v.Provider == "" &&
		v.Temperature == nil &&
		v.TopP == nil &&
		v.MaxTokens == nil &&
		len(v.RequestHeaders) == 0
}

// ExperimentConfig represents the persisted configuration for an experiment.
type ExperimentConfig struct {
	Model          string            `json:"model,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Temperature    *float64          `json:"temperature,omitempty"`
	TopP           *float64          `json:"top_p,omitempty"`
	MaxTokens      *int              `json:"max_tokens,omitempty"`
	RequestHeaders map[string]string `json:"request_headers,omitempty"`
	Threshold      *float64          `json:"threshold,omitempty"`
}

func (c ExperimentConfig) Value() (driver.Value, error) { return jsonValue(c) }
func (c *ExperimentConfig) Scan(src interface{}) error  { return jsonScan(src, c) }

// ToVariantConfig projects the experiment-level config into the shared variant shape.
func (c ExperimentConfig) ToVariantConfig() VariantConfig {
	cfg := VariantConfig{
		Model:       c.Model,
		Provider:    c.Provider,
		Temperature: c.Temperature,
		TopP:        c.TopP,
		MaxTokens:   c.MaxTokens,
	}
	if len(c.RequestHeaders) > 0 {
		cfg.RequestHeaders = maps.Clone(c.RequestHeaders)
	}
	return cfg
}

// JSONB is a type alias for PostgreSQL JSONB columns.
// pgx handles map[string]any natively as JSONB, so no custom
// driver.Valuer or sql.Scanner implementations are needed.
type JSONB map[string]any

// OTELTrace represents a raw OTEL trace span.
type OTELTrace struct {
	ID           int       `db:"id"`
	TraceID      string    `db:"trace_id"`
	SpanID       string    `db:"span_id"`
	ParentSpanID *string   `db:"parent_span_id"`
	ServiceName  string    `db:"service_name"`
	SpanKind     string    `db:"span_kind"`
	StartTime    time.Time `db:"start_time"`
	EndTime      time.Time `db:"end_time"`
	Attributes   JSONB     `db:"attributes"`
	Events       JSONB     `db:"events"`
	Status       JSONB     `db:"status"`
	CreatedAt    time.Time `db:"created_at"`
}

// ReplayTrace represents a parsed LLM call in replay-specific schema.
type ReplayTrace struct {
	ID               int       `db:"id"`
	TraceID          string    `db:"trace_id"`
	SpanID           string    `db:"span_id"`
	RunID            string    `db:"run_id"`
	StepIndex        int       `db:"step_index"`
	CreatedAt        time.Time `db:"created_at"`
	Provider         string    `db:"provider"`
	Model            string    `db:"model"`
	Prompt           JSONB     `db:"prompt"` // Still use JSONB here for flexible raw prompts
	Completion       string    `db:"completion"`
	Parameters       JSONB     `db:"parameters"`
	PromptTokens     int       `db:"prompt_tokens"`
	CompletionTokens int       `db:"completion_tokens"`
	TotalTokens      int       `db:"total_tokens"`
	LatencyMS        int       `db:"latency_ms"`
	Metadata         JSONB     `db:"metadata"`
}

// TraceSummary provides aggregated info about a multi-step trace.
type TraceSummary struct {
	TraceID   string    `db:"trace_id"`
	Models    []string  `db:"models"`
	Providers []string  `db:"providers"`
	StepCount int       `db:"step_count"`
	CreatedAt time.Time `db:"created_at"`
}

// ToolCapture represents a captured tool call for Freeze-Tools
type ToolCapture struct {
	ID        int       `db:"id"`
	TraceID   string    `db:"trace_id"`
	SpanID    string    `db:"span_id"`
	StepIndex int       `db:"step_index"`
	ToolName  string    `db:"tool_name"`
	Args      JSONB     `db:"args"`
	ArgsHash  string    `db:"args_hash"`
	Result    JSONB     `db:"result"`
	Error     *string   `db:"error"`
	LatencyMS int       `db:"latency_ms"`
	RiskClass string    `db:"risk_class"`
	CreatedAt time.Time `db:"created_at"`
}

// Experiment represents a replay experiment
type Experiment struct {
	ID              uuid.UUID        `db:"id"`
	Name            string           `db:"name"`
	BaselineTraceID string           `db:"baseline_trace_id"`
	Status          string           `db:"status"`
	Progress        float64          `db:"progress"`
	Config          ExperimentConfig `db:"config"`
	CreatedAt       time.Time        `db:"created_at"`
	CompletedAt     *time.Time       `db:"completed_at"`
}

// ExperimentRun represents a single run within an experiment
type ExperimentRun struct {
	ID            uuid.UUID     `db:"id"`
	ExperimentID  uuid.UUID     `db:"experiment_id"`
	RunType       string        `db:"run_type"`
	VariantConfig VariantConfig `db:"variant_config"`
	TraceID       *string       `db:"trace_id"`
	Status        string        `db:"status"`
	Error         *string       `db:"error"`
	CreatedAt     time.Time     `db:"created_at"`
	CompletedAt   *time.Time    `db:"completed_at"`
}

// AnalysisResult represents comparative analysis results
type AnalysisResult struct {
	ID              int             `db:"id"`
	ExperimentID    uuid.UUID       `db:"experiment_id"`
	BaselineRunID   uuid.UUID       `db:"baseline_run_id"`
	CandidateRunID  uuid.UUID       `db:"candidate_run_id"`
	BehaviorDiff    BehaviorDiff    `db:"behavior_diff"`
	FirstDivergence FirstDivergence `db:"first_divergence"`
	SafetyDiff      SafetyDiff      `db:"safety_diff"`
	SimilarityScore float64         `db:"similarity_score"`
	QualityMetrics  JSONB           `db:"quality_metrics"`
	TokenDelta      int             `db:"token_delta"`
	CostDelta       float64         `db:"cost_delta"`
	LatencyDelta    int             `db:"latency_delta"`
	CreatedAt       time.Time       `db:"created_at"`
}

// Evaluator represents an evaluator configuration
type Evaluator struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Type      string    `db:"type"`
	Config    JSONB     `db:"config"`
	Enabled   bool      `db:"enabled"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// EvaluationRun represents an evaluation execution
type EvaluationRun struct {
	ID              uuid.UUID  `db:"id"`
	ExperimentRunID uuid.UUID  `db:"experiment_run_id"`
	Status          string     `db:"status"`
	StartedAt       *time.Time `db:"started_at"`
	CompletedAt     *time.Time `db:"completed_at"`
}

// EvaluatorResult represents the result from a single evaluator
type EvaluatorResult struct {
	ID              int       `db:"id"`
	EvaluationRunID uuid.UUID `db:"evaluation_run_id"`
	EvaluatorID     int       `db:"evaluator_id"`
	Scores          JSONB     `db:"scores"`
	OverallScore    float64   `db:"overall_score"`
	Passed          bool      `db:"passed"`
	Reasoning       string    `db:"reasoning"`
	Metadata        JSONB     `db:"metadata"`
	EvaluatedAt     time.Time `db:"evaluated_at"`
}

// HumanEvaluation represents a human review queue item
type HumanEvaluation struct {
	ID              uuid.UUID  `db:"id"`
	EvaluationRunID uuid.UUID  `db:"evaluation_run_id"`
	ExperimentRunID uuid.UUID  `db:"experiment_run_id"`
	Output          string     `db:"output"`
	Context         JSONB      `db:"context"`
	Status          string     `db:"status"`
	AssignedTo      *string    `db:"assigned_to"`
	Scores          JSONB      `db:"scores"`
	Feedback        *string    `db:"feedback"`
	CreatedAt       time.Time  `db:"created_at"`
	AssignedAt      *time.Time `db:"assigned_at"`
	ReviewedAt      *time.Time `db:"reviewed_at"`
}

// GroundTruth represents a reference answer for evaluation
type GroundTruth struct {
	ID             int       `db:"id"`
	TaskID         string    `db:"task_id"`
	TaskType       string    `db:"task_type"`
	Input          JSONB     `db:"input"`
	ExpectedOutput string    `db:"expected_output"`
	Metadata       JSONB     `db:"metadata"`
	CreatedAt      time.Time `db:"created_at"`
}

// EvaluationSummary represents aggregated evaluation results
type EvaluationSummary struct {
	ID              int       `db:"id"`
	ExperimentID    uuid.UUID `db:"experiment_id"`
	ExperimentRunID uuid.UUID `db:"experiment_run_id"`
	OverallScore    float64   `db:"overall_score"`
	Passed          bool      `db:"passed"`
	EvaluatorScores JSONB     `db:"evaluator_scores"`
	Rank            int       `db:"rank"`
	IsBest          bool      `db:"is_best"`
	CreatedAt       time.Time `db:"created_at"`
}

// Baseline represents a trace marked as known-good for drift comparison
type Baseline struct {
	ID          int       `db:"id"`
	TraceID     string    `db:"trace_id"`
	Name        *string   `db:"name"`
	Description *string   `db:"description"`
	CreatedAt   time.Time `db:"created_at"`
}

// DriftResult represents the outcome of comparing a trace against a baseline
type DriftResult struct {
	ID              int          `db:"id"`
	TraceID         string       `db:"trace_id"`
	BaselineTraceID string       `db:"baseline_trace_id"`
	DriftScore      float64      `db:"drift_score"`
	Verdict         string       `db:"verdict"`
	Details         DriftDetails `db:"details"`
	CreatedAt       time.Time    `db:"created_at"`
}

// Drift verdict constants
const (
	DriftVerdictPass    = "pass"
	DriftVerdictWarn    = "warn"
	DriftVerdictFail    = "fail"
	DriftVerdictPending = "pending"
)

// IngestCounts holds the number of rows inserted by CreateIngestionBatch.
type IngestCounts struct {
	OTELTraces   int64
	ReplayTraces int64
	ToolCaptures int64
}

// Status constants
const (
	// Experiment/Run status
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	// Run types
	RunTypeBaseline = "baseline"
	RunTypeVariant  = "variant"

	// Risk classes
	RiskClassRead        = "read"
	RiskClassWrite       = "write"
	RiskClassDestructive = "destructive"

	// Human evaluation status
	HumanEvalStatusPending   = "pending"
	HumanEvalStatusInReview  = "in_review"
	HumanEvalStatusCompleted = "completed"
	HumanEvalStatusSkipped   = "skipped"

	// Evaluator types
	EvaluatorTypeRule        = "rule"
	EvaluatorTypeLLMJudge    = "llm_judge"
	EvaluatorTypeRubric      = "rubric"
	EvaluatorTypeHuman       = "human"
	EvaluatorTypeGroundTruth = "ground_truth"
)
