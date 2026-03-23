package demohelper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lethaltrifecta/replay/pkg/otelreceiver"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

type TraceDetail struct {
	Steps        []json.RawMessage `json:"steps"`
	ToolCaptures []struct {
		Error  string          `json:"error"`
		Result json.RawMessage `json:"result"`
	} `json:"toolCaptures"`
}

type Summary struct {
	Output              string
	BaselineTraceID     string
	SafeReplayTraceID   string
	UnsafeReplayTraceID string
	RunLog              string
	CMDRLog             string
	FreezeLog           string
	MigrationMCPLog     string
	MockLLMLog          string
	CaptureAGWLog       string
	ReplayAGWLog        string
	SafeVerdictLog      string
	UnsafeVerdictLog    string
}

func RandomHex(byteCount int) (string, error) {
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func WaitPostgres(rawURL string, timeout time.Duration) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse postgres url: %w", err)
	}
	host := parsed.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}

	deadline := time.Now().Add(timeout)
	address := net.JoinHostPort(host, port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timed out waiting for postgres at %s", address)
}

func WaitTrace(apiURL, traceID string, expectedSteps, expectedTools int, errorSubstring string, timeout time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	traceURL := strings.TrimRight(apiURL, "/") + "/traces/" + url.PathEscape(traceID)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(traceURL)
		if err == nil {
			matched, traceErr := traceMatches(resp, expectedSteps, expectedTools, errorSubstring)
			if traceErr == nil && matched {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timed out waiting for trace %s", traceID)
}

func WaitTraceContent(apiURL, promptText, completionText string, limit int, timeout time.Duration) (map[string]any, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := strings.TrimRight(apiURL, "/")
	listURL := fmt.Sprintf("%s/traces?limit=%d", baseURL, limit)
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		tracesResp, err := client.Get(listURL)
		if err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		if tracesResp.StatusCode != http.StatusOK {
			_ = tracesResp.Body.Close()
			lastErr = fmt.Errorf("list traces status=%d", tracesResp.StatusCode)
			time.Sleep(time.Second)
			continue
		}

		var traces []map[string]any
		if err := json.NewDecoder(tracesResp.Body).Decode(&traces); err != nil {
			_ = tracesResp.Body.Close()
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		_ = tracesResp.Body.Close()

		for _, trace := range traces {
			traceID, _ := trace["traceId"].(string)
			if traceID == "" {
				continue
			}
			resp, err := client.Get(baseURL + "/traces/" + url.PathEscape(traceID))
			if err != nil {
				lastErr = err
				continue
			}
			matched, detail, err := traceContentMatches(resp, promptText, completionText)
			if err != nil {
				lastErr = err
				continue
			}
			if matched {
				return detail, nil
			}
		}

		time.Sleep(time.Second)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("timed out waiting for matching trace: %w", lastErr)
	}
	return nil, errors.New("timed out waiting for matching trace")
}

func SeedToolCapture(ctx context.Context, postgresURL, traceID, toolName string, toolArgsJSON, toolResultJSON string) error {
	store, err := storage.NewPostgresStorage(ctx, postgresURL, 4)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	var toolArgs storage.JSONB
	if err := json.Unmarshal([]byte(toolArgsJSON), &toolArgs); err != nil {
		return fmt.Errorf("decode --tool-args: %w", err)
	}
	var toolResult storage.JSONB
	if err := json.Unmarshal([]byte(toolResultJSON), &toolResult); err != nil {
		return fmt.Errorf("decode --tool-result: %w", err)
	}

	spanSuffix, err := RandomHex(6)
	if err != nil {
		return err
	}

	capture := &storage.ToolCapture{
		TraceID:   traceID,
		SpanID:    "seed-span-" + spanSuffix,
		StepIndex: int(time.Now().Unix()) % 100000,
		ToolName:  toolName,
		Args:      toolArgs,
		ArgsHash:  otelreceiver.CalculateCaptureArgsHash(toolArgs),
		Result:    toolResult,
		LatencyMS: 5,
		RiskClass: otelreceiver.DetermineCaptureRiskClass(toolName, toolArgs),
		CreatedAt: time.Now().UTC(),
	}

	if err := store.CreateToolCapture(ctx, capture); err != nil {
		return fmt.Errorf("create tool capture: %w", err)
	}
	return nil
}

func WriteSummary(summary Summary) error {
	payload := map[string]any{
		"scenario":               "database-migration",
		"baseline_trace_id":      summary.BaselineTraceID,
		"safe_replay_trace_id":   summary.SafeReplayTraceID,
		"unsafe_replay_trace_id": summary.UnsafeReplayTraceID,
		"logs": map[string]string{
			"run_log":              summary.RunLog,
			"cmdr":                 summary.CMDRLog,
			"freeze_mcp":           summary.FreezeLog,
			"migration_mcp":        summary.MigrationMCPLog,
			"mock_llm":             summary.MockLLMLog,
			"capture_agentgateway": summary.CaptureAGWLog,
			"replay_agentgateway":  summary.ReplayAGWLog,
			"safe_verdict":         summary.SafeVerdictLog,
			"unsafe_verdict":       summary.UnsafeVerdictLog,
		},
	}

	if err := os.MkdirAll(filepath.Dir(summary.Output), 0o755); err != nil {
		return fmt.Errorf("create summary dir: %w", err)
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(summary.Output, body, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

func traceMatches(resp *http.Response, expectedSteps int, expectedTools int, errorSubstring string) (bool, error) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	var detail TraceDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return false, err
	}
	if len(detail.Steps) != expectedSteps || len(detail.ToolCaptures) != expectedTools {
		return false, nil
	}
	if errorSubstring == "" {
		return true, nil
	}
	for _, capture := range detail.ToolCaptures {
		if strings.Contains(capture.Error, errorSubstring) {
			return true, nil
		}
		if strings.Contains(string(capture.Result), errorSubstring) {
			return true, nil
		}
	}
	return false, nil
}

func traceContentMatches(resp *http.Response, promptText string, completionText string) (bool, map[string]any, error) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil, nil
	}

	var detail map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return false, nil, err
	}

	steps, _ := detail["steps"].([]any)
	foundPrompt := false
	foundCompletion := false
	for _, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		if completion, _ := step["completion"].(string); completion == completionText {
			foundCompletion = true
		}
		prompt, _ := step["prompt"].(map[string]any)
		messages, _ := prompt["messages"].([]any)
		for _, rawMessage := range messages {
			message, ok := rawMessage.(map[string]any)
			if !ok {
				continue
			}
			if content, _ := message["content"].(string); content == promptText {
				foundPrompt = true
				break
			}
		}
	}
	return foundPrompt && foundCompletion, detail, nil
}
