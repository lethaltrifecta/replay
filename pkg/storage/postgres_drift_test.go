//go:build integration

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int { return &i }

// helper to insert a replay trace for baseline tests
func insertTestTrace(t *testing.T, s *PostgresStorage, traceID string) {
	t.Helper()
	ctx := context.Background()
	trace := &ReplayTrace{
		TraceID:    traceID,
		SpanID:     "span-" + traceID,
		RunID:      traceID,
		StepIndex:  0,
		CreatedAt:  time.Now(),
		Provider:   "anthropic",
		Model:      "claude-3-5-sonnet-20241022",
		Prompt:     JSONB{"messages": []interface{}{}},
		Completion: "test completion",
	}
	err := s.CreateReplayTrace(ctx, trace)
	require.NoError(t, err)
}

func TestMarkTraceAsBaseline(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()
	insertTestTrace(t, store, "baseline-trace-1")

	// Mark as baseline
	name := "v1 baseline"
	baseline := &Baseline{
		TraceID: "baseline-trace-1",
		Name:    &name,
	}
	err := store.MarkTraceAsBaseline(ctx, baseline)
	require.NoError(t, err)
	assert.NotZero(t, baseline.ID)
	assert.False(t, baseline.CreatedAt.IsZero())

	// Verify it was stored
	got, err := store.GetBaseline(ctx, "baseline-trace-1")
	require.NoError(t, err)
	assert.Equal(t, "baseline-trace-1", got.TraceID)
	assert.Equal(t, "v1 baseline", *got.Name)
	assert.Nil(t, got.Description)

	// Upsert: re-mark with different metadata
	newName := "v2 baseline"
	desc := "updated description"
	baseline2 := &Baseline{
		TraceID:     "baseline-trace-1",
		Name:        &newName,
		Description: &desc,
	}
	err = store.MarkTraceAsBaseline(ctx, baseline2)
	require.NoError(t, err)

	got2, err := store.GetBaseline(ctx, "baseline-trace-1")
	require.NoError(t, err)
	assert.Equal(t, "v2 baseline", *got2.Name)
	assert.Equal(t, "updated description", *got2.Description)
}

func TestMarkTraceAsBaseline_PreservesOmittedFieldsOnRemark(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()
	insertTestTrace(t, store, "baseline-trace-preserve")

	name := "original name"
	description := "original description"
	err := store.MarkTraceAsBaseline(ctx, &Baseline{
		TraceID:     "baseline-trace-preserve",
		Name:        &name,
		Description: &description,
	})
	require.NoError(t, err)

	updatedName := "updated name"
	err = store.MarkTraceAsBaseline(ctx, &Baseline{
		TraceID: "baseline-trace-preserve",
		Name:    &updatedName,
	})
	require.NoError(t, err)

	got, err := store.GetBaseline(ctx, "baseline-trace-preserve")
	require.NoError(t, err)
	require.NotNil(t, got.Name)
	require.NotNil(t, got.Description)
	assert.Equal(t, "updated name", *got.Name)
	assert.Equal(t, "original description", *got.Description)
}

func TestMarkTraceAsBaseline_TraceNotFound(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()
	baseline := &Baseline{TraceID: "nonexistent-trace"}
	err := store.MarkTraceAsBaseline(ctx, baseline)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "trace not found")
}

func TestGetBaseline(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	// Not found
	_, err := store.GetBaseline(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "baseline not found")

	// Found
	insertTestTrace(t, store, "get-baseline-trace")
	name := "test"
	err = store.MarkTraceAsBaseline(ctx, &Baseline{TraceID: "get-baseline-trace", Name: &name})
	require.NoError(t, err)

	got, err := store.GetBaseline(ctx, "get-baseline-trace")
	require.NoError(t, err)
	assert.Equal(t, "get-baseline-trace", got.TraceID)
	assert.Equal(t, "test", *got.Name)
}

func TestListBaselines(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	// Empty list
	baselines, err := store.ListBaselines(ctx)
	require.NoError(t, err)
	assert.Empty(t, baselines)

	// Populated
	insertTestTrace(t, store, "list-trace-1")
	insertTestTrace(t, store, "list-trace-2")

	name1 := "first"
	name2 := "second"
	err = store.MarkTraceAsBaseline(ctx, &Baseline{TraceID: "list-trace-1", Name: &name1})
	require.NoError(t, err)
	err = store.MarkTraceAsBaseline(ctx, &Baseline{TraceID: "list-trace-2", Name: &name2})
	require.NoError(t, err)

	baselines, err = store.ListBaselines(ctx)
	require.NoError(t, err)
	assert.Len(t, baselines, 2)
	// Ordered by created_at DESC — second should be first
	assert.Equal(t, "list-trace-2", baselines[0].TraceID)
	assert.Equal(t, "list-trace-1", baselines[1].TraceID)
}

func TestUnmarkBaseline(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	// Remove non-existent
	err := store.UnmarkBaseline(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "baseline not found")

	// Remove existing
	insertTestTrace(t, store, "unmark-trace")
	err = store.MarkTraceAsBaseline(ctx, &Baseline{TraceID: "unmark-trace"})
	require.NoError(t, err)

	err = store.UnmarkBaseline(ctx, "unmark-trace")
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.GetBaseline(ctx, "unmark-trace")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "baseline not found")
}

func TestCreateDriftResult(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	result := &DriftResult{
		TraceID:         "drift-trace-1",
		BaselineTraceID: "baseline-1",
		DriftScore:      0.15,
		Verdict:         DriftVerdictPass,
		Details: DriftDetails{
			Reason:         "tool pattern changed",
			DivergenceStep: intPtr(1),
			RiskEscalation: false,
		},
	}

	err := store.CreateDriftResult(ctx, result)
	require.NoError(t, err)
	assert.NotZero(t, result.ID)
	assert.False(t, result.CreatedAt.IsZero())
}

func TestGetDriftResults(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	// Insert two results for the same trace
	r1 := &DriftResult{
		TraceID:         "drift-by-trace",
		BaselineTraceID: "baseline-a",
		DriftScore:      0.1,
		Verdict:         DriftVerdictPass,
	}
	err := store.CreateDriftResult(ctx, r1)
	require.NoError(t, err)

	r2 := &DriftResult{
		TraceID:         "drift-by-trace",
		BaselineTraceID: "baseline-b",
		DriftScore:      0.8,
		Verdict:         DriftVerdictFail,
	}
	err = store.CreateDriftResult(ctx, r2)
	require.NoError(t, err)

	// Different trace
	r3 := &DriftResult{
		TraceID:         "other-trace",
		BaselineTraceID: "baseline-c",
		DriftScore:      0.3,
		Verdict:         DriftVerdictWarn,
	}
	err = store.CreateDriftResult(ctx, r3)
	require.NoError(t, err)

	// Get by trace
	results, err := store.GetDriftResults(ctx, "drift-by-trace")
	require.NoError(t, err)
	assert.Len(t, results, 2)
	// Newest first
	assert.Equal(t, DriftVerdictFail, results[0].Verdict)
	assert.Equal(t, DriftVerdictPass, results[1].Verdict)

	// Other trace
	results, err = store.GetDriftResults(ctx, "other-trace")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestGetDriftResultsByBaseline(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	// Two traces compared against the same baseline
	r1 := &DriftResult{
		TraceID:         "trace-x",
		BaselineTraceID: "shared-baseline",
		DriftScore:      0.2,
		Verdict:         DriftVerdictPass,
	}
	err := store.CreateDriftResult(ctx, r1)
	require.NoError(t, err)

	r2 := &DriftResult{
		TraceID:         "trace-y",
		BaselineTraceID: "shared-baseline",
		DriftScore:      0.9,
		Verdict:         DriftVerdictFail,
	}
	err = store.CreateDriftResult(ctx, r2)
	require.NoError(t, err)

	results, err := store.GetDriftResultsByBaseline(ctx, "shared-baseline", 0)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	// Newest first
	assert.Equal(t, "trace-y", results[0].TraceID)
	assert.Equal(t, "trace-x", results[1].TraceID)
}

func TestGetLatestDriftResult(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	// Not found
	_, err := store.GetLatestDriftResult(ctx, "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)

	// Insert multiple, should get the latest
	r1 := &DriftResult{
		TraceID:         "latest-trace",
		BaselineTraceID: "baseline-z",
		DriftScore:      0.1,
		Verdict:         DriftVerdictPass,
	}
	err = store.CreateDriftResult(ctx, r1)
	require.NoError(t, err)

	r2 := &DriftResult{
		TraceID:         "latest-trace",
		BaselineTraceID: "baseline-z2",
		DriftScore:      0.7,
		Verdict:         DriftVerdictWarn,
	}
	err = store.CreateDriftResult(ctx, r2)
	require.NoError(t, err)

	latest, err := store.GetLatestDriftResult(ctx, "latest-trace")
	require.NoError(t, err)
	assert.Equal(t, 0.7, latest.DriftScore)
	assert.Equal(t, DriftVerdictWarn, latest.Verdict)
}

func TestListDriftResults(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	// Empty
	results, err := store.ListDriftResults(ctx, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, results)

	// Insert 3 results
	for i, tid := range []string{"trace-a", "trace-b", "trace-c"} {
		r := &DriftResult{
			TraceID:         tid,
			BaselineTraceID: "baseline-x",
			DriftScore:      float64(i) * 0.1,
			Verdict:         DriftVerdictPass,
		}
		err := store.CreateDriftResult(ctx, r)
		require.NoError(t, err)
	}

	// Limit 2 — should get the 2 newest
	results, err = store.ListDriftResults(ctx, 2, 0)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "trace-c", results[0].TraceID)
	assert.Equal(t, "trace-b", results[1].TraceID)

	// Offset 1 — skip newest, get next two (only b, a available)
	results, err = store.ListDriftResults(ctx, 10, 1)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "trace-b", results[0].TraceID)
	assert.Equal(t, "trace-a", results[1].TraceID)

	// Limit larger than count — returns all
	results, err = store.ListDriftResults(ctx, 100, 0)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestGetDriftResultForPair(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	// Insert results for multiple pairs
	pairs := [][]string{
		{"trace-1", "base-a"},
		{"trace-1", "base-b"},
		{"trace-2", "base-a"},
	}
	for _, p := range pairs {
		err := store.CreateDriftResult(ctx, &DriftResult{
			TraceID:         p[0],
			BaselineTraceID: p[1],
			DriftScore:      0.5,
			Verdict:         DriftVerdictPass,
		})
		require.NoError(t, err)
	}

	// Fetch specific pair
	res, err := store.GetDriftResultForPair(ctx, "trace-1", "base-b")
	require.NoError(t, err)
	assert.Equal(t, "trace-1", res.TraceID)
	assert.Equal(t, "base-b", res.BaselineTraceID)

	// Fetch non-existent pair
	_, err = store.GetDriftResultForPair(ctx, "trace-1", "base-c")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHasDriftResultForBaseline(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	// No results
	exists, err := store.HasDriftResultForBaseline(ctx, "trace-1", "baseline-1")
	require.NoError(t, err)
	assert.False(t, exists)

	// Insert a result for trace-1 against baseline-1
	r := &DriftResult{
		TraceID:         "trace-1",
		BaselineTraceID: "baseline-1",
		DriftScore:      0.1,
		Verdict:         DriftVerdictPass,
	}
	err = store.CreateDriftResult(ctx, r)
	require.NoError(t, err)

	// Now exists for same pair
	exists, err = store.HasDriftResultForBaseline(ctx, "trace-1", "baseline-1")
	require.NoError(t, err)
	assert.True(t, exists)

	// Does NOT exist for different baseline
	exists, err = store.HasDriftResultForBaseline(ctx, "trace-1", "baseline-2")
	require.NoError(t, err)
	assert.False(t, exists)

	// Does NOT exist for different trace
	exists, err = store.HasDriftResultForBaseline(ctx, "trace-2", "baseline-1")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCreateDriftResult_DuplicateIsIdempotent(t *testing.T) {
	store := setupTestDB(t)
	defer teardownTestDB(t, store)

	ctx := context.Background()

	r1 := &DriftResult{
		TraceID:         "dup-trace",
		BaselineTraceID: "dup-baseline",
		DriftScore:      0.25,
		Verdict:         DriftVerdictPass,
		Details: DriftDetails{
			Reason:         "first",
			DivergenceStep: intPtr(1),
			RiskEscalation: false,
		},
	}
	err := store.CreateDriftResult(ctx, r1)
	require.NoError(t, err)
	assert.NotZero(t, r1.ID)

	// Second insert with same (trace_id, baseline_trace_id) should succeed silently
	r2 := &DriftResult{
		TraceID:         "dup-trace",
		BaselineTraceID: "dup-baseline",
		DriftScore:      0.9,
		Verdict:         DriftVerdictFail,
		Details: DriftDetails{
			Reason:         "second",
			DivergenceStep: intPtr(2),
			RiskEscalation: true,
		},
	}
	err = store.CreateDriftResult(ctx, r2)
	require.NoError(t, err, "duplicate insert should be idempotent")

	// Only one row should exist
	results, err := store.GetDriftResults(ctx, "dup-trace")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 0.9, results[0].DriftScore, "upsert should replace the existing row")
	assert.Equal(t, "second", results[0].Details.Reason)
}
