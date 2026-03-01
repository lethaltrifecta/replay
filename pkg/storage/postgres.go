package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// PostgresStorage implements Storage interface using PostgreSQL
type PostgresStorage struct {
	db *sql.DB
}

// NewPostgresStorage creates a new PostgreSQL storage instance
func NewPostgresStorage(connectionURL string, maxConns int) (*PostgresStorage, error) {
	db, err := sql.Open("postgres", connectionURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns / 4)
	db.SetConnMaxLifetime(time.Hour)

	// Ping to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresStorage{db: db}, nil
}

// Close closes the database connection
func (s *PostgresStorage) Close() error {
	return s.db.Close()
}

// Ping verifies the database connection
func (s *PostgresStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Migrate runs database migrations
func (s *PostgresStorage) Migrate(ctx context.Context) error {
	// Read migration file
	migrationSQL, err := migrationsFS.ReadFile("migrations/001_initial_schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	// Execute migration
	_, err = s.db.ExecContext(ctx, string(migrationSQL))
	if err != nil {
		return fmt.Errorf("failed to execute migration 001: %w", err)
	}

	// Read and execute baselines + drift migration
	migrationSQL2, err := migrationsFS.ReadFile("migrations/002_baselines_and_drift.sql")
	if err != nil {
		return fmt.Errorf("failed to read migration file 002: %w", err)
	}

	_, err = s.db.ExecContext(ctx, string(migrationSQL2))
	if err != nil {
		return fmt.Errorf("failed to execute migration 002: %w", err)
	}

	return nil
}

// CreateOTELTrace creates a new OTEL trace record
func (s *PostgresStorage) CreateOTELTrace(ctx context.Context, trace *OTELTrace) error {
	query := `
		INSERT INTO otel_traces (
			trace_id, span_id, parent_span_id, service_name, span_kind,
			start_time, end_time, attributes, events, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (trace_id, span_id) DO NOTHING
		RETURNING id
	`

	err := s.db.QueryRowContext(ctx, query,
		trace.TraceID, trace.SpanID, trace.ParentSpanID, trace.ServiceName, trace.SpanKind,
		trace.StartTime, trace.EndTime, trace.Attributes, trace.Events, trace.Status, trace.CreatedAt,
	).Scan(&trace.ID)

	// ON CONFLICT DO NOTHING returns no rows — treat as success
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

// GetOTELTraceSpans retrieves all spans for a trace ID
func (s *PostgresStorage) GetOTELTraceSpans(ctx context.Context, traceID string) ([]*OTELTrace, error) {
	query := `
		SELECT id, trace_id, span_id, parent_span_id, service_name, span_kind,
		       start_time, end_time, attributes, events, status, created_at
		FROM otel_traces
		WHERE trace_id = $1
		ORDER BY start_time ASC
	`

	rows, err := s.db.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traces []*OTELTrace
	for rows.Next() {
		var trace OTELTrace
		err := rows.Scan(
			&trace.ID, &trace.TraceID, &trace.SpanID, &trace.ParentSpanID, &trace.ServiceName, &trace.SpanKind,
			&trace.StartTime, &trace.EndTime, &trace.Attributes, &trace.Events, &trace.Status, &trace.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		traces = append(traces, &trace)
	}

	return traces, rows.Err()
}

// CreateReplayTrace creates a new replay trace record
func (s *PostgresStorage) CreateReplayTrace(ctx context.Context, trace *ReplayTrace) error {
	query := `
		INSERT INTO replay_traces (
			trace_id, span_id, run_id, step_index, created_at, provider, model, prompt, completion,
			parameters, prompt_tokens, completion_tokens, total_tokens, latency_ms, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (trace_id, span_id) DO NOTHING
		RETURNING id
	`

	err := s.db.QueryRowContext(ctx, query,
		trace.TraceID, trace.SpanID, trace.RunID, trace.StepIndex, trace.CreatedAt,
		trace.Provider, trace.Model, trace.Prompt,
		trace.Completion, trace.Parameters, trace.PromptTokens, trace.CompletionTokens,
		trace.TotalTokens, trace.LatencyMS, trace.Metadata,
	).Scan(&trace.ID)

	// ON CONFLICT DO NOTHING returns no rows — treat as success
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

// GetReplayTraceSpans retrieves all LLM calls (spans) for a trace ID,
// ordered by step_index to reconstruct the agent conversation flow.
func (s *PostgresStorage) GetReplayTraceSpans(ctx context.Context, traceID string) ([]*ReplayTrace, error) {
	query := `
		SELECT id, trace_id, span_id, run_id, step_index, created_at, provider, model, prompt, completion,
		       parameters, prompt_tokens, completion_tokens, total_tokens, latency_ms, metadata
		FROM replay_traces
		WHERE trace_id = $1
		ORDER BY step_index ASC, created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traces []*ReplayTrace
	for rows.Next() {
		var trace ReplayTrace
		err := rows.Scan(
			&trace.ID, &trace.TraceID, &trace.SpanID, &trace.RunID, &trace.StepIndex, &trace.CreatedAt,
			&trace.Provider, &trace.Model, &trace.Prompt,
			&trace.Completion, &trace.Parameters, &trace.PromptTokens, &trace.CompletionTokens,
			&trace.TotalTokens, &trace.LatencyMS, &trace.Metadata,
		)
		if err != nil {
			return nil, err
		}
		traces = append(traces, &trace)
	}

	return traces, rows.Err()
}

// ListReplayTraces lists replay traces with filters
func (s *PostgresStorage) ListReplayTraces(ctx context.Context, filters TraceFilters) ([]*ReplayTrace, error) {
	query := `
		SELECT id, trace_id, span_id, run_id, step_index, created_at, provider, model, prompt, completion,
		       parameters, prompt_tokens, completion_tokens, total_tokens, latency_ms, metadata
		FROM replay_traces
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filters.Model != nil {
		query += fmt.Sprintf(" AND model = $%d", argNum)
		args = append(args, *filters.Model)
		argNum++
	}

	if filters.Provider != nil {
		query += fmt.Sprintf(" AND provider = $%d", argNum)
		args = append(args, *filters.Provider)
		argNum++
	}

	if filters.StartTime != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, *filters.StartTime)
		argNum++
	}

	if filters.EndTime != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, *filters.EndTime)
		argNum++
	}

	query += " ORDER BY created_at DESC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, filters.Limit)
		argNum++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, filters.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traces []*ReplayTrace
	for rows.Next() {
		var trace ReplayTrace
		err := rows.Scan(
			&trace.ID, &trace.TraceID, &trace.SpanID, &trace.RunID, &trace.StepIndex, &trace.CreatedAt,
			&trace.Provider, &trace.Model, &trace.Prompt,
			&trace.Completion, &trace.Parameters, &trace.PromptTokens, &trace.CompletionTokens,
			&trace.TotalTokens, &trace.LatencyMS, &trace.Metadata,
		)
		if err != nil {
			return nil, err
		}
		traces = append(traces, &trace)
	}

	return traces, rows.Err()
}

// CreateToolCapture creates a new tool capture record
func (s *PostgresStorage) CreateToolCapture(ctx context.Context, capture *ToolCapture) error {
	query := `
		INSERT INTO tool_captures (
			trace_id, span_id, step_index, tool_name, args, args_hash, result, error,
			latency_ms, risk_class, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`

	return s.db.QueryRowContext(ctx, query,
		capture.TraceID, capture.SpanID, capture.StepIndex, capture.ToolName, capture.Args, capture.ArgsHash,
		capture.Result, capture.Error, capture.LatencyMS, capture.RiskClass, capture.CreatedAt,
	).Scan(&capture.ID)
}

// GetToolCapturesByTrace retrieves all tool captures for a trace
func (s *PostgresStorage) GetToolCapturesByTrace(ctx context.Context, traceID string) ([]*ToolCapture, error) {
	query := `
		SELECT id, trace_id, span_id, step_index, tool_name, args, args_hash, result, error,
		       latency_ms, risk_class, created_at
		FROM tool_captures
		WHERE trace_id = $1
		ORDER BY step_index ASC, created_at ASC, id ASC
	`

	rows, err := s.db.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var captures []*ToolCapture
	for rows.Next() {
		var capture ToolCapture
		err := rows.Scan(
			&capture.ID, &capture.TraceID, &capture.SpanID, &capture.StepIndex, &capture.ToolName, &capture.Args,
			&capture.ArgsHash, &capture.Result, &capture.Error, &capture.LatencyMS,
			&capture.RiskClass, &capture.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		captures = append(captures, &capture)
	}

	return captures, rows.Err()
}

// GetToolCaptureByArgs retrieves a tool capture by tool name and args hash.
// Used by Freeze-Tools in freeze mode to look up pre-recorded results.
func (s *PostgresStorage) GetToolCaptureByArgs(ctx context.Context, toolName string, argsHash string) (*ToolCapture, error) {
	query := `
		SELECT id, trace_id, span_id, step_index, tool_name, args, args_hash, result, error,
		       latency_ms, risk_class, created_at
		FROM tool_captures
		WHERE tool_name = $1 AND args_hash = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	var capture ToolCapture
	err := s.db.QueryRowContext(ctx, query, toolName, argsHash).Scan(
		&capture.ID, &capture.TraceID, &capture.SpanID, &capture.StepIndex, &capture.ToolName, &capture.Args,
		&capture.ArgsHash, &capture.Result, &capture.Error, &capture.LatencyMS,
		&capture.RiskClass, &capture.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tool capture not found: %s/%s", toolName, argsHash)
	}
	if err != nil {
		return nil, err
	}

	return &capture, nil
}

// CreateExperiment creates a new experiment
func (s *PostgresStorage) CreateExperiment(ctx context.Context, exp *Experiment) error {
	query := `
		INSERT INTO experiments (
			id, name, baseline_trace_id, status, progress, config, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := s.db.ExecContext(ctx, query,
		exp.ID, exp.Name, exp.BaselineTraceID, exp.Status, exp.Progress, exp.Config, exp.CreatedAt,
	)

	return err
}

// GetExperiment retrieves an experiment by ID
func (s *PostgresStorage) GetExperiment(ctx context.Context, id uuid.UUID) (*Experiment, error) {
	query := `
		SELECT id, name, baseline_trace_id, status, progress, config, created_at, completed_at
		FROM experiments
		WHERE id = $1
	`

	var exp Experiment
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&exp.ID, &exp.Name, &exp.BaselineTraceID, &exp.Status, &exp.Progress,
		&exp.Config, &exp.CreatedAt, &exp.CompletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("experiment not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	return &exp, nil
}

// UpdateExperiment updates an experiment
func (s *PostgresStorage) UpdateExperiment(ctx context.Context, exp *Experiment) error {
	query := `
		UPDATE experiments
		SET status = $1, progress = $2, completed_at = $3
		WHERE id = $4
	`

	_, err := s.db.ExecContext(ctx, query, exp.Status, exp.Progress, exp.CompletedAt, exp.ID)
	return err
}

// ListExperiments lists experiments with filters
func (s *PostgresStorage) ListExperiments(ctx context.Context, filters ExperimentFilters) ([]*Experiment, error) {
	query := `
		SELECT id, name, baseline_trace_id, status, progress, config, created_at, completed_at
		FROM experiments
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filters.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, *filters.Status)
		argNum++
	}

	if filters.StartTime != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, *filters.StartTime)
		argNum++
	}

	if filters.EndTime != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, *filters.EndTime)
		argNum++
	}

	query += " ORDER BY created_at DESC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, filters.Limit)
		argNum++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, filters.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var experiments []*Experiment
	for rows.Next() {
		var exp Experiment
		err := rows.Scan(
			&exp.ID, &exp.Name, &exp.BaselineTraceID, &exp.Status, &exp.Progress,
			&exp.Config, &exp.CreatedAt, &exp.CompletedAt,
		)
		if err != nil {
			return nil, err
		}
		experiments = append(experiments, &exp)
	}

	return experiments, rows.Err()
}

// CreateExperimentRun creates a new experiment run
func (s *PostgresStorage) CreateExperimentRun(ctx context.Context, run *ExperimentRun) error {
	query := `
		INSERT INTO experiment_runs (
			id, experiment_id, run_type, variant_config, trace_id, status, error, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := s.db.ExecContext(ctx, query,
		run.ID, run.ExperimentID, run.RunType, run.VariantConfig, run.TraceID,
		run.Status, run.Error, run.CreatedAt,
	)

	return err
}

// GetExperimentRun retrieves an experiment run by ID
func (s *PostgresStorage) GetExperimentRun(ctx context.Context, id uuid.UUID) (*ExperimentRun, error) {
	query := `
		SELECT id, experiment_id, run_type, variant_config, trace_id, status, error, created_at, completed_at
		FROM experiment_runs
		WHERE id = $1
	`

	var run ExperimentRun
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&run.ID, &run.ExperimentID, &run.RunType, &run.VariantConfig, &run.TraceID,
		&run.Status, &run.Error, &run.CreatedAt, &run.CompletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("experiment run not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	return &run, nil
}

// UpdateExperimentRun updates an experiment run
func (s *PostgresStorage) UpdateExperimentRun(ctx context.Context, run *ExperimentRun) error {
	query := `
		UPDATE experiment_runs
		SET trace_id = $1, status = $2, error = $3, completed_at = $4
		WHERE id = $5
	`

	_, err := s.db.ExecContext(ctx, query, run.TraceID, run.Status, run.Error, run.CompletedAt, run.ID)
	return err
}

// ListExperimentRuns lists all runs for an experiment
func (s *PostgresStorage) ListExperimentRuns(ctx context.Context, experimentID uuid.UUID) ([]*ExperimentRun, error) {
	query := `
		SELECT id, experiment_id, run_type, variant_config, trace_id, status, error, created_at, completed_at
		FROM experiment_runs
		WHERE experiment_id = $1
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*ExperimentRun
	for rows.Next() {
		var run ExperimentRun
		err := rows.Scan(
			&run.ID, &run.ExperimentID, &run.RunType, &run.VariantConfig, &run.TraceID,
			&run.Status, &run.Error, &run.CreatedAt, &run.CompletedAt,
		)
		if err != nil {
			return nil, err
		}
		runs = append(runs, &run)
	}

	return runs, rows.Err()
}

// CreateAnalysisResult creates a new analysis result
func (s *PostgresStorage) CreateAnalysisResult(ctx context.Context, result *AnalysisResult) error {
	query := `
		INSERT INTO analysis_results (
			experiment_id, baseline_run_id, candidate_run_id, behavior_diff, first_divergence,
			safety_diff, similarity_score, quality_metrics, token_delta, cost_delta, latency_delta, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`

	return s.db.QueryRowContext(ctx, query,
		result.ExperimentID, result.BaselineRunID, result.CandidateRunID, result.BehaviorDiff,
		result.FirstDivergence, result.SafetyDiff, result.SimilarityScore, result.QualityMetrics,
		result.TokenDelta, result.CostDelta, result.LatencyDelta, result.CreatedAt,
	).Scan(&result.ID)
}

// GetAnalysisResults retrieves all analysis results for an experiment
func (s *PostgresStorage) GetAnalysisResults(ctx context.Context, experimentID uuid.UUID) ([]*AnalysisResult, error) {
	query := `
		SELECT id, experiment_id, baseline_run_id, candidate_run_id, behavior_diff, first_divergence,
		       safety_diff, similarity_score, quality_metrics, token_delta, cost_delta, latency_delta, created_at
		FROM analysis_results
		WHERE experiment_id = $1
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*AnalysisResult
	for rows.Next() {
		var result AnalysisResult
		err := rows.Scan(
			&result.ID, &result.ExperimentID, &result.BaselineRunID, &result.CandidateRunID,
			&result.BehaviorDiff, &result.FirstDivergence, &result.SafetyDiff, &result.SimilarityScore,
			&result.QualityMetrics, &result.TokenDelta, &result.CostDelta, &result.LatencyDelta, &result.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, &result)
	}

	return results, rows.Err()
}

// Evaluator methods (implementation continues...)
// I'll add the remaining methods in the next file segment

func (s *PostgresStorage) CreateEvaluator(ctx context.Context, evaluator *Evaluator) error {
	query := `
		INSERT INTO evaluators (name, type, config, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	return s.db.QueryRowContext(ctx, query,
		evaluator.Name, evaluator.Type, evaluator.Config, evaluator.Enabled,
		evaluator.CreatedAt, evaluator.UpdatedAt,
	).Scan(&evaluator.ID)
}

func (s *PostgresStorage) GetEvaluator(ctx context.Context, id int) (*Evaluator, error) {
	query := `
		SELECT id, name, type, config, enabled, created_at, updated_at
		FROM evaluators
		WHERE id = $1
	`

	var eval Evaluator
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&eval.ID, &eval.Name, &eval.Type, &eval.Config, &eval.Enabled,
		&eval.CreatedAt, &eval.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("evaluator not found: %d", id)
	}
	if err != nil {
		return nil, err
	}

	return &eval, nil
}

func (s *PostgresStorage) GetEvaluatorByName(ctx context.Context, name string) (*Evaluator, error) {
	query := `
		SELECT id, name, type, config, enabled, created_at, updated_at
		FROM evaluators
		WHERE name = $1
	`

	var eval Evaluator
	err := s.db.QueryRowContext(ctx, query, name).Scan(
		&eval.ID, &eval.Name, &eval.Type, &eval.Config, &eval.Enabled,
		&eval.CreatedAt, &eval.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("evaluator not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	return &eval, nil
}

func (s *PostgresStorage) UpdateEvaluator(ctx context.Context, evaluator *Evaluator) error {
	query := `
		UPDATE evaluators
		SET config = $1, enabled = $2, updated_at = $3
		WHERE id = $4
	`

	_, err := s.db.ExecContext(ctx, query,
		evaluator.Config, evaluator.Enabled, time.Now(), evaluator.ID,
	)
	return err
}

func (s *PostgresStorage) ListEvaluators(ctx context.Context, enabledOnly bool) ([]*Evaluator, error) {
	query := `
		SELECT id, name, type, config, enabled, created_at, updated_at
		FROM evaluators
	`

	if enabledOnly {
		query += " WHERE enabled = true"
	}

	query += " ORDER BY created_at ASC"

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evaluators []*Evaluator
	for rows.Next() {
		var eval Evaluator
		err := rows.Scan(
			&eval.ID, &eval.Name, &eval.Type, &eval.Config, &eval.Enabled,
			&eval.CreatedAt, &eval.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		evaluators = append(evaluators, &eval)
	}

	return evaluators, rows.Err()
}

func (s *PostgresStorage) DeleteEvaluator(ctx context.Context, id int) error {
	query := `DELETE FROM evaluators WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

// Continue with remaining methods in next segment...
