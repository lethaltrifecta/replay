package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestPostgresURL() string {
	url := os.Getenv("CMDR_POSTGRES_URL")
	if url == "" {
		url = "postgres://cmdr:cmdr@localhost:5432/cmdr_test?sslmode=disable"
	}
	return url
}

func setupTestDB(t *testing.T) *PostgresStorage {
	storage, err := NewPostgresStorage(getTestPostgresURL(), 10)
	require.NoError(t, err)

	// Run migrations
	ctx := context.Background()
	err = storage.Migrate(ctx)
	require.NoError(t, err)

	return storage
}

func teardownTestDB(t *testing.T, storage *PostgresStorage) {
	ctx := context.Background()

	// Clean up all tables
	tables := []string{
		"evaluation_summary",
		"ground_truth",
		"human_evaluation_queue",
		"evaluator_results",
		"evaluation_runs",
		"evaluators",
		"analysis_results",
		"experiment_runs",
		"experiments",
		"tool_captures",
		"replay_traces",
		"otel_traces",
	}

	for _, table := range tables {
		_, err := storage.db.ExecContext(ctx, "TRUNCATE TABLE "+table+" CASCADE")
		if err != nil {
			t.Logf("Warning: failed to truncate %s: %v", table, err)
		}
	}

	storage.Close()
}

func TestPostgresStorage_Ping(t *testing.T) {
	storage := setupTestDB(t)
	defer teardownTestDB(t, storage)

	ctx := context.Background()
	err := storage.Ping(ctx)
	assert.NoError(t, err)
}

func TestPostgresStorage_ReplayTraces(t *testing.T) {
	storage := setupTestDB(t)
	defer teardownTestDB(t, storage)

	ctx := context.Background()

	// Create a replay trace
	trace := &ReplayTrace{
		TraceID:          "trace-123",
		RunID:            "run-123",
		CreatedAt:        time.Now(),
		Provider:         "anthropic",
		Model:            "claude-3-5-sonnet-20241022",
		Prompt:           JSONB{"messages": []interface{}{map[string]string{"role": "user", "content": "Hello"}}},
		Completion:       "Hi there!",
		Parameters:       JSONB{"temperature": 0.7},
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		LatencyMS:        100,
	}

	err := storage.CreateReplayTrace(ctx, trace)
	require.NoError(t, err)

	// Get the trace
	retrieved, err := storage.GetReplayTrace(ctx, "trace-123")
	require.NoError(t, err)
	assert.Equal(t, "trace-123", retrieved.TraceID)
	assert.Equal(t, "claude-3-5-sonnet-20241022", retrieved.Model)
	assert.Equal(t, 15, retrieved.TotalTokens)

	// List traces
	traces, err := storage.ListReplayTraces(ctx, TraceFilters{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, traces, 1)
}

func TestPostgresStorage_ToolCaptures(t *testing.T) {
	storage := setupTestDB(t)
	defer teardownTestDB(t, storage)

	ctx := context.Background()

	// Create a replay trace first
	trace := &ReplayTrace{
		TraceID:    "trace-456",
		RunID:      "run-456",
		CreatedAt:  time.Now(),
		Provider:   "anthropic",
		Model:      "claude-3-5-sonnet-20241022",
		Prompt:     JSONB{},
		Completion: "test",
	}
	err := storage.CreateReplayTrace(ctx, trace)
	require.NoError(t, err)

	// Create a tool capture
	capture := &ToolCapture{
		TraceID:   "trace-456",
		StepIndex: 0,
		ToolName:  "search",
		Args:      JSONB{"query": "test"},
		ArgsHash:  "abc123",
		Result:    JSONB{"results": []string{"result1", "result2"}},
		LatencyMS: 50,
		RiskClass: RiskClassRead,
		CreatedAt: time.Now(),
	}

	err = storage.CreateToolCapture(ctx, capture)
	require.NoError(t, err)
	assert.NotZero(t, capture.ID)

	// Get tool captures by trace
	captures, err := storage.GetToolCapturesByTrace(ctx, "trace-456")
	require.NoError(t, err)
	assert.Len(t, captures, 1)
	assert.Equal(t, "search", captures[0].ToolName)

	// Get tool capture by args
	retrieved, err := storage.GetToolCaptureByArgs(ctx, "search", "abc123")
	require.NoError(t, err)
	assert.Equal(t, "search", retrieved.ToolName)
	assert.Equal(t, RiskClassRead, retrieved.RiskClass)
}

func TestPostgresStorage_Experiments(t *testing.T) {
	storage := setupTestDB(t)
	defer teardownTestDB(t, storage)

	ctx := context.Background()

	// Create a replay trace first
	trace := &ReplayTrace{
		TraceID:    "trace-789",
		RunID:      "run-789",
		CreatedAt:  time.Now(),
		Provider:   "anthropic",
		Model:      "claude-3-5-sonnet-20241022",
		Prompt:     JSONB{},
		Completion: "test",
	}
	err := storage.CreateReplayTrace(ctx, trace)
	require.NoError(t, err)

	// Create an experiment
	expID := uuid.New()
	exp := &Experiment{
		ID:              expID,
		Name:            "Test Experiment",
		BaselineTraceID: "trace-789",
		Status:          StatusPending,
		Progress:        0.0,
		Config:          JSONB{"variants": []string{"model1", "model2"}},
		CreatedAt:       time.Now(),
	}

	err = storage.CreateExperiment(ctx, exp)
	require.NoError(t, err)

	// Get the experiment
	retrieved, err := storage.GetExperiment(ctx, expID)
	require.NoError(t, err)
	assert.Equal(t, "Test Experiment", retrieved.Name)
	assert.Equal(t, StatusPending, retrieved.Status)

	// Update the experiment
	retrieved.Status = StatusCompleted
	retrieved.Progress = 1.0
	now := time.Now()
	retrieved.CompletedAt = &now

	err = storage.UpdateExperiment(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated, err := storage.GetExperiment(ctx, expID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, updated.Status)
	assert.Equal(t, 1.0, updated.Progress)
	assert.NotNil(t, updated.CompletedAt)

	// List experiments
	experiments, err := storage.ListExperiments(ctx, ExperimentFilters{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, experiments, 1)
}

func TestPostgresStorage_Evaluators(t *testing.T) {
	storage := setupTestDB(t)
	defer teardownTestDB(t, storage)

	ctx := context.Background()

	// Create an evaluator
	evaluator := &Evaluator{
		Name:      "test-evaluator",
		Type:      EvaluatorTypeRule,
		Config:    JSONB{"rules": []string{"rule1", "rule2"}},
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := storage.CreateEvaluator(ctx, evaluator)
	require.NoError(t, err)
	assert.NotZero(t, evaluator.ID)

	// Get evaluator by ID
	retrieved, err := storage.GetEvaluator(ctx, evaluator.ID)
	require.NoError(t, err)
	assert.Equal(t, "test-evaluator", retrieved.Name)
	assert.Equal(t, EvaluatorTypeRule, retrieved.Type)

	// Get evaluator by name
	retrieved, err = storage.GetEvaluatorByName(ctx, "test-evaluator")
	require.NoError(t, err)
	assert.Equal(t, evaluator.ID, retrieved.ID)

	// Update evaluator
	retrieved.Enabled = false
	err = storage.UpdateEvaluator(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated, err := storage.GetEvaluator(ctx, evaluator.ID)
	require.NoError(t, err)
	assert.False(t, updated.Enabled)

	// List evaluators
	evaluators, err := storage.ListEvaluators(ctx, false)
	require.NoError(t, err)
	assert.Len(t, evaluators, 1)

	// List enabled only
	enabledOnly, err := storage.ListEvaluators(ctx, true)
	require.NoError(t, err)
	assert.Len(t, enabledOnly, 0)

	// Delete evaluator
	err = storage.DeleteEvaluator(ctx, evaluator.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = storage.GetEvaluator(ctx, evaluator.ID)
	assert.Error(t, err)
}

func TestPostgresStorage_GroundTruth(t *testing.T) {
	storage := setupTestDB(t)
	defer teardownTestDB(t, storage)

	ctx := context.Background()

	// Create ground truth
	gt := &GroundTruth{
		TaskID:         "task-123",
		TaskType:       "code_generation",
		Input:          JSONB{"prompt": "Write a function"},
		ExpectedOutput: "def my_function(): pass",
		Metadata:       JSONB{"difficulty": "easy"},
		CreatedAt:      time.Now(),
	}

	err := storage.CreateGroundTruth(ctx, gt)
	require.NoError(t, err)
	assert.NotZero(t, gt.ID)

	// Get ground truth
	retrieved, err := storage.GetGroundTruth(ctx, "task-123")
	require.NoError(t, err)
	assert.Equal(t, "code_generation", retrieved.TaskType)

	// Update ground truth
	retrieved.ExpectedOutput = "def my_function():\n    return None"
	err = storage.UpdateGroundTruth(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated, err := storage.GetGroundTruth(ctx, "task-123")
	require.NoError(t, err)
	assert.Contains(t, updated.ExpectedOutput, "return None")

	// List ground truth
	taskType := "code_generation"
	gts, err := storage.ListGroundTruth(ctx, &taskType)
	require.NoError(t, err)
	assert.Len(t, gts, 1)

	// List all
	allGts, err := storage.ListGroundTruth(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, allGts, 1)

	// Delete ground truth
	err = storage.DeleteGroundTruth(ctx, "task-123")
	require.NoError(t, err)

	// Verify deletion
	_, err = storage.GetGroundTruth(ctx, "task-123")
	assert.Error(t, err)
}
