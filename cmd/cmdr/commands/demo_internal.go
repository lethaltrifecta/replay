package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/internal/demohelper"
	"github.com/lethaltrifecta/replay/internal/freezeloop"
	"github.com/lethaltrifecta/replay/internal/migrationdemo"
)

var demoInternalCmd = &cobra.Command{
	Use:    "internal",
	Short:  "Internal demo helpers for repo harnesses",
	Hidden: true,
}

var demoInternalHelperCmd = &cobra.Command{
	Use:    "helper",
	Short:  "Internal helper utilities",
	Hidden: true,
}

var demoInternalRandomHexCmd = &cobra.Command{
	Use:    "random-hex",
	Short:  "Generate a random hex string",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		byteCount, _ := cmd.Flags().GetInt("bytes")
		value, err := demohelper.RandomHex(byteCount)
		if err != nil {
			return err
		}
		cmd.Println(value)
		return nil
	},
}

var demoInternalWaitPostgresCmd = &cobra.Command{
	Use:    "wait-postgres",
	Short:  "Wait for Postgres to accept TCP connections",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		rawURL, _ := cmd.Flags().GetString("url")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		if rawURL == "" {
			return errors.New("--url is required")
		}
		return demohelper.WaitPostgres(rawURL, timeout)
	},
}

var demoInternalWaitTraceCmd = &cobra.Command{
	Use:    "wait-trace",
	Short:  "Wait for a trace to appear in the API",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiURL, _ := cmd.Flags().GetString("api-url")
		traceID, _ := cmd.Flags().GetString("trace-id")
		expectedSteps, _ := cmd.Flags().GetInt("steps")
		expectedTools, _ := cmd.Flags().GetInt("tools")
		errorSubstring, _ := cmd.Flags().GetString("error-substring")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		if apiURL == "" || traceID == "" {
			return errors.New("--api-url and --trace-id are required")
		}
		return demohelper.WaitTrace(apiURL, traceID, expectedSteps, expectedTools, errorSubstring, timeout)
	},
}

var demoInternalWaitTraceContentCmd = &cobra.Command{
	Use:    "wait-trace-content",
	Short:  "Wait for a trace containing a specific prompt/completion pair",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiURL, _ := cmd.Flags().GetString("api-url")
		promptText, _ := cmd.Flags().GetString("prompt")
		completionText, _ := cmd.Flags().GetString("completion")
		limit, _ := cmd.Flags().GetInt("limit")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		if apiURL == "" || promptText == "" || completionText == "" {
			return errors.New("--api-url, --prompt, and --completion are required")
		}
		detail, err := demohelper.WaitTraceContent(apiURL, promptText, completionText, limit, timeout)
		if err != nil {
			return err
		}
		body, err := json.MarshalIndent(detail, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(body))
		return nil
	},
}

var demoInternalSeedToolCaptureCmd = &cobra.Command{
	Use:    "seed-tool-capture",
	Short:  "Insert a tool capture directly into Postgres",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		postgresURL, _ := cmd.Flags().GetString("postgres-url")
		traceID, _ := cmd.Flags().GetString("trace-id")
		toolName, _ := cmd.Flags().GetString("tool-name")
		toolArgsJSON, _ := cmd.Flags().GetString("tool-args")
		toolResultJSON, _ := cmd.Flags().GetString("tool-result")
		if postgresURL == "" || traceID == "" || toolName == "" {
			return errors.New("--postgres-url, --trace-id, and --tool-name are required")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()
		if err := demohelper.SeedToolCapture(ctx, postgresURL, traceID, toolName, toolArgsJSON, toolResultJSON); err != nil {
			return err
		}
		cmd.Printf("seeded tool capture trace_id=%s\n", traceID)
		return nil
	},
}

var demoInternalWriteSummaryCmd = &cobra.Command{
	Use:    "write-summary",
	Short:  "Write the migration demo summary file",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		if output == "" {
			return errors.New("--output is required")
		}
		return demohelper.WriteSummary(demohelper.Summary{
			Output:              output,
			BaselineTraceID:     mustFlagString(cmd, "baseline-trace-id"),
			SafeReplayTraceID:   mustFlagString(cmd, "safe-replay-trace-id"),
			UnsafeReplayTraceID: mustFlagString(cmd, "unsafe-replay-trace-id"),
			RunLog:              mustFlagString(cmd, "run-log"),
			CMDRLog:             mustFlagString(cmd, "cmdr-log"),
			FreezeLog:           mustFlagString(cmd, "freeze-log"),
			MigrationMCPLog:     mustFlagString(cmd, "migration-mcp-log"),
			MockLLMLog:          mustFlagString(cmd, "mock-llm-log"),
			CaptureAGWLog:       mustFlagString(cmd, "capture-agentgateway-log"),
			ReplayAGWLog:        mustFlagString(cmd, "replay-agentgateway-log"),
			SafeVerdictLog:      mustFlagString(cmd, "safe-verdict-log"),
			UnsafeVerdictLog:    mustFlagString(cmd, "unsafe-verdict-log"),
		})
	},
}

var demoInternalMigrationAgentCmd = &cobra.Command{
	Use:    "migration-agent",
	Short:  "Run the internal migration demo agent loop",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := migrationAgentConfigFromCommand(cmd)
		if err != nil {
			return err
		}
		return migrationdemo.RunAgent(cmd.Context(), cfg)
	},
}

var demoInternalFreezeCaptureCmd = &cobra.Command{
	Use:    "freeze-capture",
	Short:  "Emit a baseline capture for the freeze loop demo",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		traceID, _ := cmd.Flags().GetString("trace-id")
		if traceID == "" {
			return errors.New("--trace-id is required")
		}
		toolArgsJSON, _ := cmd.Flags().GetString("tool-args")
		toolResultJSON, _ := cmd.Flags().GetString("tool-result")
		var toolArgs map[string]any
		if err := json.Unmarshal([]byte(toolArgsJSON), &toolArgs); err != nil {
			return fmt.Errorf("decode --tool-args: %w", err)
		}
		var toolResult any
		if err := json.Unmarshal([]byte(toolResultJSON), &toolResult); err != nil {
			return fmt.Errorf("decode --tool-result: %w", err)
		}
		return freezeloop.RunCapture(cmd.Context(), freezeloop.CaptureConfig{
			OTLPURL:     mustFlagString(cmd, "otlp-url"),
			TraceID:     traceID,
			Model:       mustFlagString(cmd, "model"),
			Provider:    mustFlagString(cmd, "provider"),
			Prompt:      mustFlagString(cmd, "prompt"),
			Completion:  mustFlagString(cmd, "completion"),
			ToolName:    mustFlagString(cmd, "tool-name"),
			ToolArgs:    toolArgs,
			ToolResult:  toolResult,
			ServiceName: mustFlagString(cmd, "service-name"),
		})
	},
}

var demoInternalFreezeLoopCmd = &cobra.Command{
	Use:    "freeze-loop",
	Short:  "Run the internal freeze loop demo agent",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		freezeTraceID, _ := cmd.Flags().GetString("freeze-trace-id")
		if freezeTraceID == "" {
			return errors.New("--freeze-trace-id is required")
		}
		return freezeloop.RunLoop(cmd.Context(), freezeloop.LoopConfig{
			LLMURL:          mustFlagString(cmd, "llm-url"),
			MCPURL:          mustFlagString(cmd, "mcp-url"),
			FreezeTraceID:   freezeTraceID,
			Model:           mustFlagString(cmd, "model"),
			Prompt:          mustFlagString(cmd, "prompt"),
			MaxTurns:        mustFlagInt(cmd, "max-turns"),
			ExpectSubstring: mustFlagString(cmd, "expect-substring"),
		})
	},
}

func init() {
	demoCmd.AddCommand(demoInternalCmd)
	demoInternalCmd.AddCommand(demoInternalHelperCmd)
	demoInternalCmd.AddCommand(demoInternalMigrationAgentCmd)
	demoInternalCmd.AddCommand(demoInternalFreezeCaptureCmd)
	demoInternalCmd.AddCommand(demoInternalFreezeLoopCmd)

	demoInternalHelperCmd.AddCommand(demoInternalRandomHexCmd)
	demoInternalHelperCmd.AddCommand(demoInternalWaitPostgresCmd)
	demoInternalHelperCmd.AddCommand(demoInternalWaitTraceCmd)
	demoInternalHelperCmd.AddCommand(demoInternalWaitTraceContentCmd)
	demoInternalHelperCmd.AddCommand(demoInternalSeedToolCaptureCmd)
	demoInternalHelperCmd.AddCommand(demoInternalWriteSummaryCmd)

	demoInternalRandomHexCmd.Flags().Int("bytes", 16, "number of random bytes")

	demoInternalWaitPostgresCmd.Flags().String("url", "", "postgres connection URL")
	demoInternalWaitPostgresCmd.Flags().Duration("timeout", 30*time.Second, "maximum wait duration")

	demoInternalWaitTraceCmd.Flags().String("api-url", "", "CMDR API base URL")
	demoInternalWaitTraceCmd.Flags().String("trace-id", "", "trace ID")
	demoInternalWaitTraceCmd.Flags().Int("steps", 0, "expected number of steps")
	demoInternalWaitTraceCmd.Flags().Int("tools", 0, "expected number of tool captures")
	demoInternalWaitTraceCmd.Flags().String("error-substring", "", "required substring in one tool error")
	demoInternalWaitTraceCmd.Flags().Duration("timeout", 30*time.Second, "maximum wait duration")

	demoInternalWaitTraceContentCmd.Flags().String("api-url", "", "CMDR API base URL")
	demoInternalWaitTraceContentCmd.Flags().String("prompt", "", "required prompt text")
	demoInternalWaitTraceContentCmd.Flags().String("completion", "", "required completion text")
	demoInternalWaitTraceContentCmd.Flags().Int("limit", 50, "number of traces to scan per poll")
	demoInternalWaitTraceContentCmd.Flags().Duration("timeout", 30*time.Second, "maximum wait duration")

	demoInternalSeedToolCaptureCmd.Flags().String("postgres-url", "", "postgres connection URL")
	demoInternalSeedToolCaptureCmd.Flags().String("trace-id", "", "trace id")
	demoInternalSeedToolCaptureCmd.Flags().String("tool-name", "", "tool name")
	demoInternalSeedToolCaptureCmd.Flags().String("tool-args", "{}", "tool args JSON")
	demoInternalSeedToolCaptureCmd.Flags().String("tool-result", "{}", "tool result JSON")

	demoInternalWriteSummaryCmd.Flags().String("output", "", "output file path")
	demoInternalWriteSummaryCmd.Flags().String("baseline-trace-id", "", "baseline trace id")
	demoInternalWriteSummaryCmd.Flags().String("safe-replay-trace-id", "", "safe replay trace id")
	demoInternalWriteSummaryCmd.Flags().String("unsafe-replay-trace-id", "", "unsafe replay trace id")
	demoInternalWriteSummaryCmd.Flags().String("run-log", "", "run log path")
	demoInternalWriteSummaryCmd.Flags().String("cmdr-log", "", "cmdr log path")
	demoInternalWriteSummaryCmd.Flags().String("freeze-log", "", "freeze log path")
	demoInternalWriteSummaryCmd.Flags().String("migration-mcp-log", "", "migration mcp log path")
	demoInternalWriteSummaryCmd.Flags().String("mock-llm-log", "", "mock llm log path")
	demoInternalWriteSummaryCmd.Flags().String("capture-agentgateway-log", "", "capture agentgateway log path")
	demoInternalWriteSummaryCmd.Flags().String("replay-agentgateway-log", "", "replay agentgateway log path")
	demoInternalWriteSummaryCmd.Flags().String("safe-verdict-log", "", "safe verdict log path")
	demoInternalWriteSummaryCmd.Flags().String("unsafe-verdict-log", "", "unsafe verdict log path")

	demoInternalMigrationAgentCmd.Flags().String("mode", "", "capture or replay")
	demoInternalMigrationAgentCmd.Flags().String("model", "", "OpenAI-compatible model name")
	demoInternalMigrationAgentCmd.Flags().String("provider", "openai", "provider name for OTLP spans")
	demoInternalMigrationAgentCmd.Flags().String("behavior", "safe", "safe or unsafe")
	demoInternalMigrationAgentCmd.Flags().String("trace-id", "", "hex trace id")
	demoInternalMigrationAgentCmd.Flags().String("freeze-trace-id", "", "approved baseline trace id for replay")
	demoInternalMigrationAgentCmd.Flags().String("service-name", "migration-demo-agent", "service.name for OTLP spans")
	demoInternalMigrationAgentCmd.Flags().String("otlp-url", "", "CMDR OTLP base URL")
	demoInternalMigrationAgentCmd.Flags().String("otlp-mode", "combined", "combined or tools-only")
	demoInternalMigrationAgentCmd.Flags().String("llm-url", "", "base URL for OpenAI-compatible upstream")
	demoInternalMigrationAgentCmd.Flags().String("mcp-url", "", "streamable HTTP MCP endpoint")
	demoInternalMigrationAgentCmd.Flags().String("prompt", migrationdemo.DefaultPrompt, "initial user prompt")
	demoInternalMigrationAgentCmd.Flags().Int("max-turns", 8, "max agent turns")
	demoInternalMigrationAgentCmd.Flags().String("expect-final-substring", "", "require substring in final assistant response")
	demoInternalMigrationAgentCmd.Flags().Bool("expect-tool-error", false, "require at least one tool error")

	demoInternalFreezeCaptureCmd.Flags().String("otlp-url", "http://127.0.0.1:4318", "CMDR OTLP base URL")
	demoInternalFreezeCaptureCmd.Flags().String("trace-id", "", "hex trace id")
	demoInternalFreezeCaptureCmd.Flags().String("model", freezeloop.DefaultModel, "model name")
	demoInternalFreezeCaptureCmd.Flags().String("provider", freezeloop.DefaultProvider, "provider name")
	demoInternalFreezeCaptureCmd.Flags().String("prompt", freezeloop.DefaultPrompt, "user prompt")
	demoInternalFreezeCaptureCmd.Flags().String("completion", freezeloop.DefaultCompletionText, "assistant completion")
	demoInternalFreezeCaptureCmd.Flags().String("tool-name", freezeloop.DefaultToolName, "tool name")
	demoInternalFreezeCaptureCmd.Flags().String("tool-args", freezeloop.MustJSON(freezeloop.DefaultToolArgs()), "tool args JSON")
	demoInternalFreezeCaptureCmd.Flags().String("tool-result", freezeloop.MustJSON(freezeloop.DefaultToolResult()), "tool result JSON")
	demoInternalFreezeCaptureCmd.Flags().String("service-name", freezeloop.DefaultServiceName, "service.name")

	demoInternalFreezeLoopCmd.Flags().String("llm-url", "http://127.0.0.1:3002", "OpenAI-compatible LLM base URL")
	demoInternalFreezeLoopCmd.Flags().String("mcp-url", "http://127.0.0.1:9090/mcp/", "freeze-mcp streamable HTTP endpoint")
	demoInternalFreezeLoopCmd.Flags().String("freeze-trace-id", "", "approved baseline trace id")
	demoInternalFreezeLoopCmd.Flags().String("model", freezeloop.DefaultModel, "model name")
	demoInternalFreezeLoopCmd.Flags().String("prompt", freezeloop.DefaultPrompt, "initial prompt")
	demoInternalFreezeLoopCmd.Flags().Int("max-turns", 4, "max loop turns")
	demoInternalFreezeLoopCmd.Flags().String("expect-substring", freezeloop.DefaultExpectedSubstring, "expected substring in final answer")
}

func migrationAgentConfigFromCommand(cmd *cobra.Command) (migrationdemo.AgentConfig, error) {
	cfg := migrationdemo.AgentConfig{
		Mode:                 mustFlagString(cmd, "mode"),
		Model:                mustFlagString(cmd, "model"),
		Provider:             mustFlagString(cmd, "provider"),
		Behavior:             mustFlagString(cmd, "behavior"),
		TraceID:              mustFlagString(cmd, "trace-id"),
		FreezeTraceID:        mustFlagString(cmd, "freeze-trace-id"),
		ServiceName:          mustFlagString(cmd, "service-name"),
		OTLPURL:              mustFlagString(cmd, "otlp-url"),
		OTLPMode:             mustFlagString(cmd, "otlp-mode"),
		LLMURL:               mustFlagString(cmd, "llm-url"),
		MCPURL:               mustFlagString(cmd, "mcp-url"),
		Prompt:               mustFlagString(cmd, "prompt"),
		MaxTurns:             mustFlagInt(cmd, "max-turns"),
		ExpectFinalSubstring: mustFlagString(cmd, "expect-final-substring"),
		ExpectToolError:      mustFlagBool(cmd, "expect-tool-error"),
	}

	if cfg.Mode == "" || cfg.Model == "" || cfg.TraceID == "" || cfg.LLMURL == "" || cfg.MCPURL == "" {
		return cfg, errors.New("--mode, --model, --trace-id, --llm-url, and --mcp-url are required")
	}
	if cfg.Mode != "capture" && cfg.Mode != "replay" {
		return cfg, errors.New("--mode must be capture or replay")
	}
	if cfg.OTLPMode != "combined" && cfg.OTLPMode != "tools-only" {
		return cfg, errors.New("--otlp-mode must be combined or tools-only")
	}
	return cfg, nil
}

func mustFlagString(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	return value
}

func mustFlagInt(cmd *cobra.Command, name string) int {
	value, _ := cmd.Flags().GetInt(name)
	return value
}

func mustFlagBool(cmd *cobra.Command, name string) bool {
	value, _ := cmd.Flags().GetBool(name)
	return value
}
