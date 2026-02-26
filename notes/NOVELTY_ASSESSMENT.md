# CMDR Novelty Assessment & Hackathon Strategy

## Scoring Rubric (100 points total)

| Category | Points | Weight |
|----------|--------|--------|
| Incorporation of Open Source Projects (agentgateway, kagent, agentregistry) | 40 | Highest |
| Usefulness (solves real-world problems) | 20 | |
| Product Readiness (secure, documented, usable, GitHub repo) | 20 | |
| Launch (blog post, demo video, social media) | 20 | |

Judges include Kelsey Hightower (Google), Nathan Taber (NVIDIA), Carlos Santana (AWS), and experts from Solo.io (creators of kagent, agentgateway, agentregistry).

Five categories, $1,000 each: Starter Track, Secure & Govern MCP, Building Cool Agents (kagent), Explore Agent Registry, Open Source Contributions.

---

## Novelty Assessment

### What is genuinely novel

**Freeze-Tools deterministic replay at the MCP protocol layer.** No one has shipped this as a working tool. The idea of intercepting MCP tool calls, capturing responses during a baseline run, and returning frozen responses during variant replays to isolate pure model behavior differences is a real contribution. It enables fair apples-to-apples model comparison that existing tools cannot do.

**Protocol-native, zero-SDK-change approach.** Working at the gateway/MCP layer rather than requiring agent SDK instrumentation is architecturally distinct from LangSmith, Langfuse, and Braintrust, which all require their SDK to be embedded in agent code.

**First-divergence explanation.** Most eval tools tell you *that* outputs differ. Identifying the exact step where model behavior first diverges and explaining *why* is a stronger story.

### What is not novel

**LLM comparison and evaluation frameworks.** This is well-trodden ground:

- **Promptfoo** (18k+ GitHub stars): Multi-model comparison, LLM-as-judge, rubrics, red-teaming, declarative YAML configs, CI/CD integration. Open source, runs locally.
- **Braintrust**: Scorer-first evaluation, custom code + LLM judge scorers, dataset versioning, playground diff mode, immutable experiment snapshots.
- **LangSmith**: Side-by-side trace comparison, experiment-result comparison, online evaluators for multi-turn threads, OTEL ingestion.
- **Langfuse**: Open-source observability + eval with OTEL-native tracing, dataset+experiment workflows, LLM-as-a-judge, annotation queues, prompt lifecycle features.
- **OpenAI eval stack**: Integrated trace grading, configurable grader types, async eval runs at scale, dataset-driven prompt optimizer loops.
- **Inspect (UK AISI)**: Explicit scorer/metric abstractions, safety-focused sandboxing (Docker/K8s/VM), designed for agentic evaluations.

**The 5 evaluator types, rubric scoring, human-in-the-loop, ground truth management** are standard features across all major platforms. This is table stakes, not differentiation.

**Deterministic replay as a concept** has been described:

- Sakura Sky published a detailed blog series on deterministic replay for AI agents (same architectural primitives: structured execution traces, deterministic stubs, replay engine, agent harness).
- mcp-eval (MCPProxy) does trajectory-based evaluation with baseline comparison and side-by-side HTML reports.
- LangGraph has "time travel" debugging with checkpoint-based replay and fork-from-history.
- Kagent's own roadmap explicitly lists "time travel debugging" and "evaluation frameworks" as planned features.

**Bottom line:** Freeze-Tools at the MCP layer is the novel core. Everything else is competitive but not differentiated.

---

## Critical Scoring Problem: Open Source Integration (40/100 points)

Current draft plans only integrate one of three required projects, and that integration is shallow.

| Project | Current Plan | Depth | Risk |
|---------|-------------|-------|------|
| **agentgateway** | Used as HTTP client for LLM requests + OTEL trace source | Shallow - just passing requests through it | Medium |
| **kagent** | Not mentioned | None | **Critical** |
| **agentregistry** | Not mentioned | None | **Critical** |

40% of the score depends on incorporating these projects effectively. Ignoring 2 of 3 is a disqualifying gap.

---

## Current Scoring Position

| Category | Points Available | Estimated Current Score | Notes |
|----------|-----------------|------------------------|-------|
| Open Source Integration | 40 | ~10 | Only 1/3 projects, thin usage |
| Usefulness | 20 | ~15 | Real problem, real users |
| Product Readiness | 20 | ~5 | Nothing built yet; plan is over-scoped for hackathon timeline |
| Launch | 20 | ~0 | No blog, video, or social plan in drafts |
| **Total** | **100** | **~30** | |

---

## Recommendations

### 1. Integrate kagent deeply (biggest scoring opportunity)

CMDR fills the exact gap in kagent's published roadmap. Solo.io explicitly said they want "time travel debugging" and "evaluation frameworks" for kagent. Build that.

**Concrete integration points:**

- **ToolServer CRD**: Define CMDR's Freeze-Tools as a kagent `ToolServer` custom resource so any kagent agent can use replay/eval as MCP tools.
- **Agent definition**: Create a kagent agent YAML for a "Model Evaluator Agent" that orchestrates CMDR experiments (capture a run, replay with variants, produce a diff report).
- **OTEL trace source**: Use kagent's built-in OpenTelemetry tracing as the trace capture mechanism instead of raw OTLP. Traces from kagent-managed agents flow directly into CMDR.
- **Kubernetes-native deployment**: Deploy CMDR as a service in the same cluster as kagent, discoverable via Kubernetes service DNS.
- **Demo scenario**: A kagent agent troubleshooting a K8s issue, captured and replayed across GPT-4 vs Claude vs Gemini, showing which model makes safer operational decisions.

**Why this matters for judges:** You'd be presenting a working implementation of something the kagent team said they want to build. The judges from Solo.io will notice.

### 2. Integrate agentregistry

**Concrete integration points:**

- **Register Freeze-Tools MCP server** as an artifact in agentregistry so teams can discover and deploy it.
- **Discover MCP tools** via agentregistry's API when setting up capture — instead of hardcoding tool endpoints, query the registry for available tools.
- **Publish evaluation rubrics** as curated artifacts (e.g., "safety evaluation rubric for K8s agents") that other teams can reuse.
- **Use governance workflows** — evaluation configs go through agentregistry's approval flow before being used in production experiments.

### 3. Deepen agentgateway integration

**Concrete integration points:**

- **RBAC and policy system**: Use agentgateway's existing policies for the safety/policy analysis dimension instead of reimplementing policy tracking.
- **CEL expressions**: Use agentgateway's CEL engine for defining evaluation rules and policy checks.
- **Listener-level policies**: Explore using agentgateway's frontend policies to intercept and freeze tool calls at the proxy layer.
- **Multi-tenant support**: Scope experiments to tenants using agentgateway's tenant model.
- **OpenAPI transformation**: Use agentgateway's legacy API support to bring non-MCP tools into the replay framework.

### 4. Drastically reduce scope

The current 4-week plan describes a production system. For a hackathon, cut to a demo-able MVP.

**Keep (core differentiation):**
- Freeze-Tools MCP server (capture + freeze modes)
- Single experiment flow: capture baseline, replay 2-3 variants
- Behavior diff with first-divergence detection
- Basic CLI (`cmdr capture`, `cmdr replay`, `cmdr diff`)
- Integration with all three open source projects
- Simple report output (Markdown)

**Cut (not needed for hackathon):**
- Human-in-the-loop evaluator and review queue
- Ground truth management and storage
- JWT authentication and authorization
- Worker pool scaling and rate limiting
- Prometheus metrics and health checks
- Multiple trace backends (Jaeger, Tempo)
- 5 evaluator types (keep 2: rule-based + LLM-judge)
- REST API (CLI-only for MVP)
- 99.9% uptime targets
- Load testing for 100 concurrent experiments

**Add (hackathon requirements):**
- kagent ToolServer CRD + agent definition
- agentregistry artifact publishing
- Blog post
- Demo video (3-5 minutes)
- Social media posts

### 5. Target the "Building Cool Agents" category (kagent track)

Rationale:
- Directly aligned with kagent's stated roadmap needs
- Solo.io judges will see this as valuable to their ecosystem
- Less competition than the generic starter track
- Freeze-Tools + kagent agent is a compelling "cool agent" demo
- The evaluation/replay angle is unique in a category likely dominated by chat agents and workflow bots

### 6. Build the Launch bucket (20 points for free)

**Blog post** (publish on dev.to, Medium, or personal blog):
- Title: "Time Travel for Cloud-Native AI Agents: Deterministic Replay with kagent and agentgateway"
- Structure: Problem (can't fairly compare LLMs), solution (Freeze-Tools), demo walkthrough, architecture diagram
- Link to GitHub repo

**Demo video** (3-5 minutes):
1. Show a kagent agent performing a K8s troubleshooting task
2. CMDR captures the trace and tool calls
3. Replay the same task with a different model
4. Show the behavior diff and first-divergence point
5. Show the evaluation scorecard

**Social media**:
- Twitter/X thread with key screenshots
- LinkedIn post targeting DevOps/platform engineering audience
- Tag hackathon organizers and kagent/agentgateway accounts

### 7. Sharpen the positioning

**Current positioning (weak):** "A replay-backed behavior analysis lab for LLM agents" — sounds like another eval tool.

**Recommended positioning (strong):** "The missing test infrastructure for cloud-native AI agents — capture real kagent agent runs, freeze the environment, swap models, and get behavior diffs that explain *why* agents diverge."

This frames CMDR as:
- Cloud-native infrastructure (aligned with hackathon theme)
- Built for kagent (aligned with scoring)
- About *understanding* agent behavior (not just scoring outputs)
- Filling a specific gap (not competing with general-purpose eval tools)

---

## Revised Scoring Estimate (with recommendations applied)

| Category | Points Available | Estimated Score | Notes |
|----------|-----------------|-----------------|-------|
| Open Source Integration | 40 | ~30-35 | Deep integration with all 3 projects |
| Usefulness | 20 | ~15 | Same strong use case |
| Product Readiness | 20 | ~12-15 | Focused MVP, documented, deployable |
| Launch | 20 | ~15 | Blog + video + social |
| **Total** | **100** | **~72-80** | Competitive submission |

---

## Suggested Revised Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        KUBERNETES CLUSTER                        │
│                                                                  │
│  ┌──────────────┐     ┌──────────────┐     ┌────────────────┐  │
│  │   kagent      │     │ agentgateway │     │ agentregistry  │  │
│  │   Controller  │     │ (proxy +     │     │ (MCP artifact  │  │
│  │              │     │  OTEL +      │     │  registry)     │  │
│  │  Agent CRDs  │     │  policies)   │     │                │  │
│  └──────┬───────┘     └──────┬───────┘     └───────┬────────┘  │
│         │                     │                      │           │
│         │  ┌──────────────────┼──────────────────────┘           │
│         │  │                  │                                   │
│         ▼  ▼                  ▼                                   │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    CMDR Service                          │    │
│  │                                                          │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │    │
│  │  │ Freeze-Tools │  │ Experiment   │  │ Behavior     │  │    │
│  │  │ MCP Server   │  │ Engine       │  │ Diff Engine  │  │    │
│  │  │ (registered  │  │ (capture +   │  │ (first-div + │  │    │
│  │  │  in registry)│  │  replay)     │  │  4D analysis)│  │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘  │    │
│  │                                                          │    │
│  │  ┌──────────────┐  ┌──────────────┐                     │    │
│  │  │ Evaluators   │  │ Report       │                     │    │
│  │  │ (rule-based  │  │ Generator    │                     │    │
│  │  │  + LLM-judge)│  │ (Markdown)   │                     │    │
│  │  └──────────────┘  └──────────────┘                     │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  kagent ToolServer CRD: cmdr-tools                       │    │
│  │  Exposes: capture, replay, diff, evaluate as MCP tools   │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  kagent Agent CRD: model-evaluator                       │    │
│  │  Uses: cmdr-tools + k8s-tools to evaluate agents         │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

---

## Suggested Revised Implementation Plan

### Phase 1: Freeze-Tools core + agentgateway integration
- Freeze-Tools MCP server (capture mode + freeze mode)
- Argument normalization and matching
- OTEL trace ingestion from agentgateway
- Parse traces into replay schema
- SQLite storage (not PostgreSQL — simpler for hackathon)

### Phase 2: Replay engine + kagent integration
- Experiment engine (1 baseline + N variants)
- Replay via agentgateway with frozen tools
- kagent ToolServer CRD for CMDR tools
- kagent Agent CRD for model-evaluator agent
- Register Freeze-Tools in agentregistry

### Phase 3: Analysis + evaluation + reporting
- Behavior diff (tool sequence comparison, first divergence)
- Rule-based evaluator + LLM-judge evaluator
- Markdown report generation
- CLI: `cmdr capture`, `cmdr replay`, `cmdr diff`, `cmdr report`

### Phase 4: Demo + launch
- End-to-end demo scenario (kagent agent → capture → replay → diff)
- Blog post
- Demo video
- Social media
- Documentation (README, deployment guide)
- Docker image + K8s manifests

---

## Competitive Landscape Summary

| Tool | What it does | CMDR's edge |
|------|-------------|-------------|
| **Promptfoo** | Prompt/model comparison, red-teaming, YAML configs | Freeze-Tools (deterministic env), MCP-native, no SDK |
| **LangSmith** | Trace observability, side-by-side comparison, online eval | Freeze-Tools, protocol-level not SDK-level |
| **Langfuse** | Open-source observability + eval, OTEL tracing | Freeze-Tools, behavior-first diffing |
| **Braintrust** | Scorer-first eval, dataset versioning, playground diff | Freeze-Tools, agent behavior focus |
| **mcp-eval** | MCP trajectory comparison, baseline replay | Freeze-Tools (true determinism vs probabilistic), deeper analysis |
| **LangGraph** | Checkpoint-based time travel, state forking | MCP-native, multi-model comparison, evaluation framework |
| **Inspect** | Safety-focused eval, sandboxed execution | Freeze-Tools, cloud-native deployment, kagent integration |

The consistent differentiator across all comparisons is **Freeze-Tools + MCP-native + cloud-native (kagent/agentgateway) integration**. That's the pitch.

---

## Open Questions

1. **Which hackathon category to submit to?** Recommendation is "Building Cool Agents" (kagent track) but could also argue for "Secure & Govern MCP" if the safety/policy analysis angle is emphasized.

2. **SQLite vs PostgreSQL?** SQLite is simpler for the hackathon and demo. PostgreSQL is more realistic for production. Recommend SQLite for MVP.

3. **How deep should the kagent agent be?** A minimal agent that just calls CMDR tools, or a sophisticated agent that autonomously decides what to evaluate and how? Recommend minimal for hackathon scope.

4. **Demo scenario:** K8s troubleshooting agent is compelling but requires a realistic K8s problem setup. Alternative: a simpler code-review or documentation agent that's easier to stage.
