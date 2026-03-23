package migrationdemo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	demollm "github.com/lethaltrifecta/replay/internal/demoagent/llm"
	demomcp "github.com/lethaltrifecta/replay/internal/demoagent/mcpclient"
	demotelemetry "github.com/lethaltrifecta/replay/internal/demoagent/telemetry"
)

type AgentConfig struct {
	Mode                 string
	Model                string
	Provider             string
	Behavior             string
	TraceID              string
	FreezeTraceID        string
	ServiceName          string
	OTLPURL              string
	OTLPMode             string
	LLMURL               string
	MCPURL               string
	Prompt               string
	MaxTurns             int
	ExpectFinalSubstring string
	ExpectToolError      bool
}

type toolEvent struct {
	Name      string
	Args      map[string]any
	Result    any
	Error     string
	LatencyMS int64
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers http.Header
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := h.base
	if base == nil {
		base = http.DefaultTransport
	}
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	for key, values := range h.headers {
		cloned.Header.Del(key)
		for _, value := range values {
			cloned.Header.Add(key, value)
		}
	}
	return base.RoundTrip(cloned)
}

func RunAgent(ctx context.Context, cfg AgentConfig) error {
	traceparent := makeTraceparent(cfg.TraceID, randomHex(8))

	llmHTTPClient := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &headerRoundTripper{
			headers: http.Header{"traceparent": []string{traceparent}},
		},
	}
	llmClient := &demollm.Client{
		BaseURL:    cfg.LLMURL,
		HTTPClient: llmHTTPClient,
	}

	mcpHeaders := http.Header{"traceparent": []string{traceparent}}
	if cfg.FreezeTraceID != "" {
		mcpHeaders.Set("X-Freeze-Trace-ID", cfg.FreezeTraceID)
	}
	mcpHTTPClient := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &headerRoundTripper{
			headers: mcpHeaders,
		},
	}
	mcpClient, err := demomcp.Connect(ctx, cfg.MCPURL, mcpHTTPClient)
	if err != nil {
		return err
	}
	defer mcpClient.Close()

	toolNames, err := mcpClient.ListToolNames(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("mcp tools/list => %v\n", toolNames)
	fmt.Printf("agent trace_id => %s\n", cfg.TraceID)

	var emitter *demotelemetry.Emitter
	if cfg.OTLPURL != "" {
		emitter = &demotelemetry.Emitter{
			Client:        &http.Client{Timeout: 10 * time.Second},
			BaseURL:       cfg.OTLPURL,
			TraceID:       cfg.TraceID,
			ServiceName:   cfg.ServiceName,
			Provider:      cfg.Provider,
			Model:         cfg.Model,
			ToolSchemas:   ToolDefinitions(),
			FreezeTraceID: cfg.FreezeTraceID,
			Mode:          cfg.Mode,
		}
	}

	messages := []demollm.Message{
		{Role: "system", Content: fmt.Sprintf("demo_behavior=%s", cfg.Behavior)},
		{Role: "user", Content: cfg.Prompt},
	}

	finalText := ""
	toolErrorSeen := false

	for turn := 0; turn < cfg.MaxTurns; turn++ {
		requestMessages := cloneMessages(messages)
		chatResp, err := llmClient.CreateChatCompletion(ctx, demollm.ChatRequest{
			Model:    cfg.Model,
			Messages: requestMessages,
			Tools:    ToolDefinitions(),
			Stream:   false,
		})
		if err != nil {
			return err
		}
		if len(chatResp.Choices) == 0 {
			return fmt.Errorf("llm returned no choices")
		}

		assistantMessage := chatResp.Choices[0].Message
		messages = append(messages, assistantMessage)

		toolEvents := make([]toolEvent, 0, len(assistantMessage.ToolCalls))
		for _, toolCall := range assistantMessage.ToolCalls {
			var toolArgs map[string]any
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &toolArgs); err != nil {
				return fmt.Errorf("decode tool args for %s: %w", toolCall.Function.Name, err)
			}

			toolStart := time.Now()
			result, err := mcpClient.CallTool(ctx, toolCall.Function.Name, toolArgs)
			if err != nil {
				result = normalizeToolCallError(err)
			}
			latencyMS := time.Since(toolStart).Milliseconds()

			errorMessage := ""
			if result.IsError {
				toolErrorSeen = true
				errorMessage = extractToolError(result.StructuredContent)
				if errorMessage == "" {
					errorMessage = "tool call failed"
				}
			}

			toolEvents = append(toolEvents, toolEvent{
				Name:      toolCall.Function.Name,
				Args:      toolArgs,
				Result:    result.StructuredContent,
				Error:     errorMessage,
				LatencyMS: latencyMS,
			})

			resultJSON, _ := json.Marshal(result)
			toolLog, _ := json.Marshal(map[string]any{
				"name":   toolCall.Function.Name,
				"args":   toolArgs,
				"result": json.RawMessage(resultJSON),
			})
			fmt.Printf("tool call => %s\n", toolLog)

			toolMessagePayload := normalizeToolMessagePayload(result.StructuredContent, errorMessage, result.IsError)
			toolContentBytes, err := json.Marshal(toolMessagePayload)
			if err != nil {
				return fmt.Errorf("marshal tool message payload: %w", err)
			}
			messages = append(messages, demollm.Message{
				Role:       "tool",
				Name:       toolCall.Function.Name,
				ToolCallID: toolCall.ID,
				Content:    string(toolContentBytes),
			})
		}

		if emitter != nil {
			switch cfg.OTLPMode {
			case "combined":
				if err := emitter.EmitTurn(ctx, turn, requestMessages, assistantMessage, chatResp.Usage, randomHex(8)); err != nil {
					return err
				}
				if err := emitter.EmitToolSpans(ctx, turn, toTelemetryEvents(toolEvents), func() string { return randomHex(8) }); err != nil {
					return err
				}
			case "tools-only":
				if err := emitter.EmitToolSpans(ctx, turn, toTelemetryEvents(toolEvents), func() string { return randomHex(8) }); err != nil {
					return err
				}
			}
		}

		if len(assistantMessage.ToolCalls) == 0 {
			finalText = assistantMessage.Content
			fmt.Printf("final assistant response => %s\n", finalText)
			break
		}
	}

	if finalText == "" {
		return fmt.Errorf("agent loop exceeded max turns")
	}
	if cfg.ExpectFinalSubstring != "" && !strings.Contains(finalText, cfg.ExpectFinalSubstring) {
		return fmt.Errorf("expected final response to contain %q, got %q", cfg.ExpectFinalSubstring, finalText)
	}
	if cfg.ExpectToolError && !toolErrorSeen {
		return fmt.Errorf("expected to observe at least one tool error but none occurred")
	}
	return nil
}

func cloneMessages(messages []demollm.Message) []demollm.Message {
	encoded, _ := json.Marshal(messages)
	var cloned []demollm.Message
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

func normalizeToolMessagePayload(v any, errorMessage string, isError bool) any {
	if v == nil {
		if isError {
			return map[string]any{"error": map[string]any{"message": errorMessage}}
		}
		return map[string]any{}
	}
	if !isError {
		return v
	}
	asMap, ok := v.(map[string]any)
	if ok {
		if _, found := asMap["error"]; found {
			return asMap
		}
	}
	return map[string]any{"error": map[string]any{"message": errorMessage}}
}

func extractToolError(v any) string {
	errMap, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	inner, ok := errMap["error"].(map[string]any)
	if !ok {
		return ""
	}
	message, _ := inner["message"].(string)
	return message
}

func normalizeToolCallError(err error) *demomcp.ToolResult {
	errorType := "tool_execution_error"
	if strings.Contains(err.Error(), "unknown tool") {
		errorType = "tool_not_captured"
	}

	return &demomcp.ToolResult{
		StructuredContent: map[string]any{
			"error": map[string]any{
				"type":    errorType,
				"message": err.Error(),
			},
		},
		IsError: true,
	}
}

func toTelemetryEvents(events []toolEvent) []demotelemetry.ToolEvent {
	out := make([]demotelemetry.ToolEvent, 0, len(events))
	for _, event := range events {
		out = append(out, demotelemetry.ToolEvent{
			Name:      event.Name,
			Args:      event.Args,
			Result:    event.Result,
			Error:     event.Error,
			LatencyMS: event.LatencyMS,
		})
	}
	return out
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func makeTraceparent(traceID, parentSpanID string) string {
	return fmt.Sprintf("00-%s-%s-01", traceID, parentSpanID)
}
