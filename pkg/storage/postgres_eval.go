package storage

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Evaluation Run methods

func (s *PostgresStorage) CreateEvaluationRun(ctx context.Context, run *EvaluationRun) error {
	query := `
		INSERT INTO evaluation_runs (id, experiment_run_id, status, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := s.pool.Exec(ctx, query,
		run.ID, run.ExperimentRunID, run.Status, run.StartedAt, run.CompletedAt,
	)

	return err
}

func (s *PostgresStorage) GetEvaluationRun(ctx context.Context, id uuid.UUID) (*EvaluationRun, error) {
	query := `
		SELECT id, experiment_run_id, status, started_at, completed_at
		FROM evaluation_runs
		WHERE id = $1
	`

	var run EvaluationRun
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&run.ID, &run.ExperimentRunID, &run.Status, &run.StartedAt, &run.CompletedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEvaluationRunNotFound
	}
	if err != nil {
		return nil, err
	}

	return &run, nil
}

func (s *PostgresStorage) UpdateEvaluationRun(ctx context.Context, run *EvaluationRun) error {
	query := `
		UPDATE evaluation_runs
		SET status = $1, completed_at = $2
		WHERE id = $3
	`

	_, err := s.pool.Exec(ctx, query, run.Status, run.CompletedAt, run.ID)
	return err
}

// Evaluator Result methods

func (s *PostgresStorage) CreateEvaluatorResult(ctx context.Context, result *EvaluatorResult) error {
	query := `
		INSERT INTO evaluator_results (
			evaluation_run_id, evaluator_id, scores, overall_score, passed,
			reasoning, metadata, evaluated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`

	return s.pool.QueryRow(ctx, query,
		result.EvaluationRunID, result.EvaluatorID, result.Scores, result.OverallScore,
		result.Passed, result.Reasoning, result.Metadata, result.EvaluatedAt,
	).Scan(&result.ID)
}

func (s *PostgresStorage) GetEvaluatorResults(ctx context.Context, evaluationRunID uuid.UUID) ([]*EvaluatorResult, error) {
	query := `
		SELECT id, evaluation_run_id, evaluator_id, scores, overall_score, passed,
		       reasoning, metadata, evaluated_at
		FROM evaluator_results
		WHERE evaluation_run_id = $1
		ORDER BY evaluated_at ASC
	`

	rows, err := s.pool.Query(ctx, query, evaluationRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*EvaluatorResult
	for rows.Next() {
		var result EvaluatorResult
		err := rows.Scan(
			&result.ID, &result.EvaluationRunID, &result.EvaluatorID, &result.Scores,
			&result.OverallScore, &result.Passed, &result.Reasoning, &result.Metadata,
			&result.EvaluatedAt,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, &result)
	}

	return results, rows.Err()
}

// Human Evaluation methods

func (s *PostgresStorage) CreateHumanEvaluation(ctx context.Context, eval *HumanEvaluation) error {
	query := `
		INSERT INTO human_evaluation_queue (
			id, evaluation_run_id, experiment_run_id, output, context, status,
			assigned_to, scores, feedback, created_at, assigned_at, reviewed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err := s.pool.Exec(ctx, query,
		eval.ID, eval.EvaluationRunID, eval.ExperimentRunID, eval.Output, eval.Context,
		eval.Status, eval.AssignedTo, eval.Scores, eval.Feedback, eval.CreatedAt,
		eval.AssignedAt, eval.ReviewedAt,
	)

	return err
}

func (s *PostgresStorage) GetHumanEvaluation(ctx context.Context, id uuid.UUID) (*HumanEvaluation, error) {
	query := `
		SELECT id, evaluation_run_id, experiment_run_id, output, context, status,
		       assigned_to, scores, feedback, created_at, assigned_at, reviewed_at
		FROM human_evaluation_queue
		WHERE id = $1
	`

	var eval HumanEvaluation
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&eval.ID, &eval.EvaluationRunID, &eval.ExperimentRunID, &eval.Output, &eval.Context,
		&eval.Status, &eval.AssignedTo, &eval.Scores, &eval.Feedback, &eval.CreatedAt,
		&eval.AssignedAt, &eval.ReviewedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrHumanEvaluationNotFound
	}
	if err != nil {
		return nil, err
	}

	return &eval, nil
}

func (s *PostgresStorage) UpdateHumanEvaluation(ctx context.Context, eval *HumanEvaluation) error {
	query := `
		UPDATE human_evaluation_queue
		SET status = $1, assigned_to = $2, scores = $3, feedback = $4,
		    assigned_at = $5, reviewed_at = $6
		WHERE id = $7
	`

	_, err := s.pool.Exec(ctx, query,
		eval.Status, eval.AssignedTo, eval.Scores, eval.Feedback,
		eval.AssignedAt, eval.ReviewedAt, eval.ID,
	)

	return err
}

func (s *PostgresStorage) ListPendingHumanEvaluations(ctx context.Context, assignedTo *string) ([]*HumanEvaluation, error) {
	query := `
		SELECT id, evaluation_run_id, experiment_run_id, output, context, status,
		       assigned_to, scores, feedback, created_at, assigned_at, reviewed_at
		FROM human_evaluation_queue
		WHERE status IN ($1, $2)
	`

	args := []interface{}{HumanEvalStatusPending, HumanEvalStatusInReview}

	if assignedTo != nil {
		query += " AND (assigned_to = $3 OR assigned_to IS NULL)"
		args = append(args, *assignedTo)
	}

	query += " ORDER BY created_at ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evals []*HumanEvaluation
	for rows.Next() {
		var eval HumanEvaluation
		err := rows.Scan(
			&eval.ID, &eval.EvaluationRunID, &eval.ExperimentRunID, &eval.Output, &eval.Context,
			&eval.Status, &eval.AssignedTo, &eval.Scores, &eval.Feedback, &eval.CreatedAt,
			&eval.AssignedAt, &eval.ReviewedAt,
		)
		if err != nil {
			return nil, err
		}
		evals = append(evals, &eval)
	}

	return evals, rows.Err()
}

// Ground Truth methods

func (s *PostgresStorage) CreateGroundTruth(ctx context.Context, gt *GroundTruth) error {
	query := `
		INSERT INTO ground_truth (task_id, task_type, input, expected_output, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	return s.pool.QueryRow(ctx, query,
		gt.TaskID, gt.TaskType, gt.Input, gt.ExpectedOutput, gt.Metadata, gt.CreatedAt,
	).Scan(&gt.ID)
}

func (s *PostgresStorage) GetGroundTruth(ctx context.Context, taskID string) (*GroundTruth, error) {
	query := `
		SELECT id, task_id, task_type, input, expected_output, metadata, created_at
		FROM ground_truth
		WHERE task_id = $1
	`

	var gt GroundTruth
	err := s.pool.QueryRow(ctx, query, taskID).Scan(
		&gt.ID, &gt.TaskID, &gt.TaskType, &gt.Input, &gt.ExpectedOutput, &gt.Metadata, &gt.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGroundTruthNotFound
	}
	if err != nil {
		return nil, err
	}

	return &gt, nil
}

func (s *PostgresStorage) UpdateGroundTruth(ctx context.Context, gt *GroundTruth) error {
	query := `
		UPDATE ground_truth
		SET task_type = $1, input = $2, expected_output = $3, metadata = $4
		WHERE task_id = $5
	`

	_, err := s.pool.Exec(ctx, query,
		gt.TaskType, gt.Input, gt.ExpectedOutput, gt.Metadata, gt.TaskID,
	)

	return err
}

func (s *PostgresStorage) ListGroundTruth(ctx context.Context, taskType *string) ([]*GroundTruth, error) {
	query := `
		SELECT id, task_id, task_type, input, expected_output, metadata, created_at
		FROM ground_truth
	`

	args := []interface{}{}

	if taskType != nil {
		query += " WHERE task_type = $1"
		args = append(args, *taskType)
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var gts []*GroundTruth
	for rows.Next() {
		var gt GroundTruth
		err := rows.Scan(
			&gt.ID, &gt.TaskID, &gt.TaskType, &gt.Input, &gt.ExpectedOutput, &gt.Metadata, &gt.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		gts = append(gts, &gt)
	}

	return gts, rows.Err()
}

func (s *PostgresStorage) DeleteGroundTruth(ctx context.Context, taskID string) error {
	query := `DELETE FROM ground_truth WHERE task_id = $1`
	_, err := s.pool.Exec(ctx, query, taskID)
	return err
}

// Evaluation Summary methods

func (s *PostgresStorage) CreateEvaluationSummary(ctx context.Context, summary *EvaluationSummary) error {
	query := `
		INSERT INTO evaluation_summary (
			experiment_id, experiment_run_id, overall_score, passed, evaluator_scores,
			rank, is_best, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`

	return s.pool.QueryRow(ctx, query,
		summary.ExperimentID, summary.ExperimentRunID, summary.OverallScore, summary.Passed,
		summary.EvaluatorScores, summary.Rank, summary.IsBest, summary.CreatedAt,
	).Scan(&summary.ID)
}

func (s *PostgresStorage) GetEvaluationSummary(ctx context.Context, experimentID uuid.UUID) ([]*EvaluationSummary, error) {
	query := `
		SELECT id, experiment_id, experiment_run_id, overall_score, passed, evaluator_scores,
		       rank, is_best, created_at
		FROM evaluation_summary
		WHERE experiment_id = $1
		ORDER BY rank ASC
	`

	rows, err := s.pool.Query(ctx, query, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []*EvaluationSummary
	for rows.Next() {
		var summary EvaluationSummary
		err := rows.Scan(
			&summary.ID, &summary.ExperimentID, &summary.ExperimentRunID, &summary.OverallScore,
			&summary.Passed, &summary.EvaluatorScores, &summary.Rank, &summary.IsBest, &summary.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, &summary)
	}

	return summaries, rows.Err()
}
