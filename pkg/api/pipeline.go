package api

import (
	"context"
	"fmt"

	"github.com/lethaltrifecta/replay/pkg/diff"
	"github.com/lethaltrifecta/replay/pkg/replay"
	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/lethaltrifecta/replay/pkg/utils/logger"
)

// RunGatePipeline executes the replay loop, diffs results, and persists the analysis.
// It is designed to run in a background goroutine after Setup has returned an experiment ID.
// On failure it marks the experiment as failed via the engine's internal cleanup.
func RunGatePipeline(ctx context.Context, store storage.Storage, engine *replay.Engine,
	prepared *replay.PreparedRun, threshold float64, log *logger.Logger) {

	result, err := engine.Execute(ctx, prepared)
	if err != nil {
		log.Errorw("replay execution failed",
			"experiment_id", prepared.ExperimentID,
			"error", err,
		)
		return // Execute already marks experiment failed
	}

	// Reload traces for diff comparison
	baselineSteps, err := store.GetReplayTraceSpans(ctx, prepared.Experiment.BaselineTraceID)
	if err != nil {
		log.Errorw("reload baseline for diff failed",
			"experiment_id", prepared.ExperimentID,
			"error", err,
		)
		markPipelineFailed(engine, prepared, fmt.Errorf("reload baseline: %w", err), log)
		return
	}

	variantSteps, err := store.GetReplayTraceSpans(ctx, result.VariantTraceID)
	if err != nil {
		log.Errorw("reload variant for diff failed",
			"experiment_id", prepared.ExperimentID,
			"error", err,
		)
		markPipelineFailed(engine, prepared, fmt.Errorf("reload variant: %w", err), log)
		return
	}

	// Load baseline tool captures (graceful fallback)
	var baselineCaptures []*storage.ToolCapture
	var variantToolCalls []diff.ToolCall
	captures, captureErr := store.GetToolCapturesByTrace(ctx, prepared.Experiment.BaselineTraceID)
	if captureErr != nil {
		log.Warnw("failed to load baseline tool captures, falling back to 4-dimension diff",
			"experiment_id", prepared.ExperimentID,
			"error", captureErr,
		)
	} else {
		baselineCaptures = captures
		variantToolCalls = diff.ExtractVariantToolCalls(variantSteps)
	}

	// Diff
	diffCfg := diff.Config{SimilarityThreshold: threshold}
	report := diff.CompareAll(diff.CompareInput{
		Baseline:      baselineSteps,
		Variant:       variantSteps,
		BaselineTools: baselineCaptures,
		VariantTools:  variantToolCalls,
	}, diffCfg)

	// Persist analysis result
	analysisResult := diff.ToAnalysisResult(report, prepared.ExperimentID, prepared.BaselineRunID, prepared.VariantRunID)
	if err := store.CreateAnalysisResult(ctx, analysisResult); err != nil {
		log.Errorw("failed to persist analysis result",
			"experiment_id", prepared.ExperimentID,
			"error", err,
		)
		markPipelineFailed(engine, prepared, fmt.Errorf("persist analysis: %w", err), log)
		return
	}

	log.Infow("gate pipeline completed",
		"experiment_id", prepared.ExperimentID,
		"verdict", report.Verdict,
		"similarity", report.SimilarityScore,
	)
}

// markPipelineFailed marks both the experiment and variant run as failed after Execute has returned.
func markPipelineFailed(engine *replay.Engine, prepared *replay.PreparedRun, pipelineErr error, log *logger.Logger) {
	if err := engine.FailPreparedRun(prepared, pipelineErr); err != nil {
		log.Errorw("failed to mark pipeline run as failed",
			"experiment_id", prepared.ExperimentID,
			"pipeline_error", pipelineErr,
			"update_error", err,
		)
	}
}
