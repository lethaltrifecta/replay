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
    otlpEndpoint: http://localhost:4317
    fields:
      add:
        gen_ai.system: 'llm.provider'
        gen_ai.prompt: 'flattenRecursive(llm.prompt)'
        gen_ai.completion: 'flattenRecursive(llm.completion.map(c, {"role":"assistant", "content": c}))'
```

Why this shape works:

- `flattenRecursive(llm.prompt)` produces indexed prompt attributes that CMDR already parses.
- `flattenRecursive(...)` for completion produces `gen_ai.completion.0.content`.
- Adding `gen_ai.system` keeps traces compatible with older CMDR assumptions, while native `gen_ai.provider.name` still works.

## Important Limitation

`agentgateway` does not decompose LLM tool calls into CMDR's current `tool.name` / `tool.args` / `tool.result` span events on the LLM span.

For the real capture epic, this means:

- real LLM capture through `agentgateway`: ready now
- MCP tool capture via separate MCP spans: follow-up parser/storage work
- tool capture from the agent driver itself: still a valid fallback if MCP span parsing is not enough

## Recommended Integration Sequence

1. Prove one real AI capture through `agentgateway` into `replay_traces`.
2. Record the raw OTEL span shape from that run in PostgreSQL.
3. Decide whether CMDR should derive tool captures from:
   - MCP spans emitted by `agentgateway`, or
   - agent-emitted tool telemetry
4. Only then wire the full frozen replay path.

## Working Position

For the hackathon story:

- target architecture: use `agentgateway` for both AI and MCP
- Milestone 1 implementation: start with AI capture first
- Milestone 2 implementation: add MCP proxy + frozen tool path once baseline capture is stable
