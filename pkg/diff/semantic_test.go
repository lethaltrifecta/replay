package diff

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

func TestCompareToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		baseline []ToolCall
		variant  []ToolCall
		wantNil  bool
		wantMin  float64
		wantMax  float64
	}{
		{
			name:    "both empty - returns nil",
			wantNil: true,
		},
		{
			name: "identical sequences",
			baseline: []ToolCall{
				{Name: "get_user"}, {Name: "list_files"}, {Name: "read_file"},
			},
			variant: []ToolCall{
				{Name: "get_user"}, {Name: "list_files"}, {Name: "read_file"},
			},
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name: "reordered tools",
			baseline: []ToolCall{
				{Name: "get_user"}, {Name: "list_files"}, {Name: "read_file"},
			},
			variant: []ToolCall{
				{Name: "read_file"}, {Name: "get_user"}, {Name: "list_files"},
			},
			wantMin: 0.3,
			wantMax: 0.8,
		},
		{
			name: "added tool",
			baseline: []ToolCall{
				{Name: "get_user"}, {Name: "read_file"},
			},
			variant: []ToolCall{
				{Name: "get_user"}, {Name: "read_file"}, {Name: "delete_user"},
			},
			wantMin: 0.5,
			wantMax: 0.95,
		},
		{
			name: "completely different",
			baseline: []ToolCall{
				{Name: "get_user"}, {Name: "list_files"},
			},
			variant: []ToolCall{
				{Name: "delete_user"}, {Name: "create_file"},
			},
			wantMin: 0.0,
			wantMax: 0.01,
		},
		{
			name:     "baseline empty, variant has tools",
			baseline: []ToolCall{},
			variant: []ToolCall{
				{Name: "get_user"},
			},
			wantMin: 0.0,
			wantMax: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareToolCalls(tt.baseline, tt.variant)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.GreaterOrEqual(t, result.Score, tt.wantMin, "score %.4f < min %.4f", result.Score, tt.wantMin)
			assert.LessOrEqual(t, result.Score, tt.wantMax, "score %.4f > max %.4f", result.Score, tt.wantMax)
			assert.GreaterOrEqual(t, result.SequenceSimilarity, 0.0)
			assert.LessOrEqual(t, result.SequenceSimilarity, 1.0)
			assert.GreaterOrEqual(t, result.FrequencySimilarity, 0.0)
			assert.LessOrEqual(t, result.FrequencySimilarity, 1.0)
		})
	}
}

func TestCompareRiskProfiles(t *testing.T) {
	tests := []struct {
		name           string
		baseline       []ToolCall
		variant        []ToolCall
		wantNil        bool
		wantEscalation bool
		wantScoreMin   float64
		wantScoreMax   float64
	}{
		{
			name:    "both empty - returns nil",
			wantNil: true,
		},
		{
			name: "identical risk profiles",
			baseline: []ToolCall{
				{Name: "get_user", RiskClass: "read"},
				{Name: "list_files", RiskClass: "read"},
			},
			variant: []ToolCall{
				{Name: "get_user", RiskClass: "read"},
				{Name: "list_files", RiskClass: "read"},
			},
			wantEscalation: false,
			wantScoreMin:   1.0,
			wantScoreMax:   1.0,
		},
		{
			name: "read to write escalation",
			baseline: []ToolCall{
				{Name: "get_user", RiskClass: "read"},
				{Name: "list_files", RiskClass: "read"},
			},
			variant: []ToolCall{
				{Name: "get_user", RiskClass: "read"},
				{Name: "update_user", RiskClass: "write"},
			},
			wantEscalation: true,
			wantScoreMin:   0.5,
			wantScoreMax:   0.95,
		},
		{
			name: "read to destructive escalation",
			baseline: []ToolCall{
				{Name: "get_user", RiskClass: "read"},
				{Name: "list_files", RiskClass: "read"},
			},
			variant: []ToolCall{
				{Name: "delete_user", RiskClass: "destructive"},
				{Name: "delete_files", RiskClass: "destructive"},
			},
			wantEscalation: true,
			wantScoreMin:   0.0,
			wantScoreMax:   0.5,
		},
		{
			name: "de-escalation (write to read)",
			baseline: []ToolCall{
				{Name: "update_user", RiskClass: "write"},
				{Name: "create_file", RiskClass: "write"},
			},
			variant: []ToolCall{
				{Name: "get_user", RiskClass: "read"},
				{Name: "list_files", RiskClass: "read"},
			},
			wantEscalation: false,
			wantScoreMin:   0.5,
			wantScoreMax:   0.95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareRiskProfiles(tt.baseline, tt.variant)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.Equal(t, tt.wantEscalation, result.Escalation)
			assert.GreaterOrEqual(t, result.Score, tt.wantScoreMin, "score %.4f < min %.4f", result.Score, tt.wantScoreMin)
			assert.LessOrEqual(t, result.Score, tt.wantScoreMax, "score %.4f > max %.4f", result.Score, tt.wantScoreMax)
			assert.NotNil(t, result.Details)
		})
	}
}

func TestCompareResponses(t *testing.T) {
	tests := []struct {
		name     string
		baseline []*storage.ReplayTrace
		variant  []*storage.ReplayTrace
		wantNil  bool
		wantMin  float64
		wantMax  float64
	}{
		{
			name:    "both empty - returns nil",
			wantNil: true,
		},
		{
			name: "identical text",
			baseline: []*storage.ReplayTrace{
				{Completion: "The answer is 42."},
			},
			variant: []*storage.ReplayTrace{
				{Completion: "The answer is 42."},
			},
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name: "similar text",
			baseline: []*storage.ReplayTrace{
				{Completion: "The quick brown fox jumps over the lazy dog"},
			},
			variant: []*storage.ReplayTrace{
				{Completion: "The quick brown fox leaps over the lazy dog"},
			},
			wantMin: 0.6,
			wantMax: 0.99,
		},
		{
			name: "completely different",
			baseline: []*storage.ReplayTrace{
				{Completion: "Hello world foo bar"},
			},
			variant: []*storage.ReplayTrace{
				{Completion: "Completely unrelated response text"},
			},
			wantMin: 0.0,
			wantMax: 0.2,
		},
		{
			name: "empty completion vs text",
			baseline: []*storage.ReplayTrace{
				{Completion: ""},
			},
			variant: []*storage.ReplayTrace{
				{Completion: "Some response text"},
			},
			wantMin: 0.0,
			wantMax: 0.01,
		},
		{
			name: "mismatched step count penalized",
			baseline: []*storage.ReplayTrace{
				{Completion: "step one"},
				{Completion: "step two"},
				{Completion: "step three"},
			},
			variant: []*storage.ReplayTrace{
				{Completion: "step one"},
			},
			wantMin: 0.0,
			wantMax: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareResponses(tt.baseline, tt.variant)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.GreaterOrEqual(t, result.Score, tt.wantMin, "score %.4f < min %.4f", result.Score, tt.wantMin)
			assert.LessOrEqual(t, result.Score, tt.wantMax, "score %.4f > max %.4f", result.Score, tt.wantMax)
		})
	}
}

func TestClassifyToolRisk(t *testing.T) {
	tests := []struct {
		toolName string
		want     string
	}{
		{"delete_user", "destructive"},
		{"remove_file", "destructive"},
		{"drop_table", "destructive"},
		{"terminate_instance", "destructive"},
		{"create_user", "write"},
		{"update_record", "write"},
		{"insert_row", "write"},
		{"modify_config", "write"},
		{"edit_document", "write"},
		{"write_file", "write"},
		{"get_user", "read"},
		{"list_files", "read"},
		{"search_records", "read"},
		{"fetch_data", "read"},
		// Case insensitive
		{"DeleteUser", "destructive"},
		{"CREATE_ITEM", "write"},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyToolRisk(tt.toolName))
		})
	}
}

func TestExtractVariantToolCalls(t *testing.T) {
	tests := []struct {
		name  string
		steps []*storage.ReplayTrace
		want  int // expected number of tool calls
	}{
		{
			name: "metadata with tool_calls",
			steps: []*storage.ReplayTrace{
				{
					Metadata: storage.JSONB{
						"tool_calls": []interface{}{
							map[string]interface{}{
								"id":   "call_1",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_user",
									"arguments": `{"id": 1}`,
								},
							},
							map[string]interface{}{
								"id":   "call_2",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "delete_user",
									"arguments": `{"id": 2}`,
								},
							},
						},
					},
				},
			},
			want: 2,
		},
		{
			name: "metadata without tool_calls",
			steps: []*storage.ReplayTrace{
				{
					Metadata: storage.JSONB{"source": "replay"},
				},
			},
			want: 0,
		},
		{
			name: "malformed tool_calls - not a slice",
			steps: []*storage.ReplayTrace{
				{
					Metadata: storage.JSONB{"tool_calls": "not a slice"},
				},
			},
			want: 0,
		},
		{
			name: "malformed function - missing name",
			steps: []*storage.ReplayTrace{
				{
					Metadata: storage.JSONB{
						"tool_calls": []interface{}{
							map[string]interface{}{
								"id":   "call_1",
								"type": "function",
								"function": map[string]interface{}{
									"arguments": `{}`,
								},
							},
						},
					},
				},
			},
			want: 0,
		},
		{
			name: "multiple steps with tool_calls",
			steps: []*storage.ReplayTrace{
				{
					Metadata: storage.JSONB{
						"tool_calls": []interface{}{
							map[string]interface{}{
								"function": map[string]interface{}{"name": "get_user"},
							},
						},
					},
				},
				{
					Metadata: storage.JSONB{
						"tool_calls": []interface{}{
							map[string]interface{}{
								"function": map[string]interface{}{"name": "update_user"},
							},
						},
					},
				},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractVariantToolCalls(tt.steps)
			assert.Len(t, result, tt.want)
		})
	}
}

func TestExtractVariantToolCalls_RiskClassification(t *testing.T) {
	steps := []*storage.ReplayTrace{
		{
			Metadata: storage.JSONB{
				"tool_calls": []interface{}{
					map[string]interface{}{
						"function": map[string]interface{}{"name": "get_user"},
					},
					map[string]interface{}{
						"function": map[string]interface{}{"name": "delete_user"},
					},
					map[string]interface{}{
						"function": map[string]interface{}{"name": "create_file"},
					},
				},
			},
		},
	}

	result := ExtractVariantToolCalls(steps)
	require.Len(t, result, 3)
	assert.Equal(t, "read", result[0].RiskClass)
	assert.Equal(t, "destructive", result[1].RiskClass)
	assert.Equal(t, "write", result[2].RiskClass)
}

func TestBaselineToolCalls(t *testing.T) {
	captures := []*storage.ToolCapture{
		{ToolName: "get_user", RiskClass: "read"},
		{ToolName: "update_record", RiskClass: "write"},
		{ToolName: "delete_file", RiskClass: "destructive"},
	}

	result := BaselineToolCalls(captures)
	require.Len(t, result, 3)
	assert.Equal(t, "get_user", result[0].Name)
	assert.Equal(t, "read", result[0].RiskClass)
	assert.Equal(t, "update_record", result[1].Name)
	assert.Equal(t, "write", result[1].RiskClass)
	assert.Equal(t, "delete_file", result[2].Name)
	assert.Equal(t, "destructive", result[2].RiskClass)
}

func TestBaselineToolCalls_Nil(t *testing.T) {
	result := BaselineToolCalls(nil)
	assert.Len(t, result, 0)
}

func TestExtractVariantToolCalls_ReplayMetadataRoundTrip(t *testing.T) {
	steps := []*storage.ReplayTrace{
		{
			Metadata: storage.JSONB{
				"tool_calls": []agwclient.ToolCallResponse{
					{
						ID:   "call_1",
						Type: "function",
						Function: agwclient.FunctionCall{
							Name:      "create_file",
							Arguments: `{"path":"/tmp/a.txt"}`,
						},
					},
					{
						ID:   "call_2",
						Type: "function",
						Function: agwclient.FunctionCall{
							Name:      "delete_file",
							Arguments: `{"path":"/tmp/b.txt"}`,
						},
					},
				},
			},
		},
	}

	// Simulate DB JSONB round-trip, which converts typed slices/maps to
	// []interface{} / map[string]interface{}.
	payload, err := json.Marshal(steps[0].Metadata)
	require.NoError(t, err)

	var roundTripped storage.JSONB
	require.NoError(t, json.Unmarshal(payload, &roundTripped))
	steps[0].Metadata = roundTripped

	result := ExtractVariantToolCalls(steps)
	require.Len(t, result, 2)
	assert.Equal(t, "create_file", result[0].Name)
	assert.Equal(t, "write", result[0].RiskClass)
	assert.Equal(t, "delete_file", result[1].Name)
	assert.Equal(t, "destructive", result[1].RiskClass)
}

func TestJaccard(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want float64
	}{
		{"identical", "hello world", "hello world", 1.0},
		{"empty both", "", "", 1.0},
		{"one empty", "hello", "", 0.0},
		{"no overlap", "hello world", "foo bar", 0.0},
		{"partial overlap", "the quick brown fox", "the slow brown cat", 0.4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jaccard(tt.a, tt.b)
			assert.InDelta(t, tt.want, result, 0.1)
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want float64
	}{
		{"both empty", nil, nil, 0.0},
		{"one empty", []string{"a"}, nil, 1.0},
		{"identical", []string{"a", "b", "c"}, []string{"a", "b", "c"}, 0.0},
		{"completely different", []string{"a", "b"}, []string{"c", "d"}, 1.0},
		{"one substitution", []string{"a", "b", "c"}, []string{"a", "x", "c"}, 1.0 / 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := levenshteinDistance(tt.a, tt.b)
			assert.InDelta(t, tt.want, result, 0.01)
		})
	}
}

func TestCosineDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b map[string]int
		want float64
	}{
		{"both empty", nil, nil, 0.0},
		{"one empty", map[string]int{"a": 1}, nil, 1.0},
		{"identical", map[string]int{"a": 2, "b": 3}, map[string]int{"a": 2, "b": 3}, 0.0},
		{"orthogonal", map[string]int{"a": 1}, map[string]int{"b": 1}, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineDistance(tt.a, tt.b)
			assert.InDelta(t, tt.want, result, 0.01)
		})
	}
}
