package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/lethaltrifecta/replay/pkg/otelreceiver"
	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/lethaltrifecta/replay/pkg/utils/logger"
)

func main() {
	// Create a test trace programmatically
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()

	// Add resource attributes
	rs.Resource().Attributes().PutStr("service.name", "test-service")

	// Add scope
	ss := rs.ScopeSpans().AppendEmpty()

	// Add span
	span := ss.Spans().AppendEmpty()
	span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	span.SetName("llm.completion")
	span.SetKind(ptrace.SpanKindInternal)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Second)))

	// Add LLM attributes
	span.Attributes().PutStr("gen_ai.request.model", "claude-3-5-sonnet-20241022")
	span.Attributes().PutStr("gen_ai.system", "anthropic")
	span.Attributes().PutStr("gen_ai.prompt.0.role", "user")
	span.Attributes().PutStr("gen_ai.prompt.0.content", "What is 2+2?")
	span.Attributes().PutStr("gen_ai.completion.0.content", "2+2 equals 4.")
	span.Attributes().PutInt("gen_ai.usage.input_tokens", 10)
	span.Attributes().PutInt("gen_ai.usage.output_tokens", 8)
	span.Attributes().PutDouble("gen_ai.request.temperature", 0.7)

	// Add tool call event
	event := span.Events().AppendEmpty()
	event.SetName("tool.call")
	event.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	event.Attributes().PutStr("tool.name", "calculator")
	event.Attributes().PutStr("tool.args", `{"operation": "add", "a": 2, "b": 2}`)
	event.Attributes().PutStr("tool.result", `{"result": 4}`)
	event.Attributes().PutInt("tool.latency_ms", 5)

	// Create parser
	logger, _ := logger.New("debug")
	parser := otelreceiver.NewParser(logger)

	// Test IsLLMSpan
	fmt.Println("Testing IsLLMSpan...")
	isLLM := parser.IsLLMSpan(span)
	fmt.Printf("  IsLLMSpan: %v\n", isLLM)

	if !isLLM {
		log.Fatal("ERROR: Span not detected as LLM span!")
	}

	// Test ParseLLMSpan
	fmt.Println("\nParsing LLM span...")
	replayTrace := parser.ParseLLMSpan(span, rs.Resource())
	if replayTrace == nil {
		log.Fatal("ERROR: ParseLLMSpan returned nil!")
	}

	fmt.Printf("  Trace ID: %s\n", replayTrace.TraceID)
	fmt.Printf("  Model: %s\n", replayTrace.Model)
	fmt.Printf("  Provider: %s\n", replayTrace.Provider)
	fmt.Printf("  Prompt: %+v\n", replayTrace.Prompt)
	fmt.Printf("  Completion: %s\n", replayTrace.Completion)
	fmt.Printf("  Tokens: %d input, %d output, %d total\n",
		replayTrace.PromptTokens, replayTrace.CompletionTokens, replayTrace.TotalTokens)

	// Test ParseToolCalls
	fmt.Println("\nParsing tool calls...")
	toolCaptures := parser.ParseToolCalls(span, replayTrace.TraceID, replayTrace.SpanID)
	fmt.Printf("  Found %d tool calls\n", len(toolCaptures))

	for i, capture := range toolCaptures {
		fmt.Printf("  Tool %d:\n", i)
		fmt.Printf("    Name: %s\n", capture.ToolName)
		fmt.Printf("    Args: %+v\n", capture.Args)
		fmt.Printf("    ArgsHash: %s\n", capture.ArgsHash)
		fmt.Printf("    Risk Class: %s\n", capture.RiskClass)
	}

	// Test with real database (if available)
	fmt.Println("\n Testing with database...")
	dbURL := "postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable"

	store, err := storage.NewPostgresStorage(dbURL, 10)
	if err != nil {
		fmt.Printf("  ⚠️  Could not connect to database: %v\n", err)
		fmt.Println("  Run: make dev-up")
		return
	}
	defer store.Close()

	ctx := context.Background()

	// Store replay trace
	if err := store.CreateReplayTrace(ctx, replayTrace); err != nil {
		fmt.Printf("  ❌ Failed to store replay trace: %v\n", err)
		return
	}
	fmt.Println("  ✅ Replay trace stored successfully")

	// Store tool captures
	for _, capture := range toolCaptures {
		if err := store.CreateToolCapture(ctx, capture); err != nil {
			fmt.Printf("  ❌ Failed to store tool capture: %v\n", err)
			return
		}
	}
	fmt.Printf("  ✅ %d tool captures stored successfully\n", len(toolCaptures))

	// Verify storage
	storedSpans, err := store.GetReplayTraceSpans(ctx, replayTrace.TraceID)
	if err != nil {
		fmt.Printf("  ❌ Failed to retrieve trace: %v\n", err)
		return
	}
	if len(storedSpans) == 0 {
		fmt.Println("  ❌ No spans found for trace")
		return
	}
	fmt.Printf("  ✅ Retrieved %d span(s): model=%s, tokens=%d\n", len(storedSpans), storedSpans[0].Model, storedSpans[0].TotalTokens)

	fmt.Println("\n✅ All tests passed!")
}
