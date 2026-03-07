//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lethaltrifecta/replay/pkg/otelreceiver"
	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type seededCapture struct {
	toolName string
	args     map[string]any
	result   map[string]any
	errText  string
}

type freezeScenario struct {
	name           string
	seedForTrace   []seededCapture
	seedOther      []seededCapture
	sessionHeaders map[string]string
	callTool       string
	callArgs       map[string]any
	wantTools      []string
	wantResult     *float64
	wantErrorType  string
	repeatCall     bool
}

func TestFreezeMCPContract(t *testing.T) {
	if os.Getenv("E2E_FREEZE_CONTRACT") != "1" {
		t.Skip("set E2E_FREEZE_CONTRACT=1 to run freeze contract e2e")
	}

	postgresURL := envOrDefault("CMDR_POSTGRES_URL", "postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable")
	freezeHealthURL := envOrDefault("E2E_FREEZE_HEALTH_URL", "http://localhost:9090/health")
	freezeMCPURL := envOrDefault("E2E_FREEZE_MCP_URL", "http://localhost:9090/mcp")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	httpClient := &http.Client{Timeout: 10 * time.Second}
	waitForHTTP200(t, ctx, httpClient, freezeHealthURL)

	pool, err := pgxpool.New(ctx, postgresURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	scenarios := []freezeScenario{
		{
			name: "replay-hit-is-deterministic",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 4}},
			},
			callTool:   "calculator",
			callArgs:   map[string]any{"operation": "add", "a": 2, "b": 2},
			wantTools:  []string{"calculator"},
			wantResult: floatPtr(4),
			repeatCall: true,
		},
		{
			name: "wrong-args-return-tool-not-captured",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 4}},
			},
			callTool:      "calculator",
			callArgs:      map[string]any{"operation": "add", "a": 2, "b": 3},
			wantTools:     []string{"calculator"},
			wantErrorType: "tool_not_captured",
		},
		{
			name: "whole-float-and-int-args-match",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 4}},
			},
			callTool:   "calculator",
			callArgs:   map[string]any{"operation": "add", "a": 2.0, "b": 2.0},
			wantTools:  []string{"calculator"},
			wantResult: floatPtr(4),
		},
		{
			name: "large-int-args-remain-distinct",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "id", "id": int64(9007199254740993)}, result: map[string]any{"result": 4}},
			},
			callTool:      "calculator",
			callArgs:      map[string]any{"operation": "id", "id": int64(9007199254740992)},
			wantTools:     []string{"calculator"},
			wantErrorType: "tool_not_captured",
		},
		{
			name: "locator-selects-exact-step-for-duplicate-tool-args",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 4}},
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 5}},
			},
			sessionHeaders: map[string]string{
				"X-Freeze-Span-ID":    randomHexFromIndex(1),
				"X-Freeze-Step-Index": "1",
			},
			callTool:   "calculator",
			callArgs:   map[string]any{"operation": "add", "a": 2, "b": 2},
			wantTools:  []string{"calculator"},
			wantResult: floatPtr(5),
		},
		{
			name: "invalid-locator-headers-return-invalid-capture-locator",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 4}},
			},
			sessionHeaders: map[string]string{
				"X-Freeze-Span-ID": randomHexFromIndex(0),
			},
			callTool:      "calculator",
			callArgs:      map[string]any{"operation": "add", "a": 2, "b": 2},
			wantTools:     []string{"calculator"},
			wantErrorType: "invalid_capture_locator",
		},
		{
			name: "invalid-step-index-header-returns-invalid-capture-locator",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 4}},
			},
			sessionHeaders: map[string]string{
				"X-Freeze-Span-ID":    randomHexFromIndex(0),
				"X-Freeze-Step-Index": "not-an-int",
			},
			callTool:      "calculator",
			callArgs:      map[string]any{"operation": "add", "a": 2, "b": 2},
			wantTools:     []string{"calculator"},
			wantErrorType: "invalid_capture_locator",
		},
		{
			name: "locator-miss-does-not-fallback-to-args-hash",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 4}},
			},
			sessionHeaders: map[string]string{
				"X-Freeze-Span-ID":    randomHexFromIndex(0),
				"X-Freeze-Step-Index": "99",
			},
			callTool:      "calculator",
			callArgs:      map[string]any{"operation": "add", "a": 2, "b": 2},
			wantTools:     []string{"calculator"},
			wantErrorType: "tool_not_captured",
		},
		{
			name: "missing-trace-does-not-leak-other-trace-tools",
			seedOther: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 4}},
			},
			callTool:      "calculator",
			callArgs:      map[string]any{"operation": "add", "a": 2, "b": 2},
			wantTools:     []string{},
			wantErrorType: "tool_not_captured",
		},
		{
			name: "missing-tool-in-trace-returns-tool-not-captured",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, result: map[string]any{"result": 4}},
			},
			callTool:      "weather",
			callArgs:      map[string]any{"city": "new york"},
			wantTools:     []string{"calculator"},
			wantErrorType: "tool_not_captured",
		},
		{
			name: "captured-tool-error-is-returned",
			seedForTrace: []seededCapture{
				{toolName: "calculator", args: map[string]any{"operation": "add", "a": 2, "b": 2}, errText: "upstream timeout"},
			},
			callTool:      "calculator",
			callArgs:      map[string]any{"operation": "add", "a": 2, "b": 2},
			wantTools:     []string{"calculator"},
			wantErrorType: "captured_tool_error",
		},
	}

	for _, tc := range scenarios {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			traceID := randomHex(t, 16)
			otherTraceID := randomHex(t, 16)

			defer func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cleanupCancel()
				_, _ = pool.Exec(cleanupCtx, "DELETE FROM tool_captures WHERE trace_id = $1", traceID)
				_, _ = pool.Exec(cleanupCtx, "DELETE FROM tool_captures WHERE trace_id = $1", otherTraceID)
			}()

			for i, seed := range tc.seedOther {
				if err := insertToolCapture(ctx, pool, otherTraceID, i, seed); err != nil {
					t.Fatalf("seed other-trace capture: %v", err)
				}
			}
			for i, seed := range tc.seedForTrace {
				if err := insertToolCapture(ctx, pool, traceID, i, seed); err != nil {
					t.Fatalf("seed trace capture: %v", err)
				}
			}

			headers := map[string]string{"X-Freeze-Trace-ID": traceID}
			for key, value := range tc.sessionHeaders {
				headers[key] = value
			}

			session, err := connectMCPWithHeaders(ctx, freezeMCPURL, headers)
			if err != nil {
				t.Fatalf("connect mcp client: %v", err)
			}
			defer session.Close()

			toolsResp, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatalf("mcp tools/list: %v", err)
			}
			assertToolSet(t, toolsResp.Tools, tc.wantTools)

			call := &mcp.CallToolParams{Name: tc.callTool, Arguments: tc.callArgs}
			result, err := session.CallTool(ctx, call)
			if err != nil {
				t.Fatalf("mcp tools/call: %v", err)
			}
			assertScenarioResult(t, result, tc)

			if tc.repeatCall {
				repeat, err := session.CallTool(ctx, call)
				if err != nil {
					t.Fatalf("mcp tools/call (repeat): %v", err)
				}
				assertScenarioResult(t, repeat, tc)
				assertEquivalentCallResult(t, result, repeat)
			}
		})
	}
}

func insertToolCapture(ctx context.Context, pool *pgxpool.Pool, traceID string, index int, capture seededCapture) error {
	spanID := randomHexFromIndex(index)
	argsHash := otelreceiver.CalculateCaptureArgsHash(storage.JSONB(capture.args))
	riskClass := otelreceiver.DetermineCaptureRiskClass(capture.toolName, storage.JSONB(capture.args))

	var result any
	if capture.result != nil {
		result = capture.result
	}
	var errText any
	if strings.TrimSpace(capture.errText) != "" {
		errText = capture.errText
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO tool_captures (
			trace_id, span_id, step_index, tool_name, args, args_hash,
			result, error, latency_ms, risk_class, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
	`,
		traceID,
		spanID,
		index,
		capture.toolName,
		capture.args,
		argsHash,
		result,
		errText,
		5,
		riskClass,
	)
	return err
}

func assertToolSet(t *testing.T, tools []*mcp.Tool, want []string) {
	t.Helper()

	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool != nil {
			got = append(got, tool.Name)
		}
	}
	sort.Strings(got)
	wantSorted := append([]string{}, want...)
	sort.Strings(wantSorted)

	if !slices.Equal(got, wantSorted) {
		t.Fatalf("unexpected tools: got=%v want=%v", got, wantSorted)
	}
}

func assertScenarioResult(t *testing.T, result *mcp.CallToolResult, tc freezeScenario) {
	t.Helper()

	if result == nil {
		t.Fatal("tools/call returned nil result")
	}

	if tc.wantErrorType != "" {
		if !result.IsError {
			b, _ := json.Marshal(result)
			t.Fatalf("expected error result, got success: %s", string(b))
		}
		errType, errMsg := extractStructuredError(result.StructuredContent)
		if errType != tc.wantErrorType {
			t.Fatalf("unexpected error type: got=%q want=%q msg=%q", errType, tc.wantErrorType, errMsg)
		}
		return
	}

	if result.IsError {
		b, _ := json.Marshal(result)
		t.Fatalf("unexpected error result: %s", string(b))
	}

	if tc.wantResult != nil {
		var structured struct {
			Result float64 `json:"result"`
		}
		raw, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(raw, &structured); err != nil {
			t.Fatalf("decode structuredContent: %v; payload=%s", err, string(raw))
		}
		if structured.Result != *tc.wantResult {
			t.Fatalf("unexpected result: got=%v want=%v", structured.Result, *tc.wantResult)
		}
	}
}

func assertEquivalentCallResult(t *testing.T, a, b *mcp.CallToolResult) {
	t.Helper()

	if a == nil || b == nil {
		t.Fatalf("repeat call returned nil result: a=%v b=%v", a != nil, b != nil)
	}
	if a.IsError != b.IsError {
		t.Fatalf("repeat call mismatch in error state: first=%v second=%v", a.IsError, b.IsError)
	}

	ab, _ := json.Marshal(a.StructuredContent)
	bb, _ := json.Marshal(b.StructuredContent)
	if string(ab) != string(bb) {
		t.Fatalf("repeat call structuredContent mismatch: first=%s second=%s", string(ab), string(bb))
	}
}

func extractStructuredError(v any) (errorType string, message string) {
	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if v == nil {
		return "", ""
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", ""
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	return payload.Error.Type, payload.Error.Message
}

func randomHexFromIndex(index int) string {
	return fmt.Sprintf("%016x", index+1)
}

func floatPtr(v float64) *float64 {
	return &v
}
