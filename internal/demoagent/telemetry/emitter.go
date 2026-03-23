package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lethaltrifecta/replay/internal/demoagent/llm"
)

type Emitter struct {
	Client        *http.Client
	BaseURL       string
	TraceID       string
	ServiceName   string
	Provider      string
	Model         string
	ToolSchemas   []llm.ToolDefinition
	FreezeTraceID string
	Mode          string
}

type ToolEvent struct {
	Name      string
	Args      map[string]any
	Result    any
	Error     string
	LatencyMS int64
}

func (e *Emitter) EmitTurn(ctx context.Context, turn int, requestMessages []llm.Message, assistantMessage llm.Message, usage llm.Usage, spanID string) error {
	startNS := time.Now().UnixNano()
	endNS := startNS + 100_000_000

	attributes := []map[string]any{
		attr("gen_ai.request.model", "stringValue", e.Model),
		attr("gen_ai.provider.name", "stringValue", e.Provider),
		attr("gen_ai.request.tools", "stringValue", mustJSON(e.ToolSchemas)),
		attr("demo.mode", "stringValue", e.Mode),
		attr("demo.scenario", "stringValue", "migration"),
		attr("demo.turn", "intValue", fmt.Sprintf("%d", turn)),
	}
	if e.Mode == "replay" && e.FreezeTraceID != "" {
		attributes = append(attributes, attr("demo.freeze_trace_id", "stringValue", e.FreezeTraceID))
	}
	for index, message := range requestMessages {
		attributes = append(attributes, attr(fmt.Sprintf("gen_ai.prompt.%d.role", index), "stringValue", message.Role))
		attributes = append(attributes, attr(fmt.Sprintf("gen_ai.prompt.%d.content", index), "stringValue", message.Content))
		if message.Name != "" {
			attributes = append(attributes, attr(fmt.Sprintf("gen_ai.prompt.%d.name", index), "stringValue", message.Name))
		}
		if message.ToolCallID != "" {
			attributes = append(attributes, attr(fmt.Sprintf("gen_ai.prompt.%d.tool_call_id", index), "stringValue", message.ToolCallID))
		}
		if len(message.ToolCalls) > 0 {
			attributes = append(attributes, attr(fmt.Sprintf("gen_ai.prompt.%d.tool_calls", index), "stringValue", mustJSON(message.ToolCalls)))
		}
	}
	attributes = append(attributes,
		attr("gen_ai.completion.0.content", "stringValue", assistantMessage.Content),
		attr("gen_ai.usage.input_tokens", "intValue", fmt.Sprintf("%d", usage.PromptTokens)),
		attr("gen_ai.usage.output_tokens", "intValue", fmt.Sprintf("%d", usage.CompletionTokens)),
	)

	payload := map[string]any{
		"resourceSpans": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{attr("service.name", "stringValue", e.ServiceName)},
				},
				"scopeSpans": []any{
					map[string]any{
						"spans": []any{
							map[string]any{
								"traceId":           e.TraceID,
								"spanId":            spanID,
								"name":              "llm.chat.completions",
								"kind":              1,
								"startTimeUnixNano": fmt.Sprintf("%d", startNS),
								"endTimeUnixNano":   fmt.Sprintf("%d", endNS),
								"attributes":        attributes,
								"status":            map[string]any{"code": 1},
							},
						},
					},
				},
			},
		},
	}
	return e.postOTLP(ctx, payload)
}

func (e *Emitter) EmitToolSpans(ctx context.Context, turn int, toolEvents []ToolEvent, randomSpanID func() string) error {
	if len(toolEvents) == 0 {
		return nil
	}

	baseStartNS := time.Now().UnixNano()
	spans := make([]any, 0, len(toolEvents))
	for index, toolEvent := range toolEvents {
		startNS := baseStartNS + int64(index)*10_000_000
		latencyMS := toolEvent.LatencyMS
		if latencyMS < 1 {
			latencyMS = 1
		}
		endNS := startNS + latencyMS*1_000_000
		attributes := []any{
			attr("gen_ai.operation.name", "stringValue", "execute_tool"),
			attr("gen_ai.tool.name", "stringValue", toolEvent.Name),
			attr("gen_ai.tool.call.arguments", "stringValue", mustJSON(toolEvent.Args)),
			attr("mcp.method.name", "stringValue", "tools/call"),
			attr("demo.mode", "stringValue", e.Mode),
			attr("demo.scenario", "stringValue", "migration"),
			attr("demo.turn", "intValue", fmt.Sprintf("%d", turn)),
		}
		if e.Mode == "replay" && e.FreezeTraceID != "" {
			attributes = append(attributes, attr("demo.freeze_trace_id", "stringValue", e.FreezeTraceID))
		}
		if toolEvent.Result != nil {
			attributes = append(attributes, attr("gen_ai.tool.call.result", "stringValue", mustJSON(toolEvent.Result)))
		}
		if toolEvent.Error != "" {
			attributes = append(attributes, attr("error.message", "stringValue", toolEvent.Error))
		}
		spans = append(spans, map[string]any{
			"traceId":           e.TraceID,
			"spanId":            randomSpanID(),
			"name":              fmt.Sprintf("execute_tool %s", toolEvent.Name),
			"kind":              1,
			"startTimeUnixNano": fmt.Sprintf("%d", startNS),
			"endTimeUnixNano":   fmt.Sprintf("%d", endNS),
			"attributes":        attributes,
			"status":            map[string]any{"code": 1},
		})
	}

	payload := map[string]any{
		"resourceSpans": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{attr("service.name", "stringValue", e.ServiceName)},
				},
				"scopeSpans": []any{
					map[string]any{
						"spans": spans,
					},
				},
			},
		},
	}
	return e.postOTLP(ctx, payload)
}

func (e *Emitter) postOTLP(ctx context.Context, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal otlp payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(e.BaseURL, "/")+"/v1/traces", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create otlp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.Client.Do(req)
	if err != nil {
		return fmt.Errorf("otlp request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("otlp ingest status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func attr(key, valueType string, value any) map[string]any {
	return map[string]any{"key": key, "value": map[string]any{valueType: value}}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
