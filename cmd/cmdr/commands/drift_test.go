package commands

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/drift"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// --- Test Helpers ---

func makeSpan(traceID string, createdAt time.Time) *storage.ReplayTrace {
	return &storage.ReplayTrace{
		TraceID:   traceID,
		CreatedAt: createdAt,
		Model:     "gpt-4",
		Provider:  "openai",
	}
}

// --- Tests ---

func TestPollAndCheck_NoNewTraces(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1"}
	cfg := drift.DefaultConfig()
	retryTraces := make(map[string]int)

	result, err := pollAndCheck(ctx, store, "baseline-1", baselineFP, cfg, storage.TraceFilters{}, time.Now(), retryTraces)
	require.NoError(t, err)

	assert.Equal(t, 0, result.checked)
	assert.True(t, result.highWater.IsZero())
}

func TestPollAndCheck_SkipsBaselineTrace(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("baseline-1", now),
	}

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1"}
	cfg := drift.DefaultConfig()
	retryTraces := make(map[string]int)

	result, err := pollAndCheck(ctx, store, "baseline-1", baselineFP, cfg, storage.TraceFilters{}, time.Now().Add(-time.Hour), retryTraces)
	require.NoError(t, err)

	assert.Equal(t, 0, result.checked)
	assert.Equal(t, now, result.highWater)
}

func TestPollAndCheck_SkipsAlreadyChecked(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", now),
	}
	store.driftResultsExist["trace-1:baseline-1"] = true

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1"}
	cfg := drift.DefaultConfig()
	retryTraces := make(map[string]int)

	result, err := pollAndCheck(ctx, store, "baseline-1", baselineFP, cfg, storage.TraceFilters{}, now.Add(-time.Hour), retryTraces)
	require.NoError(t, err)

	assert.Equal(t, 0, result.checked)
	assert.Equal(t, now, result.highWater)
}

func TestPollAndCheck_HighWaterMark(t *testing.T) {
	store := newMockStore()
	t1 := time.Now().Add(-2 * time.Minute)
	t2 := time.Now().Add(-1 * time.Minute)
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", t1),
		makeSpan("trace-2", t2),
	}
	store.driftResultsExist["trace-1:baseline-1"] = true
	store.driftResultsExist["trace-2:baseline-1"] = true

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1"}
	cfg := drift.DefaultConfig()
	retryTraces := make(map[string]int)

	result, err := pollAndCheck(ctx, store, "baseline-1", baselineFP, cfg, storage.TraceFilters{}, time.Now().Add(-5*time.Minute), retryTraces)
	require.NoError(t, err)

	assert.Equal(t, t2, result.highWater)
}

func TestPollAndCheck_PaginatesBacklog(t *testing.T) {
	store := newMockStore()
	start := time.Now().Add(-10 * time.Minute)
	for i := 0; i < 105; i++ {
		store.replayTraces = append(store.replayTraces, makeSpan(
			fmt.Sprintf("trace-%03d", i),
			start.Add(time.Duration(i)*time.Second),
		))
	}

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{
		TraceID:   "baseline-1",
		StepCount: 1,
		Models:    []string{"gpt-4"},
		Providers: []string{"openai"},
	}
	cfg := drift.DefaultConfig()
	retryTraces := make(map[string]int)

	result, err := pollAndCheck(ctx, store, "baseline-1", baselineFP, cfg, storage.TraceFilters{SortAsc: false}, start.Add(-time.Second), retryTraces)
	require.NoError(t, err)

	assert.Equal(t, 105, result.checked)
	assert.True(t, result.overflow)
	assert.Len(t, store.createdDriftResults, 105)
	assert.Equal(t, start.Add(104*time.Second), result.highWater)
}

func TestPollAndCheck_QueuesRetryAndAdvancesHighWaterOnTraceFailure(t *testing.T) {
	store := newMockStore()
	lastSeen := time.Now().Add(-time.Minute)
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", lastSeen.Add(10*time.Second)),
	}
	store.createResultErr = errors.New("write failed")

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{
		TraceID:   "baseline-1",
		StepCount: 1,
		Models:    []string{"gpt-4"},
		Providers: []string{"openai"},
	}
	cfg := drift.DefaultConfig()
	retryTraces := make(map[string]int)

	result, err := pollAndCheck(ctx, store, "baseline-1", baselineFP, cfg, storage.TraceFilters{SortAsc: false}, lastSeen, retryTraces)
	require.NoError(t, err)

	assert.Equal(t, 0, result.checked)
	assert.Equal(t, lastSeen.Add(10*time.Second), result.highWater)
	assert.Equal(t, []string{"trace-1"}, result.retried)
	assert.Equal(t, 1, retryTraces["trace-1"])
}

func TestPollAndCheck_DropsPoisonTraceAfterMaxRetries(t *testing.T) {
	store := newMockStore()
	lastSeen := time.Now().Add(-time.Minute)
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", lastSeen.Add(10*time.Second)),
	}
	store.createResultErr = errors.New("write failed")

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{
		TraceID:   "baseline-1",
		StepCount: 1,
		Models:    []string{"gpt-4"},
		Providers: []string{"openai"},
	}
	cfg := drift.DefaultConfig()
	retryTraces := map[string]int{"trace-1": maxDriftWatchRetries - 1}

	result, err := pollAndCheck(ctx, store, "baseline-1", baselineFP, cfg, storage.TraceFilters{SortAsc: false}, lastSeen, retryTraces)
	require.NoError(t, err)

	assert.Empty(t, result.retried)
	assert.Equal(t, []string{"trace-1"}, result.dropped)
	assert.NotContains(t, retryTraces, "trace-1")
}

func TestCheckOneTrace_HappyPath(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("candidate-1", now),
	}

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{
		TraceID:   "baseline-1",
		StepCount: 1,
		Models:    []string{"gpt-4"},
		Providers: []string{"openai"},
	}
	cfg := drift.DefaultConfig()

	report, err := checkOneTrace(ctx, store, "baseline-1", baselineFP, cfg, "candidate-1")

	require.NoError(t, err)
	assert.NotNil(t, report)
	require.Len(t, store.createdDriftResults, 1)
	assert.Equal(t, "candidate-1", store.createdDriftResults[0].TraceID)
}
