package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// Baseline methods

// MarkTraceAsBaseline marks a trace as a known-good baseline.
// Validates the trace exists in replay_traces first.
// If the trace is already a baseline, updates name/description.
func (s *PostgresStorage) MarkTraceAsBaseline(ctx context.Context, baseline *Baseline) error {
	// Validate trace exists
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM replay_traces WHERE trace_id = $1)`,
		baseline.TraceID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check trace existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("trace not found: %s", baseline.TraceID)
	}

	query := `
		INSERT INTO baselines (trace_id, name, description)
		VALUES ($1, $2, $3)
		ON CONFLICT (trace_id) DO UPDATE SET
			name = COALESCE(EXCLUDED.name, baselines.name),
			description = COALESCE(EXCLUDED.description, baselines.description)
		RETURNING id, created_at
	`

	return s.db.QueryRowContext(ctx, query,
		baseline.TraceID, baseline.Name, baseline.Description,
	).Scan(&baseline.ID, &baseline.CreatedAt)
}

// GetBaseline retrieves a baseline by trace ID.
func (s *PostgresStorage) GetBaseline(ctx context.Context, traceID string) (*Baseline, error) {
	query := `
		SELECT id, trace_id, name, description, created_at
		FROM baselines
		WHERE trace_id = $1
	`

	var b Baseline
	err := s.db.QueryRowContext(ctx, query, traceID).Scan(
		&b.ID, &b.TraceID, &b.Name, &b.Description, &b.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("baseline not found: %s", traceID)
	}
	if err != nil {
		return nil, err
	}

	return &b, nil
}

// ListBaselines returns all baselines ordered by creation time (newest first).
func (s *PostgresStorage) ListBaselines(ctx context.Context) ([]*Baseline, error) {
	query := `
		SELECT id, trace_id, name, description, created_at
		FROM baselines
		ORDER BY created_at DESC, id DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var baselines []*Baseline
	for rows.Next() {
		var b Baseline
		err := rows.Scan(&b.ID, &b.TraceID, &b.Name, &b.Description, &b.CreatedAt)
		if err != nil {
			return nil, err
		}
		baselines = append(baselines, &b)
	}

	return baselines, rows.Err()
}

// UnmarkBaseline removes a baseline by trace ID.
// Returns an error if no baseline exists for the given trace.
func (s *PostgresStorage) UnmarkBaseline(ctx context.Context, traceID string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM baselines WHERE trace_id = $1`, traceID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("baseline not found: %s", traceID)
	}

	return nil
}

// Drift Result methods

// CreateDriftResult stores a drift comparison result.
// Defaults verdict to "pending" if empty and validates drift_score is in [0, 1].
func (s *PostgresStorage) CreateDriftResult(ctx context.Context, result *DriftResult) error {
	if result.Verdict == "" {
		result.Verdict = DriftVerdictPending
	}
	if result.DriftScore < 0 || result.DriftScore > 1 {
		return fmt.Errorf("drift_score must be between 0 and 1, got %f", result.DriftScore)
	}

	query := `
		INSERT INTO drift_results (trace_id, baseline_trace_id, drift_score, verdict, details)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`

	return s.db.QueryRowContext(ctx, query,
		result.TraceID, result.BaselineTraceID, result.DriftScore, result.Verdict, result.Details,
	).Scan(&result.ID, &result.CreatedAt)
}

// GetDriftResults retrieves all drift results for a trace ID (newest first).
func (s *PostgresStorage) GetDriftResults(ctx context.Context, traceID string) ([]*DriftResult, error) {
	query := `
		SELECT id, trace_id, baseline_trace_id, drift_score, verdict, details, created_at
		FROM drift_results
		WHERE trace_id = $1
		ORDER BY created_at DESC, id DESC
	`

	rows, err := s.db.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*DriftResult
	for rows.Next() {
		var r DriftResult
		err := rows.Scan(
			&r.ID, &r.TraceID, &r.BaselineTraceID, &r.DriftScore, &r.Verdict, &r.Details, &r.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, &r)
	}

	return results, rows.Err()
}

// GetDriftResultsByBaseline retrieves all drift results for a baseline trace ID (newest first).
func (s *PostgresStorage) GetDriftResultsByBaseline(ctx context.Context, baselineTraceID string) ([]*DriftResult, error) {
	query := `
		SELECT id, trace_id, baseline_trace_id, drift_score, verdict, details, created_at
		FROM drift_results
		WHERE baseline_trace_id = $1
		ORDER BY created_at DESC, id DESC
	`

	rows, err := s.db.QueryContext(ctx, query, baselineTraceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*DriftResult
	for rows.Next() {
		var r DriftResult
		err := rows.Scan(
			&r.ID, &r.TraceID, &r.BaselineTraceID, &r.DriftScore, &r.Verdict, &r.Details, &r.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, &r)
	}

	return results, rows.Err()
}

// ListDriftResults returns the most recent drift results across all traces.
func (s *PostgresStorage) ListDriftResults(ctx context.Context, limit int) ([]*DriftResult, error) {
	query := `
		SELECT id, trace_id, baseline_trace_id, drift_score, verdict, details, created_at
		FROM drift_results
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*DriftResult
	for rows.Next() {
		var r DriftResult
		err := rows.Scan(
			&r.ID, &r.TraceID, &r.BaselineTraceID, &r.DriftScore, &r.Verdict, &r.Details, &r.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, &r)
	}

	return results, rows.Err()
}

// GetLatestDriftResult retrieves the most recent drift result for a trace ID.
func (s *PostgresStorage) GetLatestDriftResult(ctx context.Context, traceID string) (*DriftResult, error) {
	query := `
		SELECT id, trace_id, baseline_trace_id, drift_score, verdict, details, created_at
		FROM drift_results
		WHERE trace_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`

	var r DriftResult
	err := s.db.QueryRowContext(ctx, query, traceID).Scan(
		&r.ID, &r.TraceID, &r.BaselineTraceID, &r.DriftScore, &r.Verdict, &r.Details, &r.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no drift results found for trace: %s", traceID)
	}
	if err != nil {
		return nil, err
	}

	return &r, nil
}

// HasDriftResultForBaseline checks whether a drift result exists for a specific
// (trace_id, baseline_trace_id) pair. Returns (false, nil) when no result exists,
// distinguishing "not found" from actual DB errors.
func (s *PostgresStorage) HasDriftResultForBaseline(ctx context.Context, traceID string, baselineTraceID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM drift_results WHERE trace_id = $1 AND baseline_trace_id = $2)`,
		traceID, baselineTraceID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check drift result existence: %w", err)
	}
	return exists, nil
}
