package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	demollm "github.com/lethaltrifecta/replay/internal/demoagent/llm"
	"github.com/lethaltrifecta/replay/internal/freezeloop"
	"github.com/lethaltrifecta/replay/internal/migrationdemo"
)

type config struct {
	mode         string
	host         string
	port         int
	model        string
	responseText string
	toolName     string
	toolArgs     map[string]any
}

type chatCompletion struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int             `json:"index"`
		Message      demollm.Message `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage demollm.Usage `json:"usage"`
}

func main() {
	cfg := parseFlags()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": cfg.mode})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := handleChatCompletion(w, r, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})

	addr := fmt.Sprintf("%s:%d", cfg.host, cfg.port)
	log.Printf("mock openai upstream mode=%s listening on http://%s", cfg.mode, addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func parseFlags() config {
	var cfg config
	var toolArgsJSON string
	flag.StringVar(&cfg.mode, "mode", "basic", "basic, toolloop, or migration")
	flag.StringVar(&cfg.host, "host", "127.0.0.1", "listen host")
	flag.IntVar(&cfg.port, "port", 18080, "listen port")
	flag.StringVar(&cfg.model, "model", freezeloop.DefaultModel, "default model name")
	flag.StringVar(&cfg.responseText, "response-text", "Mock response from local upstream.", "basic mode assistant response")
	flag.StringVar(&cfg.toolName, "tool-name", freezeloop.DefaultToolName, "toolloop mode tool name")
	flag.StringVar(&toolArgsJSON, "tool-args", mustJSON(freezeloop.DefaultToolArgs()), "toolloop mode tool args JSON")
	flag.Parse()

	if err := json.Unmarshal([]byte(toolArgsJSON), &cfg.toolArgs); err != nil {
		fmt.Fprintf(os.Stderr, "decode --tool-args: %v\n", err)
		os.Exit(2)
	}
	return cfg
}

func handleChatCompletion(w http.ResponseWriter, r *http.Request, cfg config) error {
	var payload demollm.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	body, _ := json.Marshal(payload)
	fmt.Println(string(body))

	model := payload.Model
	if model == "" {
		model = cfg.model
	}

	var resp chatCompletion
	switch cfg.mode {
	case "basic":
		resp = basicResponse(model, cfg.responseText)
	case "toolloop":
		resp = toolLoopResponse(model, payload.Messages, cfg.toolName, cfg.toolArgs)
	case "migration":
		resp = migrationResponse(model, payload.Messages)
	default:
		return fmt.Errorf("unsupported mode %q", cfg.mode)
	}

	writeJSON(w, http.StatusOK, resp)
	return nil
}

func basicResponse(model, responseText string) chatCompletion {
	return completionResponse(
		fmt.Sprintf("chatcmpl-%s-basic", model),
		model,
		demollm.Message{Role: "assistant", Content: responseText},
		"stop",
		demollm.Usage{PromptTokens: 12, CompletionTokens: 7, TotalTokens: 19},
	)
}

func toolLoopResponse(model string, messages []demollm.Message, toolName string, toolArgs map[string]any) chatCompletion {
	var toolMessage *demollm.Message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			toolMessage = &messages[i]
			break
		}
	}

	if toolMessage == nil {
		return completionResponse(
			"chatcmpl-toolloop-1",
			model,
			demollm.Message{
				Role:    "assistant",
				Content: "",
				ToolCalls: []demollm.ToolCall{
					{
						ID:   freezeloop.DefaultToolCallID,
						Type: "function",
						Function: demollm.ToolFunction{
							Name:      toolName,
							Arguments: mustJSON(toolArgs),
						},
					},
				},
			},
			"tool_calls",
			demollm.Usage{PromptTokens: 18, CompletionTokens: 6, TotalTokens: 24},
		)
	}

	var toolPayload map[string]any
	if err := json.Unmarshal([]byte(toolMessage.Content), &toolPayload); err != nil {
		toolPayload = map[string]any{"raw": toolMessage.Content}
	}

	finalText := fmt.Sprintf("Received frozen tool payload: %s", mustJSON(toolPayload))
	if value, ok := toolPayload["result"]; ok {
		finalText = fmt.Sprintf("The frozen tool result is %v.", value)
	}
	if value, ok := toolPayload["error"]; ok {
		switch typed := value.(type) {
		case string:
			finalText = fmt.Sprintf("Tool execution failed: %s", typed)
		case map[string]any:
			finalText = fmt.Sprintf("Tool execution failed: %v", typed["message"])
		default:
			finalText = fmt.Sprintf("Tool execution failed: %v", typed)
		}
	}

	return completionResponse(
		"chatcmpl-toolloop-2",
		model,
		demollm.Message{Role: "assistant", Content: finalText},
		"stop",
		demollm.Usage{PromptTokens: 28, CompletionTokens: 9, TotalTokens: 37},
	)
}

func migrationResponse(model string, messages []demollm.Message) chatCompletion {
	behavior := "safe"
	for _, message := range messages {
		if strings.Contains(message.Content, "demo_behavior=unsafe") {
			behavior = "unsafe"
			break
		}
	}

	completedSteps := 0
	var lastToolMessage *demollm.Message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			completedSteps++
			if lastToolMessage == nil {
				lastToolMessage = &messages[i]
			}
		}
	}

	if behavior == "unsafe" {
		if completedSteps == 0 {
			step := migrationdemo.UnsafeSequence()[0]
			return completionResponse(
				"chatcmpl-migration-unsafe-1",
				model,
				demollm.Message{
					Role:    "assistant",
					Content: "",
					ToolCalls: []demollm.ToolCall{
						{
							ID:   "call_drop_table_1",
							Type: "function",
							Function: demollm.ToolFunction{
								Name:      step.Name,
								Arguments: mustJSON(step.Args),
							},
						},
					},
				},
				"tool_calls",
				demollm.Usage{PromptTokens: 24, CompletionTokens: 8, TotalTokens: 32},
			)
		}

		var toolPayload map[string]any
		if lastToolMessage != nil && json.Unmarshal([]byte(lastToolMessage.Content), &toolPayload) == nil {
			if _, ok := toolPayload["error"]; ok {
				return completionResponse(
					fmt.Sprintf("chatcmpl-%s-final", model),
					model,
					demollm.Message{Role: "assistant", Content: migrationdemo.UnsafeBlockedText},
					"stop",
					demollm.Usage{PromptTokens: 64, CompletionTokens: 16, TotalTokens: 80},
				)
			}
		}

		return completionResponse(
			fmt.Sprintf("chatcmpl-%s-final", model),
			model,
			demollm.Message{Role: "assistant", Content: "Unsafe migration path completed."},
			"stop",
			demollm.Usage{PromptTokens: 64, CompletionTokens: 16, TotalTokens: 80},
		)
	}

	steps := migrationdemo.SafeSequence()
	if completedSteps < len(steps) {
		step := steps[completedSteps]
		return completionResponse(
			fmt.Sprintf("chatcmpl-migration-safe-%d", completedSteps+1),
			model,
			demollm.Message{
				Role:    "assistant",
				Content: "",
				ToolCalls: []demollm.ToolCall{
					{
						ID:   fmt.Sprintf("call_%s_%d", step.Name, completedSteps+1),
						Type: "function",
						Function: demollm.ToolFunction{
							Name:      step.Name,
							Arguments: mustJSON(step.Args),
						},
					},
				},
			},
			"tool_calls",
			demollm.Usage{
				PromptTokens:     24 + completedSteps*8,
				CompletionTokens: 8,
				TotalTokens:      32 + completedSteps*8,
			},
		)
	}

	return completionResponse(
		fmt.Sprintf("chatcmpl-%s-final", model),
		model,
		demollm.Message{Role: "assistant", Content: migrationdemo.SafeFinalText},
		"stop",
		demollm.Usage{PromptTokens: 64, CompletionTokens: 16, TotalTokens: 80},
	)
}

func completionResponse(id, model string, message demollm.Message, finishReason string, usage demollm.Usage) chatCompletion {
	resp := chatCompletion{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Usage:   usage,
	}
	resp.Choices = []struct {
		Index        int             `json:"index"`
		Message      demollm.Message `json:"message"`
		FinishReason string          `json:"finish_reason"`
	}{
		{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		},
	}
	return resp
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func mustJSON(v any) string {
	body, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(body)
}
