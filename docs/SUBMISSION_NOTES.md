# MCP_HACK//26 Submission — Secure & Govern MCP

## One-Sentence Pitch

CMDR is an agent governance system that uses agentgateway telemetry to detect behavioral drift in production and gate deployments by replaying scenarios with frozen MCP tool responses.

## What We Built (Plain English)

We built the missing governance layer for AI agents. When you change an agent's model, prompt, or policy, CMDR tells you two things:

1. **Is it drifting?** — CMDR continuously compares live agent behavior against a known-good baseline. If tool call patterns, risk levels, or token usage shift, you get an alert.

2. **Is it safe to deploy?** — Before you swap models or change instructions, CMDR replays captured scenarios with deterministic tool responses (via freeze-mcp). If the new config makes riskier tool calls or diverges from expected behavior, the deploy is blocked.

This is what ops teams already do for microservices (canary deploys + anomaly detection). We brought it to AI agents.

## Proven With Real Models

We ran GPT-4o-mini on the same database migration task with two different system prompts:

| | Safe Instructions | Aggressive Instructions |
|---|---|---|
| **System prompt** | "Never use drop_table" | "Drop unnecessary tables first" |
| **First tool call** | `inspect_schema` | `drop_table` (BLOCKED) |
| **Tool sequence** | inspect → backup → migrate | drop × 5 → inspect → backup → drop |
| **CMDR verdict** | Baseline (approved) | **FAIL** (similarity: 0.4192) |
| **Risk** | No escalation | **ESCALATION** detected |
| **Tokens used** | Baseline | +2101 (2x more, retrying blocked ops) |

Same model. Same tools. Different instructions. CMDR caught it.

## Why This Fits "Secure & Govern MCP"

The category asks for security and governance of MCP-based agents using agentgateway. CMDR is exactly that:

- **agentgateway** is the telemetry source — CMDR ingests OTEL traces from agentgateway to capture every LLM call and tool call an agent makes.
- **freeze-mcp** is the governance mechanism — by freezing tool responses at the MCP boundary, we isolate model behavior from environmental noise.
- **Drift detection** is runtime governance — continuous monitoring that agent behavior stays within bounds.
- **Deployment gates** are pre-deployment governance — replay-based verification before changes go live.

## Why This Is Novel

Existing tools (LangSmith, Langfuse, Braintrust, Promptfoo) focus on output scoring and trace visualization. CMDR's innovations:

1. **Protocol-native deterministic replay** — Freeze tool responses at the MCP layer, not at the application layer. No SDK changes needed. Any agent that talks MCP through agentgateway gets governance for free.

2. **Behavioral fingerprinting for drift detection** — Not just "did the output change?" but "did the agent's tool-calling patterns, risk profile, and resource usage change?" This catches subtle regressions that output-scoring misses.

3. **Deployment gates for agents** — The concept of "replay a scenario before deploying" doesn't exist in the agent ecosystem. CMDR brings the microservices pattern of canary verification to LLM agents.

## Scoring Guide Mapping

### 1) Incorporation of Open Source Projects (40 pts)
- **agentgateway**: Core integration. CMDR ingests OTEL traces from agentgateway for all capture, drift detection, and replay functionality. Also used as the LLM proxy for real-model replay via `llm.models` config.
- **freeze-mcp**: Built as a companion MCP server. Reads from CMDR's shared PostgreSQL, serves frozen tool responses via standard MCP protocol.

### 2) Usefulness (20 pts)
CMDR answers two questions every agent team faces:
- "Has our agent's behavior changed since the last known-good state?" (drift detection)
- "If we change the instructions or swap models, will the agent still behave correctly?" (deployment gate)

These are practical, day-one problems. The CLI is CI/CD friendly (`exit 0` = pass, `exit 1` = fail).

### 3) Product Readiness (20 pts)
- Three-tier demo: deterministic (no keys) → full-stack (mock LLM) → real-model (OpenAI)
- Fully functional OTLP ingestion pipeline (gRPC + HTTP)
- PostgreSQL storage with migrations, 12 tables, full CRUD
- CLI with drift, gate, and demo commands
- Mission Control UI with four review surfaces
- Docker + docker-compose for one-command setup
- CI pipeline (test, lint, build)
- freeze-mcp runs as a standalone sidecar

### 4) Launch Bucket (20 pts)
- Demo video: three-tier walkthrough from `make demo` to real-model verdict
- Blog post: "Same Model, Different Instructions, CMDR Caught It" — grounded in real GPT-4o-mini results
- README with concrete output snippets for all three demo levels

## Demo Narrative (What Judges See)

1. **Level 1 (30 seconds)**: `make demo` seeds data and runs drift + gate checks. Score: 0.325 WARN drift, 0.5275 FAIL gate, 0.8707 PASS gate. CI exit codes work.

2. **Level 2 (2 minutes)**: `cmdr demo migration run` proves the full pipeline. agentgateway captures a real agent loop. freeze-mcp freezes the tool responses. A safe replay matches (PASS, 0.9000). An unsafe replay with `drop_table` fails (FAIL, 0.1000).

3. **Level 3 (the closer)**: Real GPT-4o-mini, real agentgateway, same frozen tools, different system prompts. The aggressive instructions cause the model to call `drop_table` five times. freeze-mcp blocks every attempt. CMDR verdict: FAIL, 0.4192, ESCALATION. Token delta: +2101.

## Submission Summary (Short Form)

CMDR brings microservice-style governance to AI agents. Using agentgateway's OTEL telemetry, it captures agent behavior, detects drift against known-good baselines, and gates model/prompt deployments by replaying scenarios with frozen MCP tool responses. Proven with real GPT-4o-mini: same model, different instructions, CMDR caught the behavioral divergence and blocked the deploy.
