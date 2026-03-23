package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
