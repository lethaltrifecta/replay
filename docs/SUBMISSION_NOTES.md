# MCP_HACK//26 Submission — Secure & Govern MCP

## One-Sentence Pitch

CMDR is an agent governance system that uses agentgateway telemetry to detect behavioral drift in production and gate deployments by replaying scenarios with frozen MCP tool responses.

## What We Built (Plain English)

We built the missing governance layer for AI agents. When you change an agent's model, prompt, or policy, CMDR tells you two things:

1. **Is it drifting?** — CMDR continuously compares live agent behavior against a known-good baseline. If tool call patterns, risk levels, or token usage shift, you get an alert.

2. **Is it safe to deploy?** — Before you swap models, CMDR replays captured scenarios with deterministic tool responses (via freeze-mcp). If the new model makes riskier tool calls or diverges from expected behavior, the deploy is blocked.

This is what ops teams already do for microservices (canary deploys + anomaly detection). We brought it to AI agents.

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
- **agentgateway**: Core integration. CMDR ingests OTEL traces from agentgateway for all capture, drift detection, and replay functionality. Also used as the HTTP client for prompt replay with model overrides.
- **freeze-mcp**: Built as a companion MCP server. Registered on port 9090, reads from CMDR's shared PostgreSQL, serves frozen tool responses via standard MCP protocol.

### 2) Usefulness (20 pts)
CMDR answers two questions every agent team faces:
- "Has our agent's behavior changed since the last known-good state?" (drift detection)
- "If we swap to a cheaper/newer model, will the agent still behave correctly?" (deployment gate)

These are practical, day-one problems. The CLI is CI/CD friendly (`exit 0` = pass, `exit 1` = fail).

### 3) Product Readiness (20 pts)
- Fully functional OTLP ingestion pipeline (gRPC + HTTP)
- PostgreSQL storage with migrations, 12 tables, full CRUD
- CLI with drift and gate commands
- Docker + docker-compose for one-command setup
- CI pipeline (test, lint, build)
- freeze-mcp runs as a standalone sidecar

### 4) Launch Bucket (20 pts)
- 2-3 minute demo video: capture → drift detect → gate check
- Blog post: "Governance for AI Agents: Bringing Canary Deploys to LLMs"
- Social thread with concrete example of a model swap caught by the gate

## Demo Narrative (What Judges See)

1. **Capture**: An agent runs through agentgateway. CMDR silently captures every LLM call and tool call.
2. **Baseline**: Mark this trace as "known-good" — `cmdr drift baseline set <id>`.
3. **Drift detection**: Agent runs again with a different model. `cmdr drift check` shows the behavioral fingerprint changed — tool call frequency shifted, risk class escalated from read to write.
4. **Deployment gate**: Before deploying the new model, run `cmdr gate check --baseline <id> --model gpt-4o-mini`. The gate replays the baseline scenario with frozen tools and reports: "FAIL — variant called destructive tool not in baseline."
5. **Deploy safely**: Fix the prompt, re-run the gate. It passes. Ship with confidence.

## Submission Summary (Short Form)

CMDR brings microservice-style governance to AI agents. Using agentgateway's OTEL telemetry, it captures agent behavior, detects drift against known-good baselines, and gates model/prompt deployments by replaying scenarios with frozen MCP tool responses. It's the missing "canary deploy" for the agent ecosystem.
