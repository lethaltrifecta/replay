//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReplayFreezeIngestAndReplayIntegration(t *testing.T) {
	if os.Getenv("E2E_REPLAY_FREEZE") != "1" {
		t.Skip("set E2E_REPLAY_FREEZE=1 to run replay+freeze e2e")
	}

	postgresURL := envOrDefault("CMDR_POSTGRES_URL", "postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable")
	otlpHealthURL := envOrDefault("E2E_OTLP_HEALTH_URL", "http://localhost:4318/health")
	otlpIngestURL := envOrDefault("E2E_OTLP_INGEST_URL", "http://localhost:4318/v1/traces")
	freezeHealthURL := envOrDefault("E2E_FREEZE_HEALTH_URL", "http://localhost:9090/health")
	freezeMCPURL := envOrDefault("E2E_FREEZE_MCP_URL", "http://localhost:9090/mcp")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	httpClient := &http.Client{Timeout: 10 * time.Second}

	waitForHTTP200(t, ctx, httpClient, otlpHealthURL)
	waitForHTTP200(t, ctx, httpClient, freezeHealthURL)

	pool, err := pgxpool.New(ctx, postgresURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	traceID := randomHex(t, 16) // OTLP traceId is 16 bytes hex-encoded (32 chars)
	spanID := randomHex(t, 8)   // OTLP spanId is 8 bytes hex-encoded (16 chars)

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM tool_captures WHERE trace_id = $1", traceID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM replay_traces WHERE trace_id = $1", traceID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM otel_traces WHERE trace_id = $1", traceID)
	}()

	if err := sendOTLPTrace(ctx, httpClient, otlpIngestURL, traceID, spanID); err != nil {
		t.Fatalf("send otlp trace: %v", err)
	}

	if err := waitForCapture(ctx, pool, traceID); err != nil {
		t.Fatalf("wait for tool capture: %v", err)
	}

	var toolName string
	var resultText string
	var capturedSpanID string
	var capturedStepIndex int
	err = pool.QueryRow(ctx,
		`SELECT tool_name, result::text, span_id, step_index FROM tool_captures WHERE trace_id = $1 ORDER BY created_at DESC LIMIT 1`,
		traceID,
	).Scan(&toolName, &resultText, &capturedSpanID, &capturedStepIndex)
	if err != nil {
		t.Fatalf("query capture row: %v", err)
	}
	if toolName != "calculator" {
		t.Fatalf("unexpected tool name: got %q want %q", toolName, "calculator")
	}
	if resultText != `{"result": 4}` && resultText != `{"result":4}` {
		t.Fatalf("unexpected frozen result payload: %s", resultText)
	}

	session, err := connectMCP(ctx, freezeMCPURL, traceID)
	if err != nil {
		t.Fatalf("connect mcp client: %v", err)
	}
	defer session.Close()

	listResp, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("mcp tools/list: %v", err)
	}
	if !hasTool(listResp.Tools, "calculator") {
		t.Fatalf("tools/list does not contain calculator")
	}

	callReq := &mcp.CallToolParams{
		Name: "calculator",
		Arguments: map[string]any{
			"operation": "add",
			"a":         2,
			"b":         2,
		},
	}

	callResp, err := session.CallTool(ctx, callReq)
	if err != nil {
		t.Fatalf("mcp tools/call: %v", err)
	}
	assertFrozenResult(t, callResp)

	// Repeat call to verify deterministic output across repeated requests.
	callResp2, err := session.CallTool(ctx, callReq)
	if err != nil {
		t.Fatalf("mcp tools/call (repeat): %v", err)
	}
	assertFrozenResult(t, callResp2)

	// Verify step-accurate locator path against the captured row.
	locatorSession, err := connectMCPWithHeaders(ctx, freezeMCPURL, map[string]string{
		"X-Freeze-Trace-ID":   traceID,
		"X-Freeze-Span-ID":    capturedSpanID,
		"X-Freeze-Step-Index": fmt.Sprintf("%d", capturedStepIndex),
	})
	if err != nil {
		t.Fatalf("connect mcp client (locator): %v", err)
	}
	defer locatorSession.Close()

	locatorResp, err := locatorSession.CallTool(ctx, callReq)
	if err != nil {
		t.Fatalf("mcp tools/call (locator): %v", err)
	}
	assertFrozenResult(t, locatorResp)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomHex(t *testing.T, nBytes int) string {
	t.Helper()
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(buf)
}

func waitForHTTP200(t *testing.T, ctx context.Context, client *http.Client, url string) {
	t.Helper()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			resp, doErr := client.Do(req)
			if doErr == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return
				}
			}
		}

		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for %s to return 200", url)
		case <-ticker.C:
		}
	}
}

func sendOTLPTrace(ctx context.Context, client *http.Client, ingestURL, traceID, spanID string) error {
	now := time.Now()
	start := now.Add(-100 * time.Millisecond).UnixNano()
	end := now.UnixNano()

	payload := map[string]any{
		"resourceSpans": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{
						map[string]any{"key": "service.name", "value": map[string]any{"stringValue": "e2e-agent"}},
					},
				},
				"scopeSpans": []any{
					map[string]any{
						"spans": []any{
							map[string]any{
								"traceId":           traceID,
								"spanId":            spanID,
								"name":              "llm.completion",
								"kind":              1,
								"startTimeUnixNano": fmt.Sprintf("%d", start),
								"endTimeUnixNano":   fmt.Sprintf("%d", end),
								"attributes": []any{
									map[string]any{"key": "gen_ai.request.model", "value": map[string]any{"stringValue": "claude-3-5-sonnet-20241022"}},
									map[string]any{"key": "gen_ai.system", "value": map[string]any{"stringValue": "anthropic"}},
									map[string]any{"key": "gen_ai.prompt.0.role", "value": map[string]any{"stringValue": "user"}},
									map[string]any{"key": "gen_ai.prompt.0.content", "value": map[string]any{"stringValue": "What is 2+2?"}},
									map[string]any{"key": "gen_ai.completion.0.content", "value": map[string]any{"stringValue": "2+2 equals 4."}},
									map[string]any{"key": "gen_ai.usage.input_tokens", "value": map[string]any{"intValue": "10"}},
									map[string]any{"key": "gen_ai.usage.output_tokens", "value": map[string]any{"intValue": "8"}},
								},
								"events": []any{
									map[string]any{
										"timeUnixNano": fmt.Sprintf("%d", start),
										"name":         "tool.call",
										"attributes": []any{
											map[string]any{"key": "tool.name", "value": map[string]any{"stringValue": "calculator"}},
											map[string]any{"key": "tool.args", "value": map[string]any{"stringValue": `{"operation": "add", "a": 2, "b": 2}`}},
											map[string]any{"key": "tool.result", "value": map[string]any{"stringValue": `{"result": 4}`}},
											map[string]any{"key": "tool.latency_ms", "value": map[string]any{"intValue": "5"}},
										},
									},
								},
								"status": map[string]any{"code": 1},
							},
						},
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal otlp payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ingestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create otlp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post otlp: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("otlp ingest status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

func waitForCapture(ctx context.Context, pool *pgxpool.Pool, traceID string) error {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		var count int
		err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tool_captures WHERE trace_id = $1`, traceID).Scan(&count)
		if err == nil && count > 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			if err != nil {
				return fmt.Errorf("query captures: %w", err)
			}
			return fmt.Errorf("timeout waiting for capture for trace %s", traceID)
		case <-ticker.C:
		}
	}
}

type staticHeadersRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (rt *staticHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	for key, value := range rt.headers {
		clone.Header.Set(key, value)
	}
	return rt.base.RoundTrip(clone)
}

func connectMCPWithHeaders(
	ctx context.Context,
	mcpURL string,
	headers map[string]string,
) (*mcp.ClientSession, error) {
	requestHeaders := map[string]string{}
	for key, value := range headers {
		requestHeaders[key] = value
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &staticHeadersRoundTripper{
			base:    http.DefaultTransport,
			headers: requestHeaders,
		},
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "cmdr-e2e",
		Version: "0.1.0",
	}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             mcpURL,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func connectMCP(ctx context.Context, mcpURL, traceID string) (*mcp.ClientSession, error) {
	headers := map[string]string{}
	if traceID != "" {
		headers["X-Freeze-Trace-ID"] = traceID
	}
	return connectMCPWithHeaders(ctx, mcpURL, headers)
}

func hasTool(tools []*mcp.Tool, toolName string) bool {
	for _, tool := range tools {
		if tool != nil && tool.Name == toolName {
			return true
		}
	}
	return false
}

func assertFrozenResult(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()

	if result == nil {
		t.Fatal("tools/call returned nil result")
	}
	if result.IsError {
		b, _ := json.Marshal(result)
		t.Fatalf("tools/call returned isError=true: %s", string(b))
	}
	if result.StructuredContent == nil {
		b, _ := json.Marshal(result)
		t.Fatalf("tools/call missing structuredContent: %s", string(b))
	}

	var structured struct {
		Result float64 `json:"result"`
	}
	raw, _ := json.Marshal(result.StructuredContent)
	if err := json.Unmarshal(raw, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v; payload=%s", err, string(raw))
	}
	if structured.Result != 4 {
		b, _ := json.Marshal(result)
		t.Fatalf("unexpected frozen result: got %v want 4; payload=%s", structured.Result, string(b))
	}
}
