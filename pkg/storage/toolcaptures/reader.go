package toolcaptures

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("tool capture not found")

type Capture struct {
	ID        int
	TraceID   string
	SpanID    string
	StepIndex int
	ToolName  string
	Args      map[string]any
	ArgsHash  string
	Result    map[string]any
	Error     *string
	LatencyMS int
	RiskClass string
	CreatedAt time.Time
}

type Reader interface {
	ListToolNamesByTrace(ctx context.Context, traceID string) ([]string, error)
	GetByTrace(ctx context.Context, traceID string) ([]*Capture, error)
	GetByArgs(ctx context.Context, toolName, argsHash string) (*Capture, error)
	GetByArgsAndTrace(ctx context.Context, toolName, argsHash, traceID string) (*Capture, error)
	GetByLocator(ctx context.Context, traceID, spanID string, stepIndex int) (*Capture, error)
}

type PostgresReader struct {
	pool *pgxpool.Pool
}

func NewPostgresReader(pool *pgxpool.Pool) *PostgresReader {
	return &PostgresReader{pool: pool}
}

func (r *PostgresReader) ListToolNamesByTrace(ctx context.Context, traceID string) ([]string, error) {
	query := `
		SELECT DISTINCT tool_name
		FROM tool_captures
		WHERE trace_id = $1
		ORDER BY tool_name
	`

	rows, err := r.pool.Query(ctx, query, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var toolNames []string
	for rows.Next() {
		var toolName string
		if err := rows.Scan(&toolName); err != nil {
			return nil, err
		}
		toolNames = append(toolNames, toolName)
	}

	return toolNames, rows.Err()
}

func (r *PostgresReader) GetByTrace(ctx context.Context, traceID string) ([]*Capture, error) {
	query := `
		SELECT id, trace_id, span_id, step_index, tool_name, args, args_hash, result, error,
		       latency_ms, risk_class, created_at
		FROM tool_captures
		WHERE trace_id = $1
		ORDER BY step_index ASC, created_at ASC, id ASC
	`

	rows, err := r.pool.Query(ctx, query, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var captures []*Capture
	for rows.Next() {
		capture, err := scanCapture(rows)
		if err != nil {
			return nil, err
		}
		captures = append(captures, capture)
	}

	return captures, rows.Err()
}

func (r *PostgresReader) GetByArgs(ctx context.Context, toolName, argsHash string) (*Capture, error) {
	query := `
		SELECT id, trace_id, span_id, step_index, tool_name, args, args_hash, result, error,
		       latency_ms, risk_class, created_at
		FROM tool_captures
		WHERE tool_name = $1 AND args_hash = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	return r.queryCapture(ctx, query, toolName, argsHash)
}

func (r *PostgresReader) GetByArgsAndTrace(
	ctx context.Context,
	toolName, argsHash, traceID string,
) (*Capture, error) {
	query := `
		SELECT id, trace_id, span_id, step_index, tool_name, args, args_hash, result, error,
		       latency_ms, risk_class, created_at
		FROM tool_captures
		WHERE tool_name = $1 AND args_hash = $2 AND trace_id = $3
		ORDER BY created_at DESC
		LIMIT 1
	`

	return r.queryCapture(ctx, query, toolName, argsHash, traceID)
}

func (r *PostgresReader) GetByLocator(ctx context.Context, traceID, spanID string, stepIndex int) (*Capture, error) {
	query := `
		SELECT id, trace_id, span_id, step_index, tool_name, args, args_hash, result, error,
		       latency_ms, risk_class, created_at
		FROM tool_captures
		WHERE trace_id = $1 AND span_id = $2 AND step_index = $3
		LIMIT 1
	`

	return r.queryCapture(ctx, query, traceID, spanID, stepIndex)
}

func (r *PostgresReader) queryCapture(ctx context.Context, query string, args ...any) (*Capture, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, ErrNotFound
	}

	capture, err := scanCapture(rows)
	if err != nil {
		return nil, err
	}
	return capture, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCapture(scanner rowScanner) (*Capture, error) {
	var capture Capture
	err := scanner.Scan(
		&capture.ID,
		&capture.TraceID,
		&capture.SpanID,
		&capture.StepIndex,
		&capture.ToolName,
		&capture.Args,
		&capture.ArgsHash,
		&capture.Result,
		&capture.Error,
		&capture.LatencyMS,
		&capture.RiskClass,
		&capture.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &capture, nil
}
