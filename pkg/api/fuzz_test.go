package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

func FuzzGetTracePreservesPromptToolFields(f *testing.F) {
	f.Add("user", "hello", "planner", "call-1", "calculator", "function", "calculator", true, true, true, true)
	f.Add("system", "be careful", "", "", "browser", "auto", "browser", false, false, true, true)
	f.Add("assistant", "", "", "", "", "", "", false, false, false, false)

	f.Fuzz(func(t *testing.T, role, content, name, toolCallID, toolName, toolChoiceType, toolChoiceFunction string, includeName, includeToolCalls, includeTools, includeToolChoice bool) {
		store := newMockStorage()

		message := map[string]any{
			"role":    role,
			"content": content,
		}
		if includeName {
			message["name"] = name
		}

		var expectedToolCalls []map[string]any
		if includeToolCalls {
			expectedToolCalls = []map[string]any{{
				"id":   toolCallID,
				"type": "function",
				"function": map[string]any{
					"name":      toolName,
					"arguments": "{\"value\":1}",
				},
			}}
			message["tool_calls"] = toAnySlice(expectedToolCalls)
		}

		prompt := storage.JSONB{
			"messages": []any{message},
		}

		var expectedTools []map[string]any
		if includeTools {
			expectedTools = []map[string]any{{
				"type": "function",
				"function": map[string]any{
					"name":        toolName,
					"description": "tool description",
				},
			}}
			prompt["tools"] = toAnySlice(expectedTools)
		}

		var expectedToolChoice any
		if includeToolChoice {
			expectedToolChoice = map[string]any{
				"type": toolChoiceType,
				"function": map[string]any{
					"name": toolChoiceFunction,
				},
			}
			prompt["tool_choice"] = expectedToolChoice
		}

		store.replayTraces["trace-fuzz"] = []*storage.ReplayTrace{{
			TraceID:          "trace-fuzz",
			SpanID:           uuid.NewString(),
			RunID:            "trace-fuzz",
			StepIndex:        0,
			CreatedAt:        time.Now(),
			Provider:         "openai",
			Model:            "gpt-4o",
			Prompt:           prompt,
			Completion:       "ok",
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
			LatencyMS:        20,
		}}

		srv := newTestServer(t, store, nil, 5)
		recorder := jsonRequest(t, srv, http.MethodGet, "/api/v1/traces/trace-fuzz", nil)
		require.Equal(t, http.StatusOK, recorder.Code)

		detail := decodeJSON[TraceDetail](t, recorder)
		require.NotNil(t, detail.Steps)
		require.Len(t, *detail.Steps, 1)
		require.NotNil(t, (*detail.Steps)[0].Prompt)

		gotPrompt := (*detail.Steps)[0].Prompt
		require.NotNil(t, gotPrompt.Messages)
		require.Len(t, *gotPrompt.Messages, 1)

		gotMessage := (*gotPrompt.Messages)[0]
		require.NotNil(t, gotMessage.Role)
		assert.Equal(t, role, *gotMessage.Role)
		require.NotNil(t, gotMessage.Content)
		assert.Equal(t, content, *gotMessage.Content)

		if includeName {
			require.NotNil(t, gotMessage.Name)
			assert.Equal(t, name, *gotMessage.Name)
		} else {
			assert.Nil(t, gotMessage.Name)
		}

		if includeToolCalls {
			require.NotNil(t, gotMessage.ToolCalls)
			assertJSONEqual(t, expectedToolCalls, *gotMessage.ToolCalls)
		} else {
			assert.Nil(t, gotMessage.ToolCalls)
		}

		if includeTools {
			require.NotNil(t, gotPrompt.Tools)
			assertJSONEqual(t, expectedTools, *gotPrompt.Tools)
		} else {
			assert.Nil(t, gotPrompt.Tools)
		}

		if includeToolChoice {
			require.NotNil(t, gotPrompt.ToolChoice)
			assertJSONEqual(t, expectedToolChoice, gotPrompt.ToolChoice)
		} else {
			assert.Nil(t, gotPrompt.ToolChoice)
		}
	})
}

func assertJSONEqual(t *testing.T, want, got any) {
	t.Helper()
	require.JSONEq(t, mustJSONString(t, want), mustJSONString(t, got))
}

func mustJSONString(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	require.NoError(t, err)
	return string(payload)
}

func toAnySlice(items []map[string]any) []any {
	values := make([]any, 0, len(items))
	for _, item := range items {
		values = append(values, item)
	}
	return values
}
