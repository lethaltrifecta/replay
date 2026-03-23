package mcpclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Client struct {
	session *mcp.ClientSession
}

type ToolResult struct {
	StructuredContent any
	IsError           bool
}

func Connect(ctx context.Context, endpoint string, httpClient *http.Client) (*Client, error) {
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "migration-demo-agent",
		Version: "1.0.0",
	}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: httpClient,
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("client.Connect(): %w", err)
	}
	return &Client{session: session}, nil
}

func (c *Client) Close() error {
	return c.session.Close()
}

func (c *Client) ListToolNames(ctx context.Context) ([]string, error) {
	tools, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("session.ListTools(): %w", err)
	}

	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	return names, nil
}

func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (*ToolResult, error) {
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("session.CallTool(%q): %w", name, err)
	}
	return &ToolResult{
		StructuredContent: result.StructuredContent,
		IsError:           result.IsError,
	}, nil
}
