package replay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// Completer abstracts the LLM call for testability.
type Completer interface {
	Complete(ctx context.Context, req *agwclient.CompletionRequest) (*agwclient.CompletionResponse, error)
}

// VariantConfig describes the model configuration for the variant run.
type VariantConfig struct {
	Model       string   `json:"model"`
	Provider    string   `json:"provider,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
}

// Result holds the outcome of a replay run.
type Result struct {
	ExperimentID   uuid.UUID
	BaselineRunID  uuid.UUID
	VariantRunID   uuid.UUID
	VariantTraceID string
	Steps          []*storage.ReplayTrace
	Error          error
}

// Engine orchestrates replaying baseline prompts with a variant model.
type Engine struct {
	store  storage.Storage
	client Completer
}

// NewEngine creates a new replay engine.
func NewEngine(store storage.Storage, client Completer) *Engine {
	return &Engine{store: store, client: client}
}

// Run replays every step from baselineTraceID using the variant config.
// It creates Experiment and ExperimentRun records, and persists each variant ReplayTrace.
func (e *Engine) Run(ctx context.Context, baselineTraceID string, variant VariantConfig) (*Result, error) {
	// Load baseline steps
	baselineSteps, err := e.store.GetReplayTraceSpans(ctx, baselineTraceID)
	if err != nil {
		return nil, fmt.Errorf("load baseline: %w", err)
	}
	if len(baselineSteps) == 0 {
		return nil, fmt.Errorf("no replay traces found for baseline %s", baselineTraceID)
	}

	// Create experiment
	experimentID := uuid.New()
	now := time.Now()
	exp := &storage.Experiment{
		ID:              experimentID,
		Name:            fmt.Sprintf("gate-%s-%s", variant.Model, truncateID(baselineTraceID)),
		BaselineTraceID: baselineTraceID,
		Status:          storage.StatusRunning,
		Progress:        0,
		Config: storage.JSONB{
			"variant_model":    variant.Model,
			"variant_provider": variant.Provider,
		},
		CreatedAt: now,
	}
	if err := e.store.CreateExperiment(ctx, exp); err != nil {
		return nil, fmt.Errorf("create experiment: %w", err)
	}

	// Create baseline run (already completed — it's the captured trace)
	baselineRunID := uuid.New()
	baselineRun := &storage.ExperimentRun{
		ID:            baselineRunID,
		ExperimentID:  experimentID,
		RunType:       storage.RunTypeBaseline,
		VariantConfig: storage.JSONB{}, // NOT NULL in schema; baseline has no variant config
		TraceID:       &baselineTraceID,
		Status:        storage.StatusCompleted,
		CreatedAt:     now,
		CompletedAt:   &now,
	}
	if err := e.store.CreateExperimentRun(ctx, baselineRun); err != nil {
		if cleanupErr := e.markExperimentFailed(exp); cleanupErr != nil {
			return nil, fmt.Errorf("create baseline run: %w (cleanup: %v)", err, cleanupErr)
		}
		return nil, fmt.Errorf("create baseline run: %w", err)
	}

	// Create variant run (running)
	variantRunID := uuid.New()
	variantConfig := storage.JSONB{
		"model":    variant.Model,
		"provider": variant.Provider,
	}
	variantRun := &storage.ExperimentRun{
		ID:            variantRunID,
		ExperimentID:  experimentID,
		RunType:       storage.RunTypeVariant,
		VariantConfig: variantConfig,
		Status:        storage.StatusRunning,
		CreatedAt:     now,
	}
	if err := e.store.CreateExperimentRun(ctx, variantRun); err != nil {
		if cleanupErr := e.markExperimentFailed(exp); cleanupErr != nil {
			return nil, fmt.Errorf("create variant run: %w (cleanup: %v)", err, cleanupErr)
		}
		return nil, fmt.Errorf("create variant run: %w", err)
	}

	result := &Result{
		ExperimentID:  experimentID,
		BaselineRunID: baselineRunID,
		VariantRunID:  variantRunID,
	}

	// Generate variant trace ID
	variantTraceID := uuid.New().String()
	result.VariantTraceID = variantTraceID

	// Replay each baseline step
	totalSteps := len(baselineSteps)
	for i, step := range baselineSteps {
		if ctxErr := ctx.Err(); ctxErr != nil {
			cleanupErr := e.failExperiment(exp, variantRun, ctxErr)
			result.Error = ctxErr
			if cleanupErr != nil {
				return result, fmt.Errorf("%w (cleanup: %v)", ctxErr, cleanupErr)
			}
			return result, ctxErr
		}

		variantTrace, err := e.replayStep(ctx, step, i, variantTraceID, variant)
		if err != nil {
			cleanupErr := e.failExperiment(exp, variantRun, err)
			result.Error = err
			if cleanupErr != nil {
				return result, fmt.Errorf("replay step %d: %w (cleanup: %v)", i, err, cleanupErr)
			}
			return result, fmt.Errorf("replay step %d: %w", i, err)
		}

		if err := e.store.CreateReplayTrace(ctx, variantTrace); err != nil {
			cleanupErr := e.failExperiment(exp, variantRun, err)
			result.Error = err
			if cleanupErr != nil {
				return result, fmt.Errorf("store variant step %d: %w (cleanup: %v)", i, err, cleanupErr)
			}
			return result, fmt.Errorf("store variant step %d: %w", i, err)
		}

		result.Steps = append(result.Steps, variantTrace)

		// Update progress
		exp.Progress = float64(i+1) / float64(totalSteps)
		_ = e.store.UpdateExperiment(ctx, exp)
	}

	// Finalize: mark variant run completed
	if err := e.finalizeSuccess(ctx, exp, variantRun, variantTraceID); err != nil {
		return result, err
	}

	return result, nil
}

// replayStep sends a single baseline step's prompt to the variant model and returns the variant ReplayTrace.
func (e *Engine) replayStep(ctx context.Context, baseline *storage.ReplayTrace, stepIndex int, variantTraceID string, variant VariantConfig) (*storage.ReplayTrace, error) {
	messages, err := extractMessages(baseline.Prompt)
	if err != nil {
		return nil, fmt.Errorf("extract messages: %w", err)
	}

	req := &agwclient.CompletionRequest{
		Model:       variant.Model,
		Messages:    messages,
		Temperature: variant.Temperature,
		TopP:        variant.TopP,
		MaxTokens:   variant.MaxTokens,
	}

	start := time.Now()
	resp, err := e.client.Complete(ctx, req)
	latencyMS := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, err
	}

	completion := ""
	metadata := storage.JSONB{"source": "replay", "baseline_trace_id": baseline.TraceID}
	if len(resp.Choices) > 0 {
		completion = resp.Choices[0].Message.Content
		if len(resp.Choices[0].Message.ToolCalls) > 0 {
			metadata["tool_calls"] = resp.Choices[0].Message.ToolCalls
		}
	}

	return &storage.ReplayTrace{
		TraceID:          variantTraceID,
		SpanID:           uuid.New().String(),
		RunID:            variantTraceID,
		StepIndex:        stepIndex,
		CreatedAt:        time.Now(),
		Provider:         variant.Provider,
		Model:            resp.Model,
		Prompt:           baseline.Prompt,
		Completion:       completion,
		Parameters:       storage.JSONB{},
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
		LatencyMS:        latencyMS,
		Metadata:         metadata,
	}, nil
}

// extractMessages parses the prompt JSONB into ChatMessage slice.
// Expected format: {"messages": [{"role": "...", "content": "..."}]}
func extractMessages(prompt storage.JSONB) ([]agwclient.ChatMessage, error) {
	raw, ok := prompt["messages"]
	if !ok {
		return nil, fmt.Errorf("prompt missing 'messages' key")
	}

	slice, ok := raw.([]interface{})
	if !ok {
		// Try typed slice (from test mocks or direct construction)
		if typedSlice, ok := raw.([]map[string]string); ok {
			msgs := make([]agwclient.ChatMessage, len(typedSlice))
			for i, m := range typedSlice {
				msgs[i] = agwclient.ChatMessage{Role: m["role"], Content: m["content"]}
			}
			return msgs, nil
		}
		return nil, fmt.Errorf("'messages' is not a slice (type %T)", raw)
	}

	msgs := make([]agwclient.ChatMessage, 0, len(slice))
	for _, item := range slice {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		msgs = append(msgs, agwclient.ChatMessage{Role: role, Content: content})
	}

	if len(msgs) == 0 {
		return nil, fmt.Errorf("no valid messages in prompt")
	}

	return msgs, nil
}

// failExperiment marks the variant run and experiment as failed.
// Uses a fresh context so cleanup succeeds even if the original context is cancelled.
// Returns an error if the DB writes fail so callers can surface inconsistent state.
func (e *Engine) failExperiment(exp *storage.Experiment, run *storage.ExperimentRun, runErr error) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	errMsg := runErr.Error()
	run.Status = storage.StatusFailed
	run.Error = &errMsg
	run.CompletedAt = &now
	exp.Status = storage.StatusFailed
	exp.CompletedAt = &now

	var errs []error
	if err := e.store.UpdateExperimentRun(cleanupCtx, run); err != nil {
		errs = append(errs, fmt.Errorf("mark run failed: %w", err))
	}
	if err := e.store.UpdateExperiment(cleanupCtx, exp); err != nil {
		errs = append(errs, fmt.Errorf("mark experiment failed: %w", err))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// markExperimentFailed marks only the experiment as failed (used when run creation itself fails).
// Returns an error if the DB write fails so callers can surface inconsistent state.
func (e *Engine) markExperimentFailed(exp *storage.Experiment) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	exp.Status = storage.StatusFailed
	exp.CompletedAt = &now
	if err := e.store.UpdateExperiment(cleanupCtx, exp); err != nil {
		return fmt.Errorf("mark experiment failed: %w", err)
	}
	return nil
}

// finalizeSuccess attempts to mark both run and experiment as completed.
// If either write fails, it tries to mark both records failed to preserve terminal consistency.
func (e *Engine) finalizeSuccess(ctx context.Context, exp *storage.Experiment, run *storage.ExperimentRun, traceID string) error {
	completedAt := time.Now()

	run.TraceID = &traceID
	run.Status = storage.StatusCompleted
	run.CompletedAt = &completedAt

	exp.Status = storage.StatusCompleted
	exp.Progress = 1.0
	exp.CompletedAt = &completedAt

	var finalizeErrs []error
	if err := e.store.UpdateExperimentRun(ctx, run); err != nil {
		finalizeErrs = append(finalizeErrs, fmt.Errorf("finalize variant run: %w", err))
	}
	if err := e.store.UpdateExperiment(ctx, exp); err != nil {
		finalizeErrs = append(finalizeErrs, fmt.Errorf("finalize experiment: %w", err))
	}

	if len(finalizeErrs) == 0 {
		return nil
	}

	finalizeErr := errors.Join(finalizeErrs...)
	cleanupErr := e.failExperiment(exp, run, finalizeErr)
	if cleanupErr != nil {
		return fmt.Errorf("finalize lifecycle: %w (cleanup: %v)", finalizeErr, cleanupErr)
	}

	return fmt.Errorf("finalize lifecycle: %w", finalizeErr)
}

// truncateID returns at most 8 characters of an ID string.
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
