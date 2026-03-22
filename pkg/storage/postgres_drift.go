package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Baseline methods

// MarkTraceAsBaseline marks a trace as a known-good baseline.
func (s *PostgresStorage) MarkTraceAsBaseline(ctx context.Context, b *Baseline) error {
	// Validate trace exists
	var exists bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM replay_traces WHERE trace_id = $1)", b.TraceID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrTraceNotFound
	}

	query := `
		INSERT INTO baselines (trace_id, name, description, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (trace_id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			created_at = NOW()
	`
	_, err = s.pool.Exec(ctx, query, b.TraceID, b.Name, b.Description)
	return err
}

// GetBaseline retrieves a baseline by trace ID.
func (s *PostgresStorage) GetBaseline(ctx context.Context, traceID string) (*Baseline, error) {
	query := `
		SELECT id, trace_id, name, description, created_at
		FROM baselines
		WHERE trace_id = $1
	`

	var b Baseline
	err := s.pool.QueryRow(ctx, query, traceID).Scan(
		&b.ID, &b.TraceID, &b.Name, &b.Description, &b.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBaselineNotFound
	}
	if err != nil {
		return nil, err
	}

	return &b, nil
}

// ListBaselines returns all approved baselines.
func (s *PostgresStorage) ListBaselines(ctx context.Context) ([]*Baseline, error) {
	query := `
		SELECT id, trace_id, name, description, created_at
		FROM baselines
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Baseline
	for rows.Next() {
		var b Baseline
		err := rows.Scan(&b.ID, &b.TraceID, &b.Name, &b.Description, &b.CreatedAt)
		if err != nil {
			return nil, err
		}
		results = append(results, &b)
	}

	return results, rows.Err()
}

// UnmarkBaseline removes a baseline.
func (s *PostgresStorage) UnmarkBaseline(ctx context.Context, traceID string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM baselines WHERE trace_id = $1", traceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBaselineNotFound
	}
	return nil
}

// Drift Result methods

// CreateDriftResult saves a new drift calculation.
func (s *PostgresStorage) CreateDriftResult(ctx context.Context, r *DriftResult) error {
	query := `
		INSERT INTO drift_results (trace_id, baseline_trace_id, drift_score, verdict, details, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (trace_id, baseline_trace_id) DO UPDATE SET
			drift_score = EXCLUDED.drift_score,
			verdict = EXCLUDED.verdict,
			details = EXCLUDED.details,
			created_at = NOW()
	`
	_, err := s.pool.Exec(ctx, query, r.TraceID, r.BaselineTraceID, r.DriftScore, r.Verdict, r.Details)
	return err
}

// GetDriftResults retrieves all drift results for a trace.
func (s *PostgresStorage) GetDriftResults(ctx context.Context, traceID string) ([]*DriftResult, error) {
	query := `
		SELECT id, trace_id, baseline_trace_id, drift_score, verdict, details, created_at
		FROM drift_results
		WHERE trace_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, query, traceID)
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

// GetDriftResultsByBaseline retrieves drift results for candidates checked against a specific baseline.
func (s *PostgresStorage) GetDriftResultsByBaseline(ctx context.Context, baselineTraceID string, limit int) ([]*DriftResult, error) {
	query := `
		SELECT id, trace_id, baseline_trace_id, drift_score, verdict, details, created_at
		FROM drift_results
		WHERE baseline_trace_id = $1
		ORDER BY created_at DESC
	`
	args := []interface{}{baselineTraceID}
	if limit > 0 {
		query += " LIMIT $2"
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
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
func (s *PostgresStorage) ListDriftResults(ctx context.Context, limit int, offset int) ([]*DriftResult, error) {
	query := `
		SELECT id, trace_id, baseline_trace_id, drift_score, verdict, details, created_at
		FROM drift_results
		ORDER BY created_at DESC, id DESC
	`

	var args []interface{}
	argNum := 1
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, limit)
		argNum++
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, offset)
	}

	rows, err := s.pool.Query(ctx, query, args...)
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

// GetDriftResultForPair retrieves a specific drift result for a trace/baseline pair.
func (s *PostgresStorage) GetDriftResultForPair(ctx context.Context, traceID string, baselineTraceID string) (*DriftResult, error) {
	query := `
		SELECT id, trace_id, baseline_trace_id, drift_score, verdict, details, created_at
		FROM drift_results
		WHERE trace_id = $1 AND baseline_trace_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`

	var r DriftResult
	err := s.pool.QueryRow(ctx, query, traceID, baselineTraceID).Scan(
		&r.ID, &r.TraceID, &r.BaselineTraceID, &r.DriftScore, &r.Verdict, &r.Details, &r.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &r, nil
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
	err := s.pool.QueryRow(ctx, query, traceID).Scan(
		&r.ID, &r.TraceID, &r.BaselineTraceID, &r.DriftScore, &r.Verdict, &r.Details, &r.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &r, nil
}

// HasDriftResultForBaseline checks if a comparison exists for this pair.
func (s *PostgresStorage) HasDriftResultForBaseline(ctx context.Context, traceID string, baselineTraceID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drift_results WHERE trace_id = $1 AND baseline_trace_id = $2)`,
		traceID, baselineTraceID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
