#+ Title
AI Hackathon Submission Narrative (Draft)

## One-Sentence Pitch
CMDR is a deterministic replay lab for LLM agents that freezes MCP tool responses so teams can compare model behavior fairly, find the first divergence, and make defensible deployment decisions.

## What We Built (Plain English)
We capture a real agent run, record every tool call and response, and replay the same run across different LLMs with tools “frozen” to identical outputs. This isolates model behavior from environmental noise and produces a clear diff of tool sequences, risk class changes, and output quality.

## Why This Is Novel
Existing eval/observability tools emphasize output scoring and trace comparison. CMDR’s core innovation is protocol-native deterministic replay at the MCP tool boundary. That lets us run apples-to-apples comparisons across models without SDK changes, identify the first divergence in tool behavior, and classify riskier tool usage (write/destructive) introduced by a model change.

## Scoring Guide Mapping

### 1) Incorporation of Open Source Projects (40)
We integrate all three required OSS projects in the core loop:
- agentgateway: used as the MCP gateway and entry point for tool traffic capture and replay routing.
- kagent: provides a real agent workflow with MCP tools and OTEL trace emission.
- agentregistry: publishes the Freeze-Tools MCP server so it can be discovered and installed as an MCP tool.

### 2) Usefulness (20)
CMDR answers: “Which model should we ship for this agent?” by holding tools constant and comparing behavior diffs, quality, safety, latency, and cost. This provides a defensible model selection process and avoids false conclusions caused by changing tool outputs.

### 3) Product Readiness (20)
MVP is fully runnable:
- Capture a baseline run (kagent + agentgateway).
- Freeze tool responses via the MCP Freeze-Tools server.
- Replay across multiple models.
- Generate a Markdown/JSON report with first divergence, tool graph diffs, and a scorecard.

### 4) Launch Bucket (20)
We will ship:
- A 2–3 minute demo video showing capture → replay → diff report.
- A brief blog post summarizing the architecture and results.
- A short social thread highlighting one surprising model divergence.

## Demo Narrative (What Judges See)
1. Run the same kagent workflow once with real tools and capture the trace.
2. Switch to replay mode: tools return frozen responses.
3. Replay on 2–3 models with identical tool outputs.
4. Show the first divergence (tool call sequence or args).
5. Show risk class delta (e.g., a model attempting a write or destructive tool).
6. Show the scorecard: quality vs cost vs latency.

## What Makes the Comparison Fair
We eliminate external variability (time, data changes, nondeterministic services) by freezing tool responses. Any observed difference is attributable to the model or configuration, not the environment.

## Submission Summary (Short Form)
CMDR is a replay-backed behavior analysis lab for LLM agents. It uses agentgateway + MCP to capture real tool calls and freeze tool responses, then replays identical scenarios across multiple models to surface first divergence and safety risk changes. This produces a fair, reproducible model comparison workflow that helps teams choose the best model for production.

