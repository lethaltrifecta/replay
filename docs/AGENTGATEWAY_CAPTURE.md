# Agentgateway Capture Contract

This document records the Phase 0 validation for getting a real capture path working through `agentgateway`.

## What We Validated

From the local `agentgateway` clone:

- `agentgateway` supports three backend families:
  - `ai` for LLM/provider proxying
  - `mcp` for MCP proxying
  - `host` for generic HTTP proxying
- AI spans natively include:
  - `gen_ai.request.model`
  - `gen_ai.provider.name`
  - `gen_ai.usage.input_tokens`
  - `gen_ai.usage.output_tokens`
- Prompt/completion bodies are not emitted by default.
- The tracing examples explicitly require `config.tracing.fields.add` to emit prompt/completion content.
- MCP activity is emitted on separate spans with fields like:
  - `mcp.method`
  - `mcp.resource.type`
  - `mcp.resource.name`

## What CMDR Now Accepts

CMDR's parser in `pkg/otelreceiver/parser.go` now accepts:

- `gen_ai.provider.name` as the primary provider field
- `gen_ai.system` as a fallback for older or vendor-specific traces
- `gen_ai.usage.input_tokens` / `gen_ai.usage.output_tokens`
- `gen_ai.usage.prompt_tokens` / `gen_ai.usage.completion_tokens` as fallbacks

CMDR still expects prompt/completion messages in the indexed OpenTelemetry style:

- `gen_ai.prompt.0.role`
- `gen_ai.prompt.0.content`
- `gen_ai.completion.0.content`

That means the easiest Milestone 1 path is to configure `agentgateway` tracing so those fields are added before sending OTLP to CMDR.

## Recommended Milestone 1 Setup

Use `agentgateway` as the AI proxy first, with prompt/completion tracing enabled. Add MCP proxy routing after the basic capture path is proven.

Minimal tracing config:

```yaml
config:
  tracing:
    otlpEndpoint: http://127.0.0.1:4317
    otlpProtocol: grpc
    randomSampling: true
    fields:
      add:
        gen_ai.system: 'llm.provider'
        gen_ai.prompt: 'flatten_recursive(llm.prompt)'
        gen_ai.completion: 'flatten_recursive(llm.completion.map(c, {"role":"assistant", "content": c}))'
```

The exact working local config is checked in at `scripts/agentgateway-cmdr-capture.yaml`.
Keep `randomSampling: true` for the demo path so requests without an incoming trace context still generate spans.

Why this shape works:

- `otlpProtocol: grpc` matches CMDR's OTLP gRPC receiver on port `4317`.
- `127.0.0.1` avoids localhost resolution edge cases in the local exporter path.
- `flatten_recursive(llm.prompt)` produces indexed prompt attributes that CMDR already parses.
- `flatten_recursive(...)` for completion produces `gen_ai.completion.0.content`.
- Adding `gen_ai.system` keeps traces compatible with older CMDR assumptions, while native `gen_ai.provider.name` still works.

## MCP Tool Capture

`agentgateway` can emit the semconv shape CMDR needs for tool capture ingestion, but it requires explicit CEL mapping for MCP `tools/call` requests.

The working migration-demo mapping is:

```yaml
config:
  tracing:
    fields:
      add:
        gen_ai.operation.name: 'default(json(request.body).method == "tools/call" ? "execute_tool" : "", "")'
        gen_ai.tool.call.arguments: 'default(json(request.body).method == "tools/call" ? toJson(json(request.body).params.arguments) : "", "")'
        gen_ai.tool.call.result: 'default(json(request.body).method == "tools/call" ? toJson(json(string(response.body).trim().stripPrefix("data: ")).result.structuredContent) : "", "")'
        error.message: 'default(json(request.body).method == "tools/call" ? default(json(string(response.body).trim().stripPrefix("data: ")).result.structuredContent.error.message, "") : "", "")'
```

That bridge takes MCP request/response bodies that `agentgateway` already sees and turns them into the semconv fields CMDR parses into `ToolCapture`.
The `tools/call` guard is important: touching `response.body` on the MCP stream GET path will buffer the stream and break replay sessions.

## Live Validation Result

The local no-secrets capture path is now proven:

1. `cmdr serve` receives OTLP on `4317`
2. `agentgateway` runs locally from the sibling clone
3. a mock OpenAI-compatible upstream answers `/v1/chat/completions`
4. a request through `agentgateway` lands in both `otel_traces` and `replay_traces`
5. the API surface exposes the captured trace at `GET /api/v1/traces/{traceId}`

The replay row shape from the live run was:

- provider: `openai`
- model: `mock-model`
- prompt: `Explain the outage briefly.`
- completion: `Mock response from local upstream.`

Repo assets for rerunning that flow:

- `scripts/agentgateway-cmdr-capture.yaml`
- `cmd/mock-openai-upstream`
- `scripts/test-agentgateway-capture.sh`

## Recommended Integration Sequence

1. Prove one real AI capture through `agentgateway` into `replay_traces`.
2. Record the raw OTEL span shape from that run in PostgreSQL.
3. Add the MCP CEL bridge for `tools/call`.
4. Verify CMDR now derives `ToolCapture` from gateway-emitted semconv spans.
5. Only then wire or expand the frozen replay path.

## Working Position

For the hackathon story:

- target architecture: use `agentgateway` for both AI and MCP
- migration demo implementation: now uses `agentgateway` for both LLM and tool spans
- remaining work: generalize the same MCP CEL bridge beyond the migration demo configs
