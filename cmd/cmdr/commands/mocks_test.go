package commands

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// --- Mock Store ---

type mockStore struct {
	mu                  sync.Mutex
	baselines           []*storage.Baseline
	replayTraces        []*storage.ReplayTrace
	createdDriftResults []*storage.DriftResult
	driftResultsExist   map[string]bool
	listTracesErr       error
	hasResultErr        error
	createResultErr     error
	toolCaptures        map[string][]*storage.ToolCapture
}

func newMockStore() *mockStore {
	return &mockStore{
		driftResultsExist: make(map[string]bool),
		toolCaptures:      make(map[string][]*storage.ToolCapture),
	}
}

func (m *mockStore) ListBaselines(_ context.Context) ([]*storage.Baseline, error) {
	return m.baselines, nil
}

func (m *mockStore) ListDriftResults(_ context.Context, limit int, offset int) ([]*storage.DriftResult, error) {
	return nil, nil
}

func (m *mockStore) GetBaseline(_ context.Context, traceID string) (*storage.Baseline, error) {
	for _, b := range m.baselines {
		if b.TraceID == traceID {
			return b, nil
		}
	}
	return nil, storage.ErrBaselineNotFound
}

func (m *mockStore) GetReplayTraceSpans(_ context.Context, traceID string) ([]*storage.ReplayTrace, error) {
	var results []*storage.ReplayTrace
	for _, t := range m.replayTraces {
		if t.TraceID == traceID {
			results = append(results, t)
		}
	}
	return results, nil
}

func (m *mockStore) GetToolCapturesByTrace(_ context.Context, traceID string) ([]*storage.ToolCapture, error) {
	return m.toolCaptures[traceID], nil
}

func (m *mockStore) CreateDriftResult(_ context.Context, r *storage.DriftResult) error {
	if m.createResultErr != nil {
		return m.createResultErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createdDriftResults = append(m.createdDriftResults, r)
	return nil
}

func (m *mockStore) HasDriftResultForBaseline(_ context.Context, traceID string, baselineTraceID string) (bool, error) {
	if m.hasResultErr != nil {
		return false, m.hasResultErr
	}
	return m.driftResultsExist[traceID+":"+baselineTraceID], nil
}

func (m *mockStore) ListReplayTraces(_ context.Context, filters storage.TraceFilters) ([]*storage.ReplayTrace, error) {
	if m.listTracesErr != nil {
		return nil, m.listTracesErr
	}

	var traces []*storage.ReplayTrace
	for _, trace := range m.replayTraces {
		if filters.Model != nil && trace.Model != *filters.Model {
			continue
		}
		if filters.Provider != nil && trace.Provider != *filters.Provider {
			continue
		}
		if filters.StartTime != nil && trace.CreatedAt.Before(*filters.StartTime) {
			continue
		}
		if filters.EndTime != nil && trace.CreatedAt.After(*filters.EndTime) {
			continue
		}
		traces = append(traces, trace)
	}

	sort.Slice(traces, func(i, j int) bool {
		if traces[i].CreatedAt.Equal(traces[j].CreatedAt) {
			if filters.SortAsc {
				return traces[i].TraceID < traces[j].TraceID
			}
			return traces[i].TraceID > traces[j].TraceID
		}
		if filters.SortAsc {
			return traces[i].CreatedAt.Before(traces[j].CreatedAt)
		}
		return traces[i].CreatedAt.After(traces[j].CreatedAt)
	})

	if filters.Offset >= len(traces) {
		return nil, nil
	}
	if filters.Offset > 0 {
		traces = traces[filters.Offset:]
	}
	if filters.Limit > 0 && filters.Limit < len(traces) {
		traces = traces[:filters.Limit]
	}

	return traces, nil
}

func (m *mockStore) ListUniqueTraces(_ context.Context, _ storage.TraceFilters) ([]*storage.TraceSummary, error) {
	return nil, nil
}

// Stub remaining methods
func (m *mockStore) Close() error                                                  { return nil }
func (m *mockStore) Ping(_ context.Context) error                                  { return nil }
func (m *mockStore) Migrate(_ context.Context) error                               { return nil }
func (m *mockStore) CreateOTELTrace(_ context.Context, _ *storage.OTELTrace) error { return nil }
func (m *mockStore) GetOTELTraceSpans(_ context.Context, _ string) ([]*storage.OTELTrace, error) {
	return nil, nil
}
func (m *mockStore) CreateReplayTrace(_ context.Context, _ *storage.ReplayTrace) error { return nil }
func (m *mockStore) CreateIngestionBatch(_ context.Context, _ []*storage.OTELTrace, _ []*storage.ReplayTrace, _ []*storage.ToolCapture) (storage.IngestCounts, error) {
	return storage.IngestCounts{}, nil
}
func (m *mockStore) CreateToolCapture(_ context.Context, _ *storage.ToolCapture) error { return nil }
func (m *mockStore) GetToolCaptureByArgs(_ context.Context, _ string, _ string) (*storage.ToolCapture, error) {
	return nil, nil
}
func (m *mockStore) GetExperiment(_ context.Context, _ uuid.UUID) (*storage.Experiment, error) {
	return nil, nil
}
func (m *mockStore) CreateExperiment(_ context.Context, _ *storage.Experiment) error { return nil }
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
func (m *mockStore) GetLatestAnalysisResults(_ context.Context, _ []uuid.UUID) (map[uuid.UUID]*storage.AnalysisResult, error) {
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
func (m *mockStore) GetDriftResultForPair(_ context.Context, _ string, _ string) (*storage.DriftResult, error) {
	return nil, nil
}
