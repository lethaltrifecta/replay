package freezeloop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	demollm "github.com/lethaltrifecta/replay/internal/demoagent/llm"
	demotelemetry "github.com/lethaltrifecta/replay/internal/demoagent/telemetry"
)

type CaptureConfig struct {
	OTLPURL     string
	TraceID     string
	Model       string
	Provider    string
	Prompt      string
	Completion  string
	ToolName    string
	ToolArgs    map[string]any
	ToolResult  any
	ServiceName string
}

func RunCapture(ctx context.Context, cfg CaptureConfig) error {
	emitter := &demotelemetry.Emitter{
		Client:      &http.Client{Timeout: 10 * time.Second},
		BaseURL:     cfg.OTLPURL,
		TraceID:     cfg.TraceID,
		ServiceName: cfg.ServiceName,
		Provider:    cfg.Provider,
		Model:       cfg.Model,
		ToolSchemas: ToolDefinitions(),
		Mode:        "capture",
	}

	requestMessages := []demollm.Message{{Role: "user", Content: cfg.Prompt}}
	assistantMessage := demollm.Message{Role: "assistant", Content: cfg.Completion}
	if err := emitter.EmitTurn(ctx, 0, requestMessages, assistantMessage, demollm.Usage{
		PromptTokens:     18,
		CompletionTokens: 6,
		TotalTokens:      24,
	}, randomHex(8)); err != nil {
		return err
	}
	if err := emitter.EmitToolSpans(ctx, 0, []demotelemetry.ToolEvent{{
		Name:      cfg.ToolName,
		Args:      cfg.ToolArgs,
		Result:    cfg.ToolResult,
		LatencyMS: 5,
	}}, func() string { return randomHex(8) }); err != nil {
		return err
	}

	fmt.Printf("captured baseline trace_id=%s via %s\n", cfg.TraceID, cfg.OTLPURL)
	return nil
}

func MustJSON(v any) string {
	body, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(body)
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
