package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
	"github.com/lethaltrifecta/replay/pkg/otelreceiver"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

// DefaultMaxTurns is the default limit for agent loop iterations.
const DefaultMaxTurns = 8

// StopReason constants for AgentLoopResult.
const (
	StopReasonNoToolCalls = "no_tool_calls"
	StopReasonDivergence  = "divergence"
	StopReasonMaxTurns    = "max_turns"
)

// AgentLoopConfig holds configuration for the agent loop.
type AgentLoopConfig struct {
	MaxTurns int
}

// DivergenceEvent records a point where the variant diverged from the baseline.
type DivergenceEvent struct {
	Turn      int    `json:"turn"`
	ToolName  string `json:"tool_name"`
	ErrorType string `json:"error_type"`
	Message   string `json:"message"`
}

// AgentLoopResult extends Result with agent-loop-specific metadata.
type AgentLoopResult struct {
	Result
	TurnsExecuted int               `json:"turns_executed"`
	Divergences   []DivergenceEvent `json:"divergences,omitempty"`
	StopReason    string            `json:"stop_reason"`
}

// ExecuteAgentLoop runs a real agent conversation: seed from step 0, send to variant LLM,
// execute tool calls via the ToolExecutor (freeze-mcp), and repeat until the LLM stops
// calling tools, a divergence occurs, or max turns is reached.
func (e *Engine) ExecuteAgentLoop(ctx context.Context, prepared *PreparedRun, toolExec ToolExecutor, loopCfg AgentLoopConfig) (*AgentLoopResult, error) {
	if len(prepared.BaselineSteps) == 0 {
		return nil, storage.ErrTraceNotFound
	}

	maxTurns := loopCfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}

	// Pre-load baseline captures for per-call locator matching (non-fatal on failure).
	var captureQ *captureQueue
	if caps, err := e.store.GetToolCapturesByTrace(ctx, prepared.Experiment.BaselineTraceID); err == nil && len(caps) > 0 {
		captureQ = &captureQueue{captures: caps, used: make([]bool, len(caps))}
	}

	variantTraceID := uuid.New().String()
	exp := prepared.Experiment
	variantRun := prepared.VariantRun
	variant := prepared.VariantConfig

	result := &AgentLoopResult{
		Result: Result{
			ExperimentID:   prepared.ExperimentID,
			BaselineRunID:  prepared.BaselineRunID,
			VariantRunID:   prepared.VariantRunID,
			VariantTraceID: variantTraceID,
		},
	}

	// Extract initial messages from baseline step 0
	step0 := prepared.BaselineSteps[0]
	messages, err := extractMessages(step0.Prompt)
	if err != nil {
		return agentLoopFail(e, result, exp, variantRun, "extract messages", err)
	}

	// Track current tools/tool_choice — refreshed from baseline steps as the loop progresses
	var tools []agwclient.ToolDefinition
	var toolChoice any

	for turn := 0; turn < maxTurns; turn++ {
		// Refresh tools/tool_choice from the corresponding baseline step (if one exists).
		// This mirrors per-step config changes (e.g. tool_choice going from "required" to "auto").
		// When the variant runs beyond the baseline step count, the last config is retained.
		if turn < len(prepared.BaselineSteps) {
			stepPrompt := prepared.BaselineSteps[turn].Prompt
			if t, err := extractTools(stepPrompt); err == nil {
				tools = t
			}
			if tc, err := extractToolChoice(stepPrompt); err == nil {
				toolChoice = tc
			}
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return agentLoopFail(e, result, exp, variantRun, "context cancelled", ctxErr)
		}

		// Build and send LLM request
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
			return agentLoopFail(e, result, exp, variantRun, fmt.Sprintf("llm call turn %d", turn), err)
		}

		// Parse response
		var assistantMsg agwclient.ChatMessage
		completion := ""
		if len(resp.Choices) > 0 {
			assistantMsg = resp.Choices[0].Message
			completion = assistantMsg.Content
		}

		// Build metadata
		metadata := storage.JSONB{
			"source":            "agent_loop",
			"baseline_trace_id": prepared.Experiment.BaselineTraceID,
			"turn":              turn,
		}
		if len(tools) > 0 {
			metadata["replay_tool_names"] = toolNamesFromDefinitions(tools)
		}
		if len(assistantMsg.ToolCalls) > 0 {
			metadata["tool_calls"] = assistantMsg.ToolCalls
		}

		// Build prompt JSONB for this turn
		prompt := buildPromptJSONB(messages, tools, toolChoice)

		// Store ReplayTrace for this turn
		variantTrace := &storage.ReplayTrace{
			TraceID:          variantTraceID,
			SpanID:           uuid.New().String(),
			RunID:            variantTraceID,
			StepIndex:        turn,
			CreatedAt:        time.Now(),
			Provider:         variant.Provider,
			Model:            resp.Model,
			Prompt:           prompt,
			Completion:       completion,
			Parameters:       storage.JSONB{},
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			LatencyMS:        latencyMS,
			Metadata:         metadata,
		}

		if err := e.store.CreateReplayTrace(ctx, variantTrace); err != nil {
			return agentLoopFail(e, result, exp, variantRun, fmt.Sprintf("store turn %d", turn), err)
		}

		result.Steps = append(result.Steps, variantTrace)
		result.TurnsExecuted = turn + 1

		// Update progress
		exp.Progress = float64(turn+1) / float64(maxTurns)
		_ = e.store.UpdateExperiment(ctx, exp)

		// Append assistant message to conversation
		messages = append(messages, assistantMsg)

		// If no tool calls, we're done
		if len(assistantMsg.ToolCalls) == 0 {
			result.StopReason = StopReasonNoToolCalls
			break
		}

		// Execute each tool call
		diverged := false
		for _, tc := range assistantMsg.ToolCalls {
			var toolArgs map[string]any
			// Use json.Number to preserve int64 precision for large values (>2^53).
			// freeze-mcp hashes the exact numeric payload, so float64 rounding would
			// produce a different args hash and miss the frozen capture.
			dec := json.NewDecoder(bytes.NewReader([]byte(tc.Function.Arguments)))
			dec.UseNumber()
			if err := dec.Decode(&toolArgs); err != nil {
				toolArgs = map[string]any{"_raw": tc.Function.Arguments}
			}

			// Compute args hash before CallTool for locator matching and capture reuse.
			argsJSONB := storage.JSONB(toolArgs)
			argsHash := otelreceiver.CalculateCaptureArgsHash(argsJSONB)

			// Set per-call locator if we have a matching baseline capture.
			if matched := captureQ.match(tc.Function.Name, argsHash); matched != nil {
				if locator, ok := toolExec.(ToolLocator); ok {
					locator.SetLocator(matched.SpanID, matched.StepIndex)
				}
			}

			toolResult, toolErr := toolExec.CallTool(ctx, tc.Function.Name, toolArgs)

			// Clear locator regardless of outcome.
			if locator, ok := toolExec.(ToolLocator); ok {
				locator.ClearLocator()
			}

			if toolErr != nil {
				return agentLoopFail(e, result, exp, variantRun, fmt.Sprintf("tool call %s turn %d", tc.Function.Name, turn), toolErr)
			}

			// Store ToolCapture (non-fatal if it fails — audit data)
			capture := &storage.ToolCapture{
				TraceID:   variantTraceID,
				SpanID:    variantTrace.SpanID,
				StepIndex: turn,
				ToolName:  tc.Function.Name,
				Args:      argsJSONB,
				ArgsHash:  argsHash,
				Result:    parseToolResultJSON(toolResult.Content),
				LatencyMS: toolResult.LatencyMS,
				RiskClass: otelreceiver.DetermineCaptureRiskClass(tc.Function.Name, argsJSONB),
				CreatedAt: time.Now(),
			}
			if toolResult.IsError {
				errStr := toolResult.Content
				capture.Error = &errStr
			}
			_ = e.store.CreateToolCapture(ctx, capture)

			// Check for divergence (tool_not_captured)
			if toolResult.IsError && toolResult.ErrorType == "tool_not_captured" {
				result.Divergences = append(result.Divergences, DivergenceEvent{
					Turn:      turn,
					ToolName:  tc.Function.Name,
					ErrorType: toolResult.ErrorType,
					Message:   fmt.Sprintf("variant called %s which was not in the baseline capture", tc.Function.Name),
				})
				result.StopReason = StopReasonDivergence
				diverged = true
				break
			}

			// Append tool result message for the LLM
			messages = append(messages, agwclient.ChatMessage{
				Role:       "tool",
				Content:    toolResult.Content,
				ToolCallID: tc.ID,
			})
		}

		if diverged {
			break
		}

		// If we've exhausted max turns
		if turn == maxTurns-1 {
			result.StopReason = StopReasonMaxTurns
		}
	}

	if result.StopReason == "" {
		result.StopReason = StopReasonMaxTurns
	}

	// Finalize: mark experiment completed (divergence is a valid result, not a failure)
	if err := e.finalizeSuccess(ctx, exp, variantRun, variantTraceID); err != nil {
		return result, err
	}

	return result, nil
}

// agentLoopFail marks the experiment as failed, records the error in the result,
// and returns the result with a wrapped error (including cleanup status if that also failed).
func agentLoopFail(e *Engine, result *AgentLoopResult, exp *storage.Experiment, run *storage.ExperimentRun, msg string, err error) (*AgentLoopResult, error) {
	result.Error = err
	cleanupErr := e.failExperiment(exp, run, err)
	if cleanupErr != nil {
		return result, fmt.Errorf("%s: %w (cleanup: %v)", msg, err, cleanupErr)
	}
	return result, fmt.Errorf("%s: %w", msg, err)
}

// buildPromptJSONB constructs a prompt JSONB matching the baseline format.
func buildPromptJSONB(messages []agwclient.ChatMessage, tools []agwclient.ToolDefinition, toolChoice any) storage.JSONB {
	prompt := storage.JSONB{
		"messages": messages,
	}
	if len(tools) > 0 {
		prompt["tools"] = tools
	}
	if toolChoice != nil {
		prompt["tool_choice"] = toolChoice
	}
	return prompt
}

// parseToolResultJSON attempts to parse content as a JSON object and return it
// directly as JSONB. This avoids double-encoding (wrapping JSON text in {"content": ...})
// which would produce escaped payloads in tool_captures. Falls back to {"content": ...}
// for non-JSON text content.
func parseToolResultJSON(content string) storage.JSONB {
	var parsed storage.JSONB
	if err := json.Unmarshal([]byte(content), &parsed); err == nil && parsed != nil {
		return parsed
	}
	return storage.JSONB{"content": content}
}

// captureQueue tracks baseline captures and matches them to variant tool calls
// by (tool_name, args_hash), scanning forward for the first unused match.
type captureQueue struct {
	captures []*storage.ToolCapture
	used     []bool
}

// match finds the first unused capture with matching tool_name and args_hash.
// Returns nil on nil receiver or no match.
func (q *captureQueue) match(toolName, argsHash string) *storage.ToolCapture {
	if q == nil {
		return nil
	}
	for i, cap := range q.captures {
		if !q.used[i] && cap.ToolName == toolName && cap.ArgsHash == argsHash {
			q.used[i] = true
			return cap
		}
	}
	return nil
}
