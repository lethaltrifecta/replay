package freezeloop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	demollm "github.com/lethaltrifecta/replay/internal/demoagent/llm"
	demomcp "github.com/lethaltrifecta/replay/internal/demoagent/mcpclient"
)

type LoopConfig struct {
	LLMURL          string
	MCPURL          string
	FreezeTraceID   string
	Model           string
	Prompt          string
	MaxTurns        int
	ExpectSubstring string
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

func RunLoop(ctx context.Context, cfg LoopConfig) error {
	llmClient := &demollm.Client{
		BaseURL:    cfg.LLMURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
	mcpClient, err := demomcp.Connect(ctx, cfg.MCPURL, &http.Client{
		Timeout: 15 * time.Second,
		Transport: &headerRoundTripper{
			headers: http.Header{"X-Freeze-Trace-ID": []string{cfg.FreezeTraceID}},
		},
	})
	if err != nil {
		return err
	}
	defer mcpClient.Close()

	toolNames, err := mcpClient.ListToolNames(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("freeze-mcp tools/list => %v\n", toolNames)

	messages := []demollm.Message{{Role: "user", Content: cfg.Prompt}}
	finalText := ""

	for turn := 0; turn < cfg.MaxTurns; turn++ {
		response, err := llmClient.CreateChatCompletion(ctx, demollm.ChatRequest{
			Model:    cfg.Model,
			Messages: messages,
			Tools:    ToolDefinitions(),
			Stream:   false,
		})
		if err != nil {
			return err
		}
		if len(response.Choices) == 0 {
			return fmt.Errorf("llm returned no choices")
		}

		message := response.Choices[0].Message
		messages = append(messages, message)
		if len(message.ToolCalls) == 0 {
			finalText = message.Content
			fmt.Printf("final assistant response => %s\n", finalText)
			break
		}

		for _, toolCall := range message.ToolCalls {
			var toolArgs map[string]any
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &toolArgs); err != nil {
				return fmt.Errorf("decode tool args for %s: %w", toolCall.Function.Name, err)
			}

			result, err := mcpClient.CallTool(ctx, toolCall.Function.Name, toolArgs)
			if err != nil {
				return err
			}
			toolLog, _ := json.Marshal(map[string]any{
				"name":   toolCall.Function.Name,
				"args":   toolArgs,
				"result": result,
			})
			fmt.Printf("tool call => %s\n", toolLog)
			if result.IsError {
				return fmt.Errorf("freeze-mcp returned isError=true")
			}

			contentBytes, err := json.Marshal(result.StructuredContent)
			if err != nil {
				return fmt.Errorf("marshal tool result: %w", err)
			}
			messages = append(messages, demollm.Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Content:    string(contentBytes),
			})
		}
	}

	if finalText == "" {
		return fmt.Errorf("agent loop exceeded max turns")
	}
	if cfg.ExpectSubstring != "" && !strings.Contains(finalText, cfg.ExpectSubstring) {
		return fmt.Errorf("expected final response to contain %q, got %q", cfg.ExpectSubstring, finalText)
	}
	return nil
}
