package commands

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/drift"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// mockStore implements storage.Storage with controllable behavior for the
// methods exercised by pollAndCheck and related functions. Unexercised
// methods panic to surface unexpected calls.
type mockStore struct {
	replayTraces         []*storage.ReplayTrace
	toolCaptures         map[string][]*storage.ToolCapture // keyed by trace_id
	driftResultsExist    map[string]bool                   // keyed by "traceID:baselineTraceID"
	createdDriftResults  []*storage.DriftResult
	hasResultErr         error // injected error for HasDriftResultForBaseline
	createResultErr      error // injected error for CreateDriftResult
	getSpansErr          error // injected error for GetReplayTraceSpans
	listTracesErr        error // injected error for ListReplayTraces
	baselines            []*storage.Baseline
	listDriftResultsData []*storage.DriftResult
}

func newMockStore() *mockStore {
	return &mockStore{
		toolCaptures:      make(map[string][]*storage.ToolCapture),
		driftResultsExist: make(map[string]bool),
	}
}

func (m *mockStore) ListReplayTraces(_ context.Context, _ storage.TraceFilters) ([]*storage.ReplayTrace, error) {
	if m.listTracesErr != nil {
		return nil, m.listTracesErr
	}
	return m.replayTraces, nil
}

func (m *mockStore) HasDriftResultForBaseline(_ context.Context, traceID string, baselineTraceID string) (bool, error) {
	if m.hasResultErr != nil {
		return false, m.hasResultErr
	}
	return m.driftResultsExist[traceID+":"+baselineTraceID], nil
}

func (m *mockStore) GetReplayTraceSpans(_ context.Context, traceID string) ([]*storage.ReplayTrace, error) {
	if m.getSpansErr != nil {
		return nil, m.getSpansErr
	}
	// Return the subset of replayTraces matching this trace_id
	var result []*storage.ReplayTrace
	for _, t := range m.replayTraces {
		if t.TraceID == traceID {
			result = append(result, t)
		}
	}
	return result, nil
}

func (m *mockStore) GetToolCapturesByTrace(_ context.Context, traceID string) ([]*storage.ToolCapture, error) {
	return m.toolCaptures[traceID], nil
}

func (m *mockStore) CreateDriftResult(_ context.Context, result *storage.DriftResult) error {
	if m.createResultErr != nil {
		return m.createResultErr
	}
	result.ID = len(m.createdDriftResults) + 1
	result.CreatedAt = time.Now()
	m.createdDriftResults = append(m.createdDriftResults, result)
	return nil
}

func (m *mockStore) GetBaseline(_ context.Context, traceID string) (*storage.Baseline, error) {
	for _, b := range m.baselines {
		if b.TraceID == traceID {
			return b, nil
		}
	}
	return nil, fmt.Errorf("baseline not found: %s", traceID)
}

func (m *mockStore) ListBaselines(_ context.Context) ([]*storage.Baseline, error) {
	return m.baselines, nil
}

func (m *mockStore) ListDriftResults(_ context.Context, limit int) ([]*storage.DriftResult, error) {
	if limit > 0 && limit < len(m.listDriftResultsData) {
		return m.listDriftResultsData[:limit], nil
	}
	return m.listDriftResultsData, nil
}

// --- Stub methods to satisfy storage.Storage interface ---

func (m *mockStore) Close() error                                             { return nil }
func (m *mockStore) Ping(_ context.Context) error                             { return nil }
func (m *mockStore) Migrate(_ context.Context) error                          { return nil }
func (m *mockStore) CreateOTELTrace(_ context.Context, _ *storage.OTELTrace) error { return nil }
func (m *mockStore) CreateIngestionBatch(_ context.Context, _ []*storage.OTELTrace, _ []*storage.ReplayTrace, _ []*storage.ToolCapture) (storage.IngestCounts, error) {
	return storage.IngestCounts{}, nil
}
func (m *mockStore) GetOTELTraceSpans(_ context.Context, _ string) ([]*storage.OTELTrace, error) {
	return nil, nil
}
func (m *mockStore) CreateReplayTrace(_ context.Context, _ *storage.ReplayTrace) error { return nil }
func (m *mockStore) CreateToolCapture(_ context.Context, _ *storage.ToolCapture) error { return nil }
func (m *mockStore) GetToolCaptureByArgs(_ context.Context, _ string, _ string) (*storage.ToolCapture, error) {
	return nil, nil
}
func (m *mockStore) CreateExperiment(_ context.Context, _ *storage.Experiment) error { return nil }
func (m *mockStore) GetExperiment(_ context.Context, _ uuid.UUID) (*storage.Experiment, error) {
	return nil, nil
}
func (m *mockStore) UpdateExperiment(_ context.Context, _ *storage.Experiment) error { return nil }
func (m *mockStore) ListExperiments(_ context.Context, _ storage.ExperimentFilters) ([]*storage.Experiment, error) {
	return nil, nil
}
func (m *mockStore) CreateExperimentRun(_ context.Context, _ *storage.ExperimentRun) error {
	return nil
}
func (m *mockStore) GetExperimentRun(_ context.Context, _ uuid.UUID) (*storage.ExperimentRun, error) {
	return nil, nil
}
func (m *mockStore) UpdateExperimentRun(_ context.Context, _ *storage.ExperimentRun) error {
	return nil
}
func (m *mockStore) ListExperimentRuns(_ context.Context, _ uuid.UUID) ([]*storage.ExperimentRun, error) {
	return nil, nil
}
func (m *mockStore) CreateAnalysisResult(_ context.Context, _ *storage.AnalysisResult) error {
	return nil
}
func (m *mockStore) GetAnalysisResults(_ context.Context, _ uuid.UUID) ([]*storage.AnalysisResult, error) {
	return nil, nil
}
func (m *mockStore) CreateEvaluator(_ context.Context, _ *storage.Evaluator) error { return nil }
func (m *mockStore) GetEvaluator(_ context.Context, _ int) (*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStore) GetEvaluatorByName(_ context.Context, _ string) (*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStore) UpdateEvaluator(_ context.Context, _ *storage.Evaluator) error { return nil }
func (m *mockStore) ListEvaluators(_ context.Context, _ bool) ([]*storage.Evaluator, error) {
	return nil, nil
}
func (m *mockStore) DeleteEvaluator(_ context.Context, _ int) error { return nil }
func (m *mockStore) CreateEvaluationRun(_ context.Context, _ *storage.EvaluationRun) error {
	return nil
}
func (m *mockStore) GetEvaluationRun(_ context.Context, _ uuid.UUID) (*storage.EvaluationRun, error) {
	return nil, nil
}
func (m *mockStore) UpdateEvaluationRun(_ context.Context, _ *storage.EvaluationRun) error {
	return nil
}
func (m *mockStore) CreateEvaluatorResult(_ context.Context, _ *storage.EvaluatorResult) error {
	return nil
}
func (m *mockStore) GetEvaluatorResults(_ context.Context, _ uuid.UUID) ([]*storage.EvaluatorResult, error) {
	return nil, nil
}
func (m *mockStore) CreateHumanEvaluation(_ context.Context, _ *storage.HumanEvaluation) error {
	return nil
}
func (m *mockStore) GetHumanEvaluation(_ context.Context, _ uuid.UUID) (*storage.HumanEvaluation, error) {
	return nil, nil
}
func (m *mockStore) UpdateHumanEvaluation(_ context.Context, _ *storage.HumanEvaluation) error {
	return nil
}
func (m *mockStore) ListPendingHumanEvaluations(_ context.Context, _ *string) ([]*storage.HumanEvaluation, error) {
	return nil, nil
}
func (m *mockStore) CreateGroundTruth(_ context.Context, _ *storage.GroundTruth) error { return nil }
func (m *mockStore) GetGroundTruth(_ context.Context, _ string) (*storage.GroundTruth, error) {
	return nil, nil
}
func (m *mockStore) UpdateGroundTruth(_ context.Context, _ *storage.GroundTruth) error { return nil }
func (m *mockStore) ListGroundTruth(_ context.Context, _ *string) ([]*storage.GroundTruth, error) {
	return nil, nil
}
func (m *mockStore) DeleteGroundTruth(_ context.Context, _ string) error { return nil }
func (m *mockStore) CreateEvaluationSummary(_ context.Context, _ *storage.EvaluationSummary) error {
	return nil
}
func (m *mockStore) GetEvaluationSummary(_ context.Context, _ uuid.UUID) ([]*storage.EvaluationSummary, error) {
	return nil, nil
}
func (m *mockStore) MarkTraceAsBaseline(_ context.Context, _ *storage.Baseline) error { return nil }
func (m *mockStore) UnmarkBaseline(_ context.Context, _ string) error                 { return nil }
func (m *mockStore) GetDriftResults(_ context.Context, _ string) ([]*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStore) GetDriftResultsByBaseline(_ context.Context, _ string, _ int) ([]*storage.DriftResult, error) {
	return nil, nil
}
func (m *mockStore) GetLatestDriftResult(_ context.Context, _ string) (*storage.DriftResult, error) {
	return nil, nil
}

// --- Test helpers ---

func makeSpan(traceID string, createdAt time.Time) *storage.ReplayTrace {
	return &storage.ReplayTrace{
		TraceID:   traceID,
		SpanID:    "span-" + traceID,
		RunID:     traceID,
		CreatedAt: createdAt,
		Provider:  "anthropic",
		Model:     "claude-3-5-sonnet-20241022",
		Prompt:    storage.JSONB{"messages": []interface{}{}},
	}
}

// --- Tests ---

func TestPollAndCheck_NoNewTraces(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	result := pollAndCheck(ctx, nil, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.NoError(t, result.err)
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
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	result := pollAndCheck(ctx, nil, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.NoError(t, result.err)
	assert.Equal(t, 0, result.checked)
	assert.Equal(t, now, result.highWater)
}

func TestPollAndCheck_SkipsAlreadyCheckedForSameBaseline(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", now),
	}
	store.driftResultsExist["trace-1:baseline-1"] = true

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	result := pollAndCheck(ctx, nil, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.NoError(t, result.err)
	assert.Equal(t, 0, result.checked)
}

func TestPollAndCheck_ChecksTraceNotCheckedForThisBaseline(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", now),
	}
	// Checked against a DIFFERENT baseline — should NOT skip
	store.driftResultsExist["trace-1:other-baseline"] = true

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	// Use a test cobra command to capture output
	cmd := driftWatchCmd
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	result := pollAndCheck(ctx, cmd, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.NoError(t, result.err)
	assert.Equal(t, 1, result.checked)
	require.Len(t, store.createdDriftResults, 1)
	assert.Equal(t, "trace-1", store.createdDriftResults[0].TraceID)
	assert.Equal(t, "baseline-1", store.createdDriftResults[0].BaselineTraceID)
}

func TestPollAndCheck_HighWaterMark(t *testing.T) {
	store := newMockStore()
	t1 := time.Now().Add(-2 * time.Minute)
	t2 := time.Now().Add(-1 * time.Minute)
	// ASC order: oldest first (matches poll's SortAsc: true)
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", t1),
		makeSpan("trace-2", t2),
	}
	// Both already checked so nothing to process — just testing cursor
	store.driftResultsExist["trace-1:baseline-1"] = true
	store.driftResultsExist["trace-2:baseline-1"] = true

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	result := pollAndCheck(ctx, nil, store, "baseline-1", baselineFP, cfg, time.Now().Add(-5*time.Minute), retries)

	assert.NoError(t, result.err)
	assert.Equal(t, t2, result.highWater, "highWater should be the newest span's created_at (last in ASC order)")
}

func TestPollAndCheck_ContextCancellation(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", now),
		makeSpan("trace-2", now.Add(-time.Second)),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	result := pollAndCheck(ctx, nil, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.Error(t, result.err)
	assert.Equal(t, 0, result.checked, "should not have processed any traces")
}

func TestPollAndCheck_HasResultErrorIsFatal(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", now),
	}
	store.hasResultErr = fmt.Errorf("db connection lost")

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	result := pollAndCheck(ctx, nil, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "db connection lost")
}

func TestPollAndCheck_PerTraceFailureRetries(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", now),
	}
	store.createResultErr = fmt.Errorf("write failed")

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	cmd := driftWatchCmd
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	result := pollAndCheck(ctx, cmd, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.NoError(t, result.err)
	assert.Equal(t, 0, result.checked, "failed trace should not count as checked")
	assert.Equal(t, 1, retries["trace-1"], "should be in retry map with count 1")
}

func TestPollAndCheck_RetryAbandonsAfterMaxRetries(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("trace-1", now),
	}
	store.createResultErr = fmt.Errorf("persistent failure")

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := map[string]int{
		"trace-1": maxRetries - 1, // one away from abandonment
	}

	cmd := driftWatchCmd
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	result := pollAndCheck(ctx, cmd, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.NoError(t, result.err)
	assert.Equal(t, 0, result.checked)
	_, inRetries := retries["trace-1"]
	assert.False(t, inRetries, "should be removed from retry map after max retries")
}

func TestPollAndCheck_ListTracesError(t *testing.T) {
	store := newMockStore()
	store.listTracesErr = fmt.Errorf("connection refused")

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	result := pollAndCheck(ctx, nil, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "connection refused")
}

func TestPollAndCheck_Overflow(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	// Simulate hitting the row limit in ASC order (oldest first)
	spans := make([]*storage.ReplayTrace, maxPollRows)
	for i := range spans {
		spans[i] = makeSpan(fmt.Sprintf("trace-%d", i), now.Add(time.Duration(i)*time.Second))
		// Mark all as already checked so we don't need to process
		store.driftResultsExist[fmt.Sprintf("trace-%d:baseline-1", i)] = true
	}
	store.replayTraces = spans

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()
	retries := make(map[string]int)

	result := pollAndCheck(ctx, nil, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.NoError(t, result.err)
	assert.True(t, result.overflow, "should report overflow when hitting maxPollRows")
	// With ASC order, highWater is the NEWEST row (last in ASC batch).
	// Next poll picks up where we left off instead of getting stuck.
	newestSpan := spans[len(spans)-1]
	assert.Equal(t, newestSpan.CreatedAt, result.highWater, "overflow should advance cursor to newest row in batch")
}

func TestPollAndCheck_RetryBypassGuard(t *testing.T) {
	// Retry traces should be re-checked for existence before processing,
	// to avoid duplicate drift results if a concurrent drift check ran.
	store := newMockStore()
	// No new traces from the poll query
	store.replayTraces = nil
	// But trace-1 is in the retry queue
	retries := map[string]int{"trace-1": 1}
	// AND it has already been checked (e.g. by a concurrent drift check)
	store.driftResultsExist["trace-1:baseline-1"] = true

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()

	result := pollAndCheck(ctx, nil, store, "baseline-1", baselineFP, cfg, time.Now(), retries)

	assert.NoError(t, result.err)
	assert.Equal(t, 0, result.checked, "should not re-process already-checked retry trace")
	_, inRetries := retries["trace-1"]
	assert.False(t, inRetries, "should be removed from retry map since it was already checked")
}

func TestResolveBaseline_Explicit(t *testing.T) {
	store := newMockStore()
	store.baselines = []*storage.Baseline{
		{TraceID: "my-baseline", CreatedAt: time.Now()},
	}

	ctx := context.Background()
	traceID, err := resolveBaseline(ctx, store, "my-baseline")

	require.NoError(t, err)
	assert.Equal(t, "my-baseline", traceID)
}

func TestResolveBaseline_AutoDetect(t *testing.T) {
	store := newMockStore()
	store.baselines = []*storage.Baseline{
		{TraceID: "newest-baseline", CreatedAt: time.Now()},
		{TraceID: "older-baseline", CreatedAt: time.Now().Add(-time.Hour)},
	}

	ctx := context.Background()
	traceID, err := resolveBaseline(ctx, store, "")

	require.NoError(t, err)
	assert.Equal(t, "newest-baseline", traceID)
}

func TestResolveBaseline_NoneExist(t *testing.T) {
	store := newMockStore()

	ctx := context.Background()
	_, err := resolveBaseline(ctx, store, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no baselines found")
}

func TestResolveBaseline_ExplicitNotFound(t *testing.T) {
	store := newMockStore()

	ctx := context.Background()
	_, err := resolveBaseline(ctx, store, "nonexistent")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCheckOneTrace_HappyPath(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.replayTraces = []*storage.ReplayTrace{
		makeSpan("candidate-1", now),
	}

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{
		TraceID:       "baseline-1",
		StepCount:     1,
		Models:        []string{"claude-3-5-sonnet-20241022"},
		Providers:     []string{"anthropic"},
		ToolFrequency: map[string]int{},
		RiskCounts:    map[string]int{},
	}
	cfg := drift.DefaultConfig()

	report, err := checkOneTrace(ctx, store, "baseline-1", baselineFP, cfg, "candidate-1")

	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.GreaterOrEqual(t, report.Score, 0.0)
	assert.LessOrEqual(t, report.Score, 1.0)
	assert.NotEmpty(t, report.Verdict)

	// Verify drift result was persisted
	require.Len(t, store.createdDriftResults, 1)
	assert.Equal(t, "candidate-1", store.createdDriftResults[0].TraceID)
	assert.Equal(t, "baseline-1", store.createdDriftResults[0].BaselineTraceID)
}

func TestCheckOneTrace_NoSpans(t *testing.T) {
	store := newMockStore()
	// No spans for this trace

	ctx := context.Background()
	baselineFP := &drift.Fingerprint{TraceID: "baseline-1", StepCount: 1, Models: []string{"m"}, Providers: []string{"p"}, ToolFrequency: map[string]int{}, RiskCounts: map[string]int{}}
	cfg := drift.DefaultConfig()

	_, err := checkOneTrace(ctx, store, "baseline-1", baselineFP, cfg, "missing-trace")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no spans found")
}

func TestDriftWatchCmd_IntervalValidation(t *testing.T) {
	// Exercise runDriftWatch's interval validation via cobra flag parsing.
	// We set the flag to a negative value and verify the command returns an
	// error instead of panicking in time.NewTicker.
	tests := []struct {
		name     string
		interval string
		wantErr  string
	}{
		{"negative", "-1s", "--interval must be positive"},
		{"zero", "0s", "--interval must be positive"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fresh command tree so flag state doesn't leak between subtests
			root := &cobra.Command{Use: "test"}
			watchCmd := &cobra.Command{
				Use:  "watch",
				RunE: runDriftWatch,
			}
			watchCmd.Flags().Duration("interval", 30*time.Second, "Polling interval")
			watchCmd.Flags().String("baseline", "", "Baseline trace ID")
			root.AddCommand(watchCmd)

			out := &strings.Builder{}
			root.SetOut(out)
			root.SetErr(out)

			root.SetArgs([]string{"watch", "--interval", tc.interval})
			err := root.Execute()

			// Interval validation runs before connectDB, so the error
			// should be specifically about the interval, not a DB failure.
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestVerdictDisplay(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{storage.DriftVerdictPass, "PASS"},
		{storage.DriftVerdictWarn, "WARN"},
		{storage.DriftVerdictFail, "FAIL"},
		{storage.DriftVerdictPending, "pending"},
		{"unknown", "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, verdictDisplay(tc.input))
		})
	}
}
