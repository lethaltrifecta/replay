package freezeloop

import demollm "github.com/lethaltrifecta/replay/internal/demoagent/llm"

const (
	DefaultPrompt            = "Use the calculator to add 5 and 3."
	DefaultModel             = "mock-toolloop-model"
	DefaultToolName          = "calculator"
	DefaultCompletionText    = "I will use the calculator."
	DefaultExpectedSubstring = "8"
	DefaultServiceName       = "freeze-loop-capture"
	DefaultProvider          = "mock-openai"
	DefaultOperation         = "add"
	DefaultToolCallID        = "call_calculator_1"
	DefaultToolDescription   = "Perform basic arithmetic operations."
)

func ToolDefinitions() []demollm.ToolDefinition {
	return []demollm.ToolDefinition{
		{
			"type": "function",
			"function": map[string]any{
				"name":        DefaultToolName,
				"description": DefaultToolDescription,
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"operation": map[string]any{"type": "string"},
						"a":         map[string]any{"type": "number"},
						"b":         map[string]any{"type": "number"},
					},
					"required": []string{"operation", "a", "b"},
				},
			},
		},
	}
}

func DefaultToolArgs() map[string]any {
	return map[string]any{
		"operation": DefaultOperation,
		"a":         float64(5),
		"b":         float64(3),
	}
}

func DefaultToolResult() map[string]any {
	return map[string]any{"result": float64(8)}
}
