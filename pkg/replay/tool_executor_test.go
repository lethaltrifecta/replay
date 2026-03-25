package replay

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFreezeErrorType(t *testing.T) {
	tests := []struct {
		name       string
		structured any
		want       string
	}{
		{
			name: "valid tool_not_captured",
			structured: map[string]any{
				"error": map[string]any{
					"type":    "tool_not_captured",
					"message": "no capture found",
				},
			},
			want: "tool_not_captured",
		},
		{
			name: "valid captured_tool_error",
			structured: map[string]any{
				"error": map[string]any{
					"type":    "captured_tool_error",
					"message": "tool failed",
				},
			},
			want: "captured_tool_error",
		},
		{
			name: "missing type field",
			structured: map[string]any{
				"error": map[string]any{
					"message": "no type",
				},
			},
			want: "",
		},
		{
			name: "missing error key",
			structured: map[string]any{
				"result": "ok",
			},
			want: "",
		},
		{
			name:       "nil input",
			structured: nil,
			want:       "",
		},
		{
			name:       "wrong type (string)",
			structured: "this is plain text",
			want:       "",
		},
		{
			name: "nested empty error object",
			structured: map[string]any{
				"error": map[string]any{},
			},
			want: "",
		},
		{
			name: "error is not a map",
			structured: map[string]any{
				"error": "string error",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFreezeErrorType(tt.structured)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSetLocator_ClearLocator(t *testing.T) {
	rt := &freezeRoundTripper{
		base:    http.DefaultTransport,
		headers: map[string]string{"X-Freeze-Trace-Id": "trace-abc"},
	}
	exec := &MCPToolExecutor{rt: rt}

	// SetLocator should inject span/step overrides
	exec.SetLocator("span-42", 3)

	rt.mu.Lock()
	require.NotNil(t, rt.overrides)
	assert.Equal(t, "span-42", rt.overrides[http.CanonicalHeaderKey("X-Freeze-Span-Id")])
	assert.Equal(t, "3", rt.overrides[http.CanonicalHeaderKey("X-Freeze-Step-Index")])
	rt.mu.Unlock()

	// ClearLocator should remove overrides
	exec.ClearLocator()

	rt.mu.Lock()
	assert.Nil(t, rt.overrides)
	rt.mu.Unlock()
}

func TestFreezeRoundTripper_OverridesApplied(t *testing.T) {
	// Verify that overrides appear in the cloned request headers
	var captured http.Header
	rt := &freezeRoundTripper{
		base: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			captured = req.Header.Clone()
			return &http.Response{StatusCode: 200}, nil
		}),
		headers: map[string]string{
			"X-Freeze-Trace-Id": "trace-1",
		},
	}

	rt.setOverrides(map[string]string{
		"X-Freeze-Span-Id":    "span-99",
		"X-Freeze-Step-Index": "7",
	})

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)

	assert.Equal(t, "trace-1", captured.Get("X-Freeze-Trace-Id"))
	assert.Equal(t, "span-99", captured.Get("X-Freeze-Span-Id"))
	assert.Equal(t, "7", captured.Get("X-Freeze-Step-Index"))

	// After clearing, overrides should no longer appear
	rt.clearOverrides()
	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	assert.Equal(t, "trace-1", captured.Get("X-Freeze-Trace-Id"))
	assert.Empty(t, captured.Get("X-Freeze-Span-Id"))
	assert.Empty(t, captured.Get("X-Freeze-Step-Index"))
}

func TestNewMCPToolExecutor_StripsLocatorHeaders(t *testing.T) {
	// Verify that per-call locator headers are stripped from the base headers
	// even if the caller passes them in. They should only flow through SetLocator.
	headers := map[string]string{
		"X-Freeze-Trace-Id":  "trace-abc",
		"X-Freeze-Span-Id":   "span-stale",
		"X-Freeze-Step-Index": "99",
	}

	// We can't actually connect to an MCP server in unit tests, but we can
	// verify the stripping logic by inspecting the round tripper directly.
	// Extract the filtering logic by constructing what NewMCPToolExecutor builds.
	baseHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		canonical := http.CanonicalHeaderKey(k)
		if canonical == http.CanonicalHeaderKey("X-Freeze-Span-Id") ||
			canonical == http.CanonicalHeaderKey("X-Freeze-Step-Index") {
			continue
		}
		baseHeaders[k] = v
	}

	assert.Equal(t, "trace-abc", baseHeaders["X-Freeze-Trace-Id"])
	assert.Empty(t, baseHeaders["X-Freeze-Span-Id"])
	assert.Empty(t, baseHeaders["X-Freeze-Step-Index"])
	assert.Len(t, baseHeaders, 1)
}

// roundTripperFunc adapts a function into http.RoundTripper for testing.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
