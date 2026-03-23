package replay

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolExecutor abstracts tool execution for the agent loop.
type ToolExecutor interface {
	CallTool(ctx context.Context, toolName string, args map[string]any) (ToolResult, error)
	Close() error
}

// ToolResult holds the outcome of a single tool call.
type ToolResult struct {
	Content   string
	IsError   bool
	ErrorType string
	LatencyMS int
}

// MCPToolExecutor implements ToolExecutor via an MCP client session.
type MCPToolExecutor struct {
	session *mcp.ClientSession
}

// NewMCPToolExecutor creates an MCP-backed ToolExecutor.
// headers should carry freeze-scoping headers like X-Freeze-Trace-ID.
func NewMCPToolExecutor(ctx context.Context, mcpURL string, headers map[string]string) (*MCPToolExecutor, error) {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &staticHeadersRoundTripper{
			base:    http.DefaultTransport,
			headers: headers,
		},
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "cmdr-replay",
		Version: "0.1.0",
	}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             mcpURL,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect: %w", err)
	}

	return &MCPToolExecutor{session: session}, nil
}

// CallTool invokes a tool via the MCP session and returns the result.
func (m *MCPToolExecutor) CallTool(ctx context.Context, toolName string, args map[string]any) (ToolResult, error) {
	start := time.Now()

	mcpResult, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	latencyMS := int(time.Since(start).Milliseconds())

	if err != nil {
		return ToolResult{LatencyMS: latencyMS}, fmt.Errorf("mcp call_tool %s: %w", toolName, err)
	}

	// Extract text content (human-readable message for errors, JSON result for success)
	content := ""
	if len(mcpResult.Content) > 0 {
		if tc, ok := mcpResult.Content[0].(*mcp.TextContent); ok {
			content = tc.Text
		}
	}

	result := ToolResult{
		Content:   content,
		IsError:   mcpResult.IsError,
		LatencyMS: latencyMS,
	}

	// freeze-mcp puts the error type in StructuredContent, not in Content[0].Text
	if mcpResult.IsError {
		result.ErrorType = extractFreezeErrorType(mcpResult.StructuredContent)
	}

	return result, nil
}

// Close terminates the MCP session.
func (m *MCPToolExecutor) Close() error {
	if m.session != nil {
		return m.session.Close()
	}
	return nil
}

// extractFreezeErrorType extracts the error type from freeze-mcp's StructuredContent.
// freeze-mcp returns StructuredContent as: {"error":{"type":"tool_not_captured","message":"..."}}
func extractFreezeErrorType(structured any) string {
	m, ok := structured.(map[string]any)
	if !ok {
		return ""
	}
	errObj, ok := m["error"].(map[string]any)
	if !ok {
		return ""
	}
	errType, _ := errObj["type"].(string)
	return errType
}

// staticHeadersRoundTripper injects static headers into every outbound request.
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
