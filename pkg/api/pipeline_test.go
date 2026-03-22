package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/replay"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

func TestRunGatePipeline_AnalysisPersistFailureMarksVariantRunFailed(t *testing.T) {
	store := newMockStorage()
	seedBaseline(store, "baseline-1", 1)
	completer := &mockCompleter{}
	srv := newTestServer(t, store, completer, 1)

	engine := replay.NewEngine(store, completer)
	prepared, err := engine.Setup(context.Background(), "baseline-1", replay.VariantConfig{
		Model:    "gpt-4o",
		Provider: "openai",
	}, 0.8)
	require.NoError(t, err)

	store.createAnalysisErr = errors.New("analysis write failed")

	RunGatePipeline(context.Background(), store, engine, prepared, 0.8, srv.log)

	exp, err := store.GetExperiment(context.Background(), prepared.ExperimentID)
	require.NoError(t, err)
	assert.Equal(t, storage.StatusFailed, exp.Status)

	runs, err := store.ListExperimentRuns(context.Background(), prepared.ExperimentID)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	assert.Equal(t, storage.StatusCompleted, runs[0].Status)
	assert.Equal(t, storage.StatusFailed, runs[1].Status)
	require.NotNil(t, runs[1].Error)
	assert.Contains(t, *runs[1].Error, "persist analysis")
}
