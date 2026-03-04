package agwclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// ClientConfig holds configuration for the agentgateway HTTP client.
type ClientConfig struct {
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
}

// Client is an HTTP client for the agentgateway OpenAI-compatible API.
type Client struct {
	httpClient *http.Client
	config     ClientConfig
}

// ToolCallResponse represents a tool call in an OpenAI-compatible API response.
type ToolCallResponse struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents the function invocation within a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string
}

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role      string             `json:"role"`
	Content   string             `json:"content"`
	ToolCalls []ToolCallResponse `json:"tool_calls,omitempty"`
}

// CompletionRequest is the request body for the chat completions endpoint.
type CompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
}

// CompletionResponse is the response body from the chat completions endpoint.
type CompletionResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a single completion choice.
type Choice struct {
	Index   int         `json:"index"`
	Message ChatMessage `json:"message"`
}

// Usage contains token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// NewClient creates a new agentgateway client.
func NewClient(cfg ClientConfig) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		config: cfg,
	}
}

// Complete sends a chat completion request to agentgateway.
// Retries on 429/5xx with exponential backoff. Fails immediately on 4xx (non-429).
func (c *Client) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Force non-streaming
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	baseURL := strings.TrimRight(c.config.BaseURL, "/")
	url := baseURL + "/v1/chat/completions"
	maxAttempts := c.config.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := c.doRequest(ctx, url, body)
		if err != nil {
			// Non-retryable errors (4xx except 429) fail immediately
			if _, ok := err.(*NonRetryableError); ok {
				return nil, err
			}
			lastErr = err
			// Context errors are not retryable
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("agentgateway request failed after %d attempts: %w", maxAttempts, lastErr)
}

// doRequest performs a single HTTP request and returns the parsed response or an error.
// Returns a retryable error for 429/5xx, a non-retryable error for other 4xx.
func (c *Client) doRequest(ctx context.Context, url string, body []byte) (*CompletionResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode >= 200 && httpResp.StatusCode < 300 {
		var result CompletionResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
		return &result, nil
	}

	errMsg := fmt.Sprintf("status %d: %s", httpResp.StatusCode, string(respBody))

	// 429 or 5xx: retryable
	if httpResp.StatusCode == 429 || httpResp.StatusCode >= 500 {
		return nil, fmt.Errorf("retryable error: %s", errMsg)
	}

	// Other 4xx: non-retryable, wrap with a sentinel so caller can distinguish
	return nil, &NonRetryableError{StatusCode: httpResp.StatusCode, Body: string(respBody)}
}

// NonRetryableError indicates a client error that should not be retried.
type NonRetryableError struct {
	StatusCode int
	Body       string
}

func (e *NonRetryableError) Error() string {
	return fmt.Sprintf("non-retryable error (status %d): %s", e.StatusCode, e.Body)
}
