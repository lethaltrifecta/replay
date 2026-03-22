package replay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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
	Model          string            `json:"model"`
	Provider       string            `json:"provider,omitempty"`
	Temperature    *float64          `json:"temperature,omitempty"`
	TopP           *float64          `json:"top_p,omitempty"`
	MaxTokens      *int              `json:"max_tokens,omitempty"`
	RequestHeaders map[string]string `json:"request_headers,omitempty"`
}

// PreparedRun holds the state created by Setup, ready for Execute.
type PreparedRun struct {
	ExperimentID  uuid.UUID
	BaselineRunID uuid.UUID
	VariantRunID  uuid.UUID
	BaselineSteps []*storage.ReplayTrace
	Experiment    *storage.Experiment
	VariantRun    *storage.ExperimentRun
	VariantConfig VariantConfig
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
// This is the synchronous all-in-one entry point (calls Setup then Execute).
func (e *Engine) Run(ctx context.Context, baselineTraceID string, variant VariantConfig, threshold float64) (*Result, error) {
	prepared, err := e.Setup(ctx, baselineTraceID, variant, threshold)
	if err != nil {
		return nil, err
	}
	return e.Execute(ctx, prepared)
}

// Setup loads the baseline, creates Experiment + ExperimentRun records, and returns
// a PreparedRun that can be passed to Execute. This is the synchronous first phase
// that produces an experiment ID before the (potentially long-running) replay loop.
func (e *Engine) Setup(ctx context.Context, baselineTraceID string, variant VariantConfig, threshold float64) (*PreparedRun, error) {
	// Load baseline steps
	baselineSteps, err := e.store.GetReplayTraceSpans(ctx, baselineTraceID)
	if err != nil {
		return nil, fmt.Errorf("load baseline: %w", err)
	}
	if len(baselineSteps) == 0 {
		return nil, storage.ErrTraceNotFound
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
		Config: storage.ExperimentConfig{
			Model:          variant.Model,
			Provider:       variant.Provider,
			Temperature:    variant.Temperature,
			TopP:           variant.TopP,
			MaxTokens:      variant.MaxTokens,
			RequestHeaders: maps.Clone(variant.RequestHeaders),
			Threshold:      &threshold,
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
		VariantConfig: storage.VariantConfig{}, // structured default
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
	variantRun := &storage.ExperimentRun{
		ID:           variantRunID,
		ExperimentID: experimentID,
		RunType:      storage.RunTypeVariant,
		VariantConfig: storage.VariantConfig{
			Model:          variant.Model,
			Provider:       variant.Provider,
			Temperature:    variant.Temperature,
			TopP:           variant.TopP,
			MaxTokens:      variant.MaxTokens,
			RequestHeaders: variant.RequestHeaders,
		},
		Status:    storage.StatusRunning,
		CreatedAt: now,
	}
	if err := e.store.CreateExperimentRun(ctx, variantRun); err != nil {
		if cleanupErr := e.markExperimentFailed(exp); cleanupErr != nil {
			return nil, fmt.Errorf("create variant run: %w (cleanup: %v)", err, cleanupErr)
		}
		return nil, fmt.Errorf("create variant run: %w", err)
	}

	return &PreparedRun{
		ExperimentID:  experimentID,
		BaselineRunID: baselineRunID,
		VariantRunID:  variantRunID,
		BaselineSteps: baselineSteps,
		Experiment:    exp,
		VariantRun:    variantRun,
		VariantConfig: variant,
	}, nil
}

// Execute runs the replay loop using a previously prepared run.
// It sends each baseline prompt to the variant model, persists the results,
// and finalizes the experiment status.
func (e *Engine) Execute(ctx context.Context, prepared *PreparedRun) (*Result, error) {
	result := &Result{
		ExperimentID:  prepared.ExperimentID,
		BaselineRunID: prepared.BaselineRunID,
		VariantRunID:  prepared.VariantRunID,
	}

	// Generate variant trace ID
	variantTraceID := uuid.New().String()
	result.VariantTraceID = variantTraceID

	exp := prepared.Experiment
	variantRun := prepared.VariantRun
	variant := prepared.VariantConfig

	// Replay each baseline step
	totalSteps := len(prepared.BaselineSteps)
	for i, step := range prepared.BaselineSteps {
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
	tools, err := extractTools(baseline.Prompt)
	if err != nil {
		return nil, fmt.Errorf("extract tools: %w", err)
	}
	toolChoice, err := extractToolChoice(baseline.Prompt)
	if err != nil {
		return nil, fmt.Errorf("extract tool choice: %w", err)
	}

	req := &agwclient.CompletionRequest{
		Model:       variant.Model,
		Messages:    messages,
		Tools:       tools,
		ToolChoice:  toolChoice,
		Temperature: variant.Temperature,
		TopP:        variant.TopP,
		MaxTokens:   variant.MaxTokens,
		Headers:     variant.RequestHeaders,
	}

	start := time.Now()
	resp, err := e.client.Complete(ctx, req)
	latencyMS := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, err
	}

	completion := ""
	metadata := storage.JSONB{
		"source":              "replay",
		"baseline_trace_id":   baseline.TraceID,
		"baseline_step_index": baseline.StepIndex,
	}
	if len(tools) > 0 {
		metadata["replay_tool_names"] = toolNamesFromDefinitions(tools)
	}
	if toolChoice != nil {
		metadata["replay_tool_choice"] = toolChoice
	}
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
func extractMessages(prompt storage.JSONB) ([]agwclient.ChatMessage, error) {
	raw, ok := prompt["messages"]
	if !ok {
		return nil, fmt.Errorf("prompt missing 'messages' key")
	}

	slice, err := decodeJSONValue[[]map[string]interface{}](raw)
	if err != nil {
		return nil, fmt.Errorf("'messages' is not a valid slice: %w", err)
	}

	msgs := make([]agwclient.ChatMessage, 0, len(slice))
	for i, item := range slice {
		role := stringValue(item["role"])
		if role == "" {
			return nil, fmt.Errorf("message %d missing required 'role' field", i)
		}
		msg := agwclient.ChatMessage{
			Role:       role,
			Content:    contentToString(item["content"]),
			Name:       stringValue(item["name"]),
			ToolCallID: stringValue(item["tool_call_id"]),
		}
		if rawToolCalls, ok := item["tool_calls"]; ok {
			toolCalls, err := decodeJSONValue[[]agwclient.ToolCallResponse](rawToolCalls)
			if err != nil {
				return nil, fmt.Errorf("decode message %d tool_calls: %w", i, err)
			}
			msg.ToolCalls = toolCalls
		}
		msgs = append(msgs, msg)
	}

	if len(msgs) == 0 {
		return nil, fmt.Errorf("no valid messages in prompt")
	}

	return msgs, nil
}

func extractTools(prompt storage.JSONB) ([]agwclient.ToolDefinition, error) {
	raw, ok := prompt["tools"]
	if !ok {
		return nil, nil
	}

	tools, err := decodeJSONValue[[]agwclient.ToolDefinition](raw)
	if err != nil {
		return nil, fmt.Errorf("prompt tools: %w", err)
	}
	if len(tools) == 0 {
		return nil, nil
	}
	return tools, nil
}

func extractToolChoice(prompt storage.JSONB) (interface{}, error) {
	raw, ok := prompt["tool_choice"]
	if !ok {
		return nil, nil
	}

	choice, err := normalizeJSONValue(raw)
	if err != nil {
		return nil, fmt.Errorf("prompt tool_choice: %w", err)
	}
	return choice, nil
}

func decodeJSONValue[T any](raw interface{}) (T, error) {
	var out T
	body, err := json.Marshal(raw)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, err
	}
	return out, nil
}

func normalizeJSONValue(raw interface{}) (interface{}, error) {
	return decodeJSONValue[interface{}](raw)
}

func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func contentToString(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	default:
		body, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(body)
	}
}

func toolNamesFromDefinitions(tools []agwclient.ToolDefinition) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.Function != nil && tool.Function.Name != "" {
			names = append(names, tool.Function.Name)
		}
	}
	return names
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

// FailPreparedRun marks the prepared variant run and experiment as failed.
// This is used by callers when post-replay pipeline steps fail after Execute returns.
func (e *Engine) FailPreparedRun(prepared *PreparedRun, runErr error) error {
	if prepared == nil || prepared.Experiment == nil || prepared.VariantRun == nil {
		return fmt.Errorf("prepared run is incomplete")
	}
	return e.failExperiment(prepared.Experiment, prepared.VariantRun, runErr)
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
