---
date: 2026-02-26T00:00:00-08:00
researcher: Claude Code
git_commit: 89aa1705e191bb03cbb367688fb6a3c9a2f83fa5
branch: main
repository: lethaltrifecta/replay
topic: "Comparison of DRAFT_PLAN.md vs IMPLEMENTATION_PLAN.md and Current Codebase State"
tags: [research, codebase, planning, architecture, cmdr, replay-service, freeze-tools]
status: complete
last_updated: 2026-02-26
last_updated_by: Claude Code
last_updated_note: "Added architectural decisions and Freeze-Tools explanation"
---

# Research: DRAFT_PLAN.md vs IMPLEMENTATION_PLAN.md Comparison and Codebase State

**Date**: 2026-02-26
**Researcher**: Claude Code
**Git Commit**: 89aa1705e191bb03cbb367688fb6a3c9a2f83fa5
**Branch**: main
**Repository**: lethaltrifecta/replay

## Research Question
What currently exists in the codebase versus what's planned? How do DRAFT_PLAN.md and IMPLEMENTATION_PLAN.md relate to each other?

## Summary
The `/Users/jaden.lee/code/lethal/replay` repository is currently in the **planning phase only** with **zero implementation**. The repository contains exactly two planning documents that describe different approaches to LLM replay and comparison:

1. **DRAFT_PLAN.md** - Describes "CMDR," an ambitious replay-backed behavior analysis lab
2. **IMPLEMENTATION_PLAN.md** - Describes "LLM Replay Service," a more focused cross-model comparison service

These documents represent **two distinct architectural visions** that are related but take different approaches to similar problems. Neither has been implemented yet.

## Current Codebase State

### What Exists
The repository contains only:
- `.git/` - Git repository metadata
- `DRAFT_PLAN.md` - CMDR product/architecture specification
- `IMPLEMENTATION_PLAN.md` - Go service implementation plan

### What Does NOT Exist
- **No source code** (no `.go` files, no `pkg/` directory, no `cmd/` directory)
- **No configuration files** (no `go.mod`, no `Makefile`, no YAML configs)
- **No tests** (no test files or test infrastructure)
- **No deployment artifacts** (no Dockerfiles, no Kubernetes manifests)
- **No documentation** beyond the two planning documents
- **No dependencies** installed or declared

**Conclusion**: The repository is a blank slate with only planning documents.

## Detailed Comparison: DRAFT_PLAN vs IMPLEMENTATION_PLAN

### Product Vision

#### DRAFT_PLAN.md (CMDR)
- **Product**: Replay-backed behavior analysis lab for LLM agents
- **Mission**: Help engineers answer behavior change questions across models and scenarios
- **Tagline**: "Replay is the control layer that keeps environment conditions fixed"
- **Primary Use Cases**:
  1. Compare model behavior on same captured run
  2. Run scenario experiments (prompt/policy/tool/retrieval variants)
  3. Produce explainable model diffs and evaluation scorecards

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
- **Product**: Standalone Go service for cross-model differential replay
- **Mission**: Enable data-driven model selection and cost optimization on production workloads
- **Primary Use Cases**:
  1. Fetch historical traces from OTLP collectors
  2. Replay prompts across multiple models
  3. Compare outputs, tokens, costs, and latency

### Architecture Philosophy

#### DRAFT_PLAN.md (CMDR)
**Approach**: Integrated gateway-centric architecture with deterministic control
- Hybrid OTEL + Replay store architecture
- Gateway captures runs with canonical `step_index`
- Feature-flagged capture sink in gateway
- Custom Freeze-Tools replay MCP server for determinism
- Per-run overrides for model/policy/tools
- **Key Innovation**: Deterministic environment pinning as first-class primitive

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**Approach**: Standalone microservice that operates on existing traces
- Separate Go service (not gateway-integrated)
- Fetches traces from existing Jaeger/Tempo OTLP collectors
- Replays through agentgateway as external client
- In-memory or PostgreSQL storage
- REST API for job management
- **Key Innovation**: Lightweight, minimal gateway changes (none required)

### Data Architecture

#### DRAFT_PLAN.md (CMDR)
**Schema**: Custom replay-specific tables with rich semantics
```
runs: run_id, trace_id, experiment_id, scenario_id, variant_id, tenant_id, app_id
steps: run_id, step_index (canonical order), step_type (llm/tool/policy/io)
llm_calls: provider, model, params, request_ref, response_ref, tokens, latency
tool_calls: tool_name, args_ref, result_ref, risk_class (read/write/destructive)
policy_events: policy_name, decision (allow/deny/transform), reason_ref
```
**Source**: Captured directly from gateway at runtime

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**Schema**: Job-centric storage with replay results
```
replay_jobs: id, name, trace_ids[], models[], status, progress, results_jsonb
replay_results: id, job_id, trace_id, model, response, metrics_jsonb, error
```
**Source**: Fetched from existing Jaeger/Tempo via HTTP API
**Data Extraction**: Parses OTLP span attributes (`gen_ai.prompt.0.role`, `gen_ai.usage.*`)

### Analysis Capabilities

#### DRAFT_PLAN.md (CMDR)
**Breadth**: Multi-dimensional behavior analysis

1. **Behavior Analysis**:
   - Tool-call sequence drift
   - Missing/extra tool calls
   - Argument drift (normalized JSON diff)
   - Tool graph behavior changes

2. **Safety/Policy Analysis**:
   - Risky tool behaviors (`write`, `destructive`)
   - Policy decision changes (`allow`, `deny`, `transform`)

3. **Result Quality**:
   - Semantic similarity to baseline
   - Optional rubric scoring
   - First-divergence explanations

4. **Efficiency**:
   - End-to-end latency delta
   - Model latency delta
   - Token/cost deltas

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**Breadth**: Output-centric comparison

1. **Comparison Metrics**:
   - Text similarity (Levenshtein, cosine similarity)
   - Token count deltas
   - Cost estimation (configurable pricing)
   - Latency comparison
   - Text/JSON diff generation

### Determinism Strategy

#### DRAFT_PLAN.md (CMDR)
**Approach**: Strict deterministic replay as core feature
- **Freeze-Tools**: Replay MCP server returns pre-captured tool results
- **Protocol-native replay**: Works at MCP/tool graph level without SDK changes
- **Risk Mitigation**: "Tool nondeterminism → strict Freeze-Tools for experiments"
- **Non-Negotiable**: "Deterministic Freeze-Tools replay mode" is MVP requirement

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**Approach**: Best-effort replay of prompts (no determinism guarantees)
- Replays prompts through live agentgateway
- No mention of freezing tool results
- Tools execute normally during replay
- Focus is on comparing model outputs, not ensuring identical conditions

### Integration Requirements

#### DRAFT_PLAN.md (CMDR)
**Gateway Changes Required**: Minimal but necessary
1. Feature-flagged replay capture sink
2. Canonical monotonic `step_index`
3. Stable correlation IDs: `run_id`, `trace_id`, `experiment_id`
4. Per-run override support (model, policy, tool endpoint)
**Philosophy**: "Zero agent SDK changes" but gateway must be instrumented

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**Gateway Changes Required**: None
- Operates as external client to existing agentgateway
- Uses existing OTLP trace data
- No gateway instrumentation needed
- Pure microservice approach

### User Interface

#### DRAFT_PLAN.md (CMDR)
**CLI Commands**: Experiment-focused workflow
```bash
cmdr experiment run --baseline <RUN> --variant <SPEC>...
cmdr diff --baseline <RUN> --candidate <RUN>
cmdr eval --baseline <RUN> --candidate <RUN> --scenario <YAML>
cmdr report --experiment <ID> --format json,md
```
**Configuration**: YAML scenario files with variant matrix, evaluation weights, thresholds

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**API**: REST endpoints for job management
```
POST /api/v1/replay              # Submit job
GET  /api/v1/replay/:id          # Get status
GET  /api/v1/replay/:id/results  # Get results
GET  /api/v1/replay              # List jobs
DELETE /api/v1/replay/:id        # Cancel job
```
**Configuration**: Environment variables with `REPLAY_` prefix

### Competitive Positioning

#### DRAFT_PLAN.md (CMDR)
**Differentiators vs Market** (LangSmith, Langfuse, Braintrust, etc.):
1. Deterministic environment pinning (`freeze-tools`) as first-class primitive
2. Protocol/gateway-native replay for MCP/tool graphs without SDK assumptions
3. Behavior-first diffing (tool graph + risk class + first divergence), not only output scoring
4. Scenario replay lab: same trace, swap model/prompt/policy/tool availability

**Positioning**: Not just observability or eval, but controlled experimental lab

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**No competitive analysis included**
- Focuses on implementation mechanics
- Does not position against alternatives
- More of an internal engineering plan than product strategy

### Implementation Timeline

#### DRAFT_PLAN.md (CMDR)
**Timeline**: 4-week phased rollout
- Week 1: Capture + Schema
- Week 2: Replay + Experiment Orchestration
- Week 3: Diff + Eval + Reports
- Week 4: Hardening
**Focus**: Product delivery with clear milestones

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**Timeline**: 28-day detailed implementation sequence
- Days 1-3: Foundation (config, logging, Makefile)
- Days 4-6: Trace Fetcher
- Days 7-9: Agentgateway Client
- Days 10-12: Storage Layer
- Days 13-16: Replay Engine
- Days 17-18: Comparator
- Days 19-21: REST API
- Days 22-23: CLI Entry Point
- Days 24-25: Docker & Deployment
- Days 26-28: Documentation & Polish
**Focus**: Engineering execution with granular task breakdown

### Technology Stack

#### DRAFT_PLAN.md (CMDR)
**Stack**: Not fully specified
- Mentions SQLite for MVP, pluggable later
- Replay MCP server (language unspecified)
- CLI tool (language unspecified)
- Focus on architecture, not implementation language

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**Stack**: Go-based microservice
- **Language**: Go 1.23
- **HTTP**: `gorilla/mux` router
- **Config**: `kelseyhightower/envconfig`
- **Logging**: `go.uber.org/zap`
- **Retry**: `github.com/avast/retry-go/v4`
- **DB**: PostgreSQL with `lib/pq`
- **CLI**: `spf13/cobra`
- **OTLP**: OpenTelemetry Go libraries
- **Testing**: `stretchr/testify`, `golang/mock`

### Code References from Agentgateway

#### DRAFT_PLAN.md (CMDR)
**References**: High-level mentions only
- References agentgateway as gateway to integrate with
- No specific file paths or code patterns referenced

#### IMPLEMENTATION_PLAN.md (LLM Replay Service)
**References**: Detailed code pattern references

Key files to reference for implementation:
1. **Worker Pool Pattern**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/kgateway/agentgatewaysyncer/status/workerpool.go` (lines 10-206)
2. **Options Pattern**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/utils/requestutils/curl/option.go`
3. **Configuration Loading**: `/Users/jaden.lee/code/lethal/agentgateway/controller/api/settings/settings.go` (lines 114-202)
4. **Error Handling**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/deployer/errors.go` (lines 9-18)
5. **Testing Suite**: `/Users/jaden.lee/code/lethal/agentgateway/controller/test/e2e/tests/base/base_suite.go`
6. **OTEL Tracing**: `crates/agentgateway/src/telemetry/trc.rs`

## Relationship Analysis

### Are They Related?
**Yes** - Both documents address the same core problem space: replaying LLM requests across different models and comparing the results.

### How Do They Differ?
They represent **different architectural philosophies**:

**DRAFT_PLAN (CMDR)**: Product-first, integrated, deterministic
- Starts with product vision and user needs
- Integrates deeply with gateway for capture and control
- Prioritizes determinism and behavior analysis
- More ambitious scope (policy, safety, scenarios)
- Positions against competitive landscape

**IMPLEMENTATION_PLAN**: Engineering-first, standalone, pragmatic
- Starts with technical implementation details
- Minimal gateway integration (uses as client only)
- Fetches existing traces (doesn't capture new ones)
- Narrower scope (output comparison only)
- Detailed 28-day execution plan

### Which Came First?
**Likely**: IMPLEMENTATION_PLAN → DRAFT_PLAN (evolved)

Evidence:
1. IMPLEMENTATION_PLAN is more granular and immediately actionable
2. DRAFT_PLAN includes "Adjacent Options in Market" analysis (later-stage thinking)
3. DRAFT_PLAN references more advanced concepts (Freeze-Tools, policy analysis)
4. DRAFT_PLAN's 4-week phases abstract over implementation details
5. IMPLEMENTATION_PLAN's detailed file structure suggests earlier design phase

**Theory**: The team likely started with IMPLEMENTATION_PLAN as a straightforward replay service, then evolved the vision into CMDR after considering competitive positioning and user needs.

### Can They Coexist?
**Possibly, but unlikely**. Key conflicts:

1. **Data Source Conflict**:
   - CMDR: Captures from gateway with custom schema
   - LLM Replay Service: Fetches from Jaeger/Tempo
   - **Resolution needed**: Choose one source of truth

2. **Architecture Conflict**:
   - CMDR: Gateway-integrated with capture sink
   - LLM Replay Service: Standalone microservice
   - **Resolution needed**: Decide on integration level

3. **Determinism Conflict**:
   - CMDR: Freeze-Tools deterministic replay is core
   - LLM Replay Service: Best-effort replay through live gateway
   - **Resolution needed**: Critical product differentiator

**Recommendation**: These should be unified into a single plan that decides:
- Where does data come from? (Gateway capture vs OTLP fetch)
- What level of determinism is required? (Freeze-Tools vs live replay)
- What's the integration strategy? (Gateway changes vs standalone)
- What's the scope? (Full behavior analysis vs output comparison)

## Key Decision Points

If moving forward with implementation, the team must decide:

### 1. Data Capture Strategy
- [ ] **Option A (CMDR)**: Implement gateway capture sink with custom schema
- [ ] **Option B (Replay Service)**: Use existing OTLP traces from Jaeger
- [ ] **Option C (Hybrid)**: Support both sources

### 2. Determinism Level
- [ ] **Option A (CMDR)**: Full Freeze-Tools deterministic replay
- [ ] **Option B (Replay Service)**: Best-effort replay (non-deterministic)
- [ ] **Option C (Progressive)**: Start with best-effort, add Freeze-Tools later

### 3. Architecture
- [ ] **Option A (CMDR)**: Gateway-integrated with feature flags
- [ ] **Option B (Replay Service)**: Standalone microservice
- [ ] **Option C (Hybrid)**: Standalone service with gateway SDK for capture

### 4. Analysis Scope
- [ ] **Option A (CMDR)**: Full multi-dimensional analysis (behavior, policy, quality, efficiency)
- [ ] **Option B (Replay Service)**: Output comparison only
- [ ] **Option C (Phased)**: Start narrow, expand over time

### 5. Implementation Language
- [ ] **Go** (as specified in IMPLEMENTATION_PLAN)
- [ ] **Rust** (matching agentgateway codebase)
- [ ] **Python** (for ML/similarity algorithms)
- [ ] **Multi-language** (Go service + Python analysis)

## Recommended Next Steps

1. **Align on Vision**: Team discussion to decide between CMDR vision vs simpler replay service
2. **Unify Plans**: Merge both documents into single authoritative spec
3. **Prototype**: Build minimal proof-of-concept to validate key technical decisions
4. **Decide Data Source**: Gateway capture vs OTLP fetch (blocking decision)
5. **Decide Determinism**: Is Freeze-Tools critical or nice-to-have?
6. **Choose Stack**: Go vs Rust vs Python for implementation
7. **Define MVP**: What's the absolute minimum to validate value?

## Code References
- [DRAFT_PLAN.md](DRAFT_PLAN.md) - CMDR product specification
- [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) - LLM Replay Service implementation guide

## Architecture Documentation

### What Exists Today
- No implemented architecture
- Two competing architectural visions documented

### DRAFT_PLAN Architecture (Not Implemented)
```
┌─────────────────┐
│   Agent SDK     │ (zero changes)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Gateway       │ ◄── Feature-flagged capture sink
│   (modified)    │ ◄── Canonical step_index
└────────┬────────┘ ◄── Per-run overrides
         │
         ├─────────► OTEL (trace discovery)
         │
         ├─────────► Replay Store (SQLite)
         │           ├── runs
         │           ├── steps
         │           ├── llm_calls
         │           ├── tool_calls
         │           └── policy_events
         │
         ▼
┌─────────────────┐
│ Freeze-Tools    │ (deterministic tool responses)
│ Replay MCP      │
└─────────────────┘

CLI: cmdr experiment run/diff/eval/report
```

### IMPLEMENTATION_PLAN Architecture (Not Implemented)
```
┌─────────────────┐
│ Jaeger/Tempo    │ (existing OTLP)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Trace Fetcher   │ (HTTP API client)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Replay Engine   │
│ - Worker Pool   │
│ - Job Mgmt      │
└────────┬────────┘
         │
         ├─────────► Storage (Memory/Postgres)
         │
         ├─────────► Agentgateway (as client)
         │
         ▼
┌─────────────────┐
│  Comparator     │ (similarity, diffs, metrics)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   REST API      │ (job CRUD, results)
└─────────────────┘
```

## Historical Context

This repository was created to enable cross-model comparison of LLM requests. Two planning documents were created:

1. **IMPLEMENTATION_PLAN.md** - A detailed engineering plan for a straightforward replay service
2. **DRAFT_PLAN.md** - An evolved product vision ("CMDR") with more ambitious goals

Neither has been implemented. The repository remains in the planning phase, waiting for architectural decisions and implementation to begin.

## Related Research
None yet - this is the first research document in this repository.

## Open Questions

1. **Which plan should be implemented?** CMDR, LLM Replay Service, or a hybrid?
2. **Who is the primary user?** Engineers evaluating models, or product teams running experiments?
3. **What's the relationship to agentgateway?** Should this be part of agentgateway or separate?
4. **What's the deployment model?** Self-hosted, cloud service, or both?
5. **What's the MVP scope?** How narrow should the initial release be?
6. **Is deterministic replay critical?** Or can we start with best-effort comparison?
7. **What data exists already?** Do we have OTLP traces to work with, or need to start capturing?
8. **What's the business case?** Cost savings, quality improvement, compliance, or all three?

---

## Follow-up Research: Architectural Decisions (2026-02-26)

### Architecture Decisions Made

The following architectural decisions were made to unify the two plans into a coherent implementation strategy:

**Update**: Added comprehensive evaluation framework to enable scoring and ranking of model outputs beyond basic analysis.

#### 1. Data Capture Strategy: **OTEL Exporter from Gateway**
**Decision**: Gateway emits OTEL traces → Replay service captures via OTEL exporter

**Rationale**:
- Leverages existing OTEL infrastructure (from IMPLEMENTATION_PLAN)
- No custom gateway capture sink needed (simplifies gateway changes)
- Replay service consumes standard OTEL traces (interoperability)
- Can use existing OTEL collectors (Jaeger/Tempo) or custom exporters

**Architecture**:
```
┌─────────────────┐
│  Agentgateway   │ (existing, already emits OTEL traces)
└────────┬────────┘
         │ OTEL spans
         │ (gen_ai.* attributes)
         ▼
┌─────────────────┐
│ OTEL Collector  │ (optional - Jaeger/Tempo/custom)
│ or Direct Export│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Replay Service  │ (consumes OTEL traces)
│ - OTLP Receiver │
│ - Trace Parser  │
└─────────────────┘
```

**Implementation Notes**:
- Replay service implements OTLP receiver endpoint (gRPC or HTTP)
- Parses OTEL span attributes: `gen_ai.prompt.*.role`, `gen_ai.prompt.*.content`, `gen_ai.usage.*`, `gen_ai.request.*`
- Stores parsed traces in replay-specific schema for analysis
- Can also query historical traces from Jaeger/Tempo via HTTP API for batch replay

#### 2. Determinism Strategy: **Freeze-Tools (Deterministic Replay)**
**Decision**: Include Freeze-Tools as core feature

**What is Freeze-Tools?**

Freeze-Tools is a deterministic replay mechanism that ensures identical tool execution results across multiple replays. When comparing different models on the same scenario, tools must return the exact same results to isolate model behavior differences.

**The Problem Without Freeze-Tools**:
```
Baseline Run (GPT-4):
1. LLM calls search_docs("authentication")
2. Tool executes → returns 10 docs (live search)
3. LLM processes results → generates answer A

Replay Run (Claude):
1. LLM calls search_docs("authentication")
2. Tool executes → returns 12 docs (docs were updated!)
3. LLM processes results → generates answer B

Problem: Is difference due to model behavior or different tool results?
```

**The Solution With Freeze-Tools**:
```
Baseline Run (GPT-4) - CAPTURE MODE:
1. LLM calls search_docs("authentication")
2. Tool executes → returns 10 docs
3. CAPTURED: tool_name="search_docs", args={query:"authentication"}, result=[10 docs]
4. LLM processes results → generates answer A

Replay Run (Claude) - FREEZE MODE:
1. LLM calls search_docs("authentication")
2. Freeze-Tools intercepts → returns CAPTURED result [10 docs]
3. LLM processes results → generates answer B

Now: Difference is PURELY due to model behavior (identical inputs)
```

**Freeze-Tools Architecture**:

Freeze-Tools operates as a special MCP (Model Context Protocol) server that:

1. **Capture Phase** (Baseline Run):
   - Acts as transparent proxy to real tools
   - Records all tool calls: name, arguments (JSON), results, errors
   - Stores captured data in replay store with run_id + step_index

2. **Replay Phase** (Variant Runs):
   - Replaces real MCP tools
   - When LLM requests tool call, looks up captured result by (tool_name, args)
   - Returns pre-captured result instead of executing tool
   - Maintains determinism: identical inputs → identical tool outputs

**Freeze-Tools MCP Server Implementation**:
```go
// Freeze-Tools acts as MCP server that returns frozen responses
type FreezeToolsServer struct {
    mode         ReplayMode     // CAPTURE or FREEZE
    captureStore *CaptureStore  // Recorded tool results
    fallbackMCP  MCPClient      // Real tools (for CAPTURE mode)
}

func (f *FreezeToolsServer) ExecuteTool(ctx context.Context, req ToolRequest) (ToolResponse, error) {
    if f.mode == CAPTURE {
        // CAPTURE MODE: Execute real tool and record result
        resp, err := f.fallbackMCP.ExecuteTool(ctx, req)
        f.captureStore.Record(req.ToolName, req.Args, resp, err)
        return resp, err
    }

    // FREEZE MODE: Return pre-captured result
    captured, found := f.captureStore.Lookup(req.ToolName, req.Args)
    if !found {
        return nil, ErrToolCallNotCaptured
    }
    return captured.Response, captured.Error
}
```

**Key Properties**:
- **Deterministic**: Same tool calls always return same results
- **Transparent**: LLM doesn't know tools are frozen (same MCP interface)
- **Argument Matching**: Uses normalized JSON comparison for args (handles whitespace, ordering)
- **Error Replay**: Captures and replays tool errors (not just successes)
- **Tool Graph Preservation**: Maintains exact tool call sequences

**Risk Classes** (from DRAFT_PLAN):
Freeze-Tools also tracks risk classifications:
- `read`: Safe operations (search, read files)
- `write`: Modifying operations (create files, update records)
- `destructive`: Dangerous operations (delete, terminate)

This enables safety analysis: "Did the new model attempt risky operations?"

**Benefits for Analysis**:
1. **Fair Comparison**: Models see identical tool outputs
2. **Behavioral Attribution**: Differences must be due to model reasoning, not environment changes
3. **Reproducibility**: Can replay experiments weeks later with same results
4. **Cost Efficiency**: Don't re-execute expensive tools (API calls, databases)
5. **Safety**: Don't re-execute destructive operations

**Implementation Requirements**:
- MCP server implementation (Go package: `pkg/freezetools/`)
- Capture store with argument normalization (JSON canonicalization)
- Argument matching algorithm (fuzzy match for equivalent JSON)
- Gateway configuration to route tools to Freeze-Tools MCP during replay

#### 3. Architecture: **Standalone Service Leveraging Agentgateway**
**Decision**: Standalone Go microservice that uses agentgateway as client (not integrated into gateway)

**Rationale**:
- Simpler to develop, test, and deploy independently
- No gateway code changes required
- Uses agentgateway via HTTP client (standard API)
- Can evolve independently without gateway release coupling

**Service Responsibilities**:
1. Consume OTEL traces (via OTLP receiver or Jaeger/Tempo query)
2. Store traces in replay-specific schema
3. Orchestrate replay experiments across models
4. Configure agentgateway requests with Freeze-Tools MCP endpoint
5. Analyze results (behavior, policy, quality, efficiency)
6. Expose REST API for job management

**Agentgateway Integration**:
- Replay service acts as HTTP client to agentgateway
- Sends LLM requests with special headers/params to indicate replay mode
- Configures tool endpoint to point to Freeze-Tools MCP server
- Agentgateway remains unchanged (just processes requests normally)

#### 4. Analysis Scope: **Full Behavior Analysis + Evaluation**
**Decision**: Implement full multi-dimensional analysis (from DRAFT_PLAN) + robust evaluation framework

**Analysis Dimensions**:

1. **Behavior Analysis**:
   - Tool call sequence comparison (added, removed, reordered)
   - Argument drift detection (normalized JSON diff)
   - Tool graph visualization (call tree comparison)
   - First divergence point identification

2. **Safety/Policy Analysis**:
   - Risk class changes (did model attempt write/destructive operations?)
   - Policy decision deltas (if policy layer exists)
   - Unauthorized tool access attempts

3. **Result Quality Analysis**:
   - Semantic similarity scoring (embeddings-based)
   - Optional rubric-based evaluation (configurable criteria)
   - First-divergence explanation (why did models diverge?)

4. **Efficiency Analysis**:
   - Token count deltas (vs baseline)
   - Cost deltas (configurable pricing per model)
   - Latency comparison (model latency, e2e latency)
   - Tool execution efficiency

5. **Evaluation Framework** (NEW):
   - Automated scoring of model outputs against ground truth or criteria
   - Multiple evaluation strategies (rule-based, LLM-as-judge, human-in-loop)
   - Configurable rubrics and scoring dimensions
   - Pass/fail thresholds for experiment acceptance
   - Comparative ranking across variants

**Implementation**:
- Each dimension implemented as separate analyzer package
- Evaluation engine as separate component with pluggable evaluators
- Comparator orchestrates all analyzers + evaluation
- Results aggregated into unified diff report + scorecard

#### 5. Implementation Language: **Go**
**Decision**: Go for all components

**Rationale**:
- Matches IMPLEMENTATION_PLAN stack
- Strong concurrency primitives (worker pools)
- Good OTLP library support
- Existing agentgateway patterns to reference (controller code)
- Excellent HTTP performance for REST API

**Stack** (from IMPLEMENTATION_PLAN):
- `gorilla/mux` - HTTP routing
- `kelseyhightower/envconfig` - Configuration
- `go.uber.org/zap` - Structured logging
- `avast/retry-go/v4` - Retry with backoff
- `lib/pq` - PostgreSQL driver
- `spf13/cobra` - CLI framework
- `go.opentelemetry.io/otel` - OTLP handling
- `stretchr/testify` - Testing
- `golang/mock` - Mocking

### Unified Architecture

**Final Architecture** (synthesizing both plans):

```
┌─────────────────────────────────────────────────────────────┐
│                    CAPTURE PHASE (Baseline)                  │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐    OTEL spans    ┌──────────────┐        │
│  │ Agentgateway │─────────────────►│ OTEL Exporter│        │
│  │ (with real   │                   │ (Jaeger/     │        │
│  │  tools)      │                   │  Tempo/      │        │
│  └──────────────┘                   │  Direct)     │        │
│         │                            └──────┬───────┘        │
│         │                                   │                │
│         ▼                                   ▼                │
│  ┌──────────────┐                   ┌──────────────┐        │
│  │  Real Tools  │                   │ Replay       │        │
│  │  (MCP)       │                   │ Service      │        │
│  └──────────────┘                   │ - Parse OTEL │        │
│                                      │ - Store Trace│        │
│                                      │ - Capture    │        │
│                                      │   Tool Data  │        │
│                                      └──────────────┘        │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                  REPLAY PHASE (Variants)                     │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────────────────────────────────┐              │
│  │ Replay Service (Orchestrator)            │              │
│  │ ┌────────────────────────────────────┐   │              │
│  │ │ Experiment Engine                  │   │              │
│  │ │ - Load baseline trace              │   │              │
│  │ │ - Generate variant matrix          │   │              │
│  │ │ - Configure Freeze-Tools endpoint  │   │              │
│  │ └────────────────────────────────────┘   │              │
│  │           │                               │              │
│  │           ▼                               │              │
│  │ ┌────────────────────────────────────┐   │              │
│  │ │ Worker Pool (concurrent replays)   │   │              │
│  │ │ - Per-model workers                │   │              │
│  │ │ - Rate limiting                    │   │              │
│  │ └────────┬───────────────────────────┘   │              │
│  └──────────┼───────────────────────────────┘              │
│             │                                               │
│             ▼                                               │
│  ┌──────────────────┐          ┌──────────────┐           │
│  │  Agentgateway    │◄─────────│ Freeze-Tools │           │
│  │  (HTTP Client)   │ MCP      │ MCP Server   │           │
│  │  - Model variant │ endpoint │ (frozen tool │           │
│  │  - Prompt variant│          │  results)    │           │
│  │  - Policy variant│          └──────┬───────┘           │
│  └──────────┬───────┘                 │                    │
│             │                          │                    │
│             ▼                          ▼                    │
│  ┌──────────────────────────────────────────────────┐     │
│  │           Replay Storage (Postgres)              │     │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │     │
│  │  │  Traces  │  │ Captures │  │ Replay Jobs  │   │     │
│  │  │  (OTEL)  │  │  (Tools) │  │ & Results    │   │     │
│  │  └──────────┘  └──────────┘  └──────────────┘   │     │
│  └──────────────────────────────────────────────────┘     │
│                          │                                 │
│                          ▼                                 │
│  ┌──────────────────────────────────────────────────┐     │
│  │           Analysis Engine                        │     │
│  │  ┌──────────────┐  ┌──────────────┐            │     │
│  │  │  Behavior    │  │  Safety/     │            │     │
│  │  │  Analyzer    │  │  Policy      │            │     │
│  │  │  - Tool seq  │  │  Analyzer    │            │     │
│  │  │  - Arg drift │  │  - Risk class│            │     │
│  │  └──────────────┘  └──────────────┘            │     │
│  │  ┌──────────────┐  ┌──────────────┐            │     │
│  │  │  Quality     │  │  Efficiency  │            │     │
│  │  │  Analyzer    │  │  Analyzer    │            │     │
│  │  │  - Similarity│  │  - Tokens    │            │     │
│  │  │  - Rubrics   │  │  - Cost      │            │     │
│  │  └──────────────┘  └──────────────┘            │     │
│  └──────────────────────────────────────────────────┘     │
│                          │                                 │
│                          ▼                                 │
│  ┌──────────────────────────────────────────────────┐     │
│  │           Report Generator                       │     │
│  │  - JSON format (machine-readable)                │     │
│  │  - Markdown format (human-readable)              │     │
│  │  - First divergence explanations                 │     │
│  │  - Scorecard (quality/safety/cost tradeoffs)     │     │
│  └──────────────────────────────────────────────────┘     │
│                          │                                 │
│                          ▼                                 │
│  ┌──────────────────────────────────────────────────┐     │
│  │           REST API                               │     │
│  │  POST   /api/v1/experiments                      │     │
│  │  GET    /api/v1/experiments/:id                  │     │
│  │  GET    /api/v1/experiments/:id/results          │     │
│  │  GET    /api/v1/experiments/:id/report           │     │
│  │  DELETE /api/v1/experiments/:id                  │     │
│  └──────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

### Data Flow

**1. Capture Phase** (Baseline run):
```
Agent Request → Agentgateway → Real Tools (MCP)
                     ↓
                OTEL Traces (spans with gen_ai.* attributes)
                     ↓
                OTEL Exporter → Replay Service
                     ↓
           Parse & Store in Replay Schema:
           - traces (prompts, completions, params)
           - tool_calls (name, args, results, risk_class)
           - metrics (tokens, latency, cost)
```

**2. Replay Phase** (Variant runs):
```
User: POST /api/v1/experiments
      {
        "baseline_trace_id": "abc123",
        "variants": [
          {"model": "claude-3-5-sonnet-20241022"},
          {"model": "gpt-4"},
          {"prompt": "alternative_prompt"}
        ]
      }

Replay Service:
1. Load baseline trace (prompts + captured tool results)
2. Start Freeze-Tools MCP server with captured tool data
3. For each variant:
   - Build LLM request with variant params
   - Configure agentgateway to use Freeze-Tools MCP endpoint
   - Send request to agentgateway
   - Collect response + metrics
4. Run analyzers (behavior, safety, quality, efficiency)
5. Generate diff report with first divergence
6. Return job_id

User: GET /api/v1/experiments/:id/report
      → Returns JSON/Markdown with comparisons
```

**3. Evaluation Phase** (NEW):
```
User: POST /api/v1/experiments/:id/evaluate
      {
        "evaluators": ["rule_check", "llm_judge", "rubric"],
        "config": {
          "pass_threshold": 0.75,
          "fail_fast": true
        }
      }

Replay Service:
1. Load experiment runs (baseline + variants)
2. For each run:
   a. Execute rule-based evaluators (synchronous)
      - Format validation
      - Required content checks
      - Pattern matching

   b. Execute LLM-judge evaluators (async)
      - Send output to judge model
      - Parse scores and reasoning
      - Store evaluator results

   c. Execute rubric evaluators (synchronous)
      - Score against defined rubric dimensions
      - Calculate weighted scores
      - Check pass thresholds

   d. Check ground-truth (if available)
      - Compare against expected output
      - Calculate similarity score

   e. Queue for human review (conditional)
      - If score < threshold
      - Or random sampling
      - Or fail-fast triggered

3. Aggregate evaluator results:
   - Calculate weighted overall score
   - Determine pass/fail per run
   - Rank runs by overall score
   - Identify winner

4. Generate evaluation summary:
   - Best performing model
   - Score breakdown per evaluator
   - Trade-off analysis (score vs cost)
   - Recommendations

5. Return evaluation_id

User: GET /api/v1/experiments/:id/evaluation/summary
      → Returns scorecard with winner and rankings
```

**4. Human Review Phase** (if applicable):
```
User: GET /api/v1/evaluations/human/pending
      → Returns list of runs queued for review

Reviewer: GET /api/v1/evaluations/human/:review_id
          → Shows output, context, automated scores

Reviewer: POST /api/v1/evaluations/human/:review_id/review
          {
            "scores": {
              "clarity": 8,
              "accuracy": 9,
              "helpfulness": 7
            },
            "feedback": "Good output but could be more concise",
            "approved": true
          }

Replay Service:
1. Update human evaluation record
2. Re-calculate overall score with human input
3. Update rankings if needed
4. Mark evaluation as complete
```

**Complete Flow**:
```
Capture → Replay → Analysis → Evaluation → (Human Review) → Report
   ↓         ↓         ↓           ↓              ↓            ↓
 OTEL    Freeze-   4D Diff   Rule/LLM/     Queue/Review  Scorecard
 Traces   Tools    Report    Rubric/GT                  with Winner
```

### Key Implementation Components

**Package Structure** (from IMPLEMENTATION_PLAN + DRAFT_PLAN synthesis):
```
pkg/
├── config/          # Configuration (envconfig)
├── otelreceiver/    # OTLP receiver (NEW - not in either plan)
├── tracefetcher/    # Fetch from Jaeger/Tempo (IMPLEMENTATION_PLAN)
├── traceparser/     # Parse OTEL spans → replay schema (IMPLEMENTATION_PLAN)
├── storage/         # Postgres storage (IMPLEMENTATION_PLAN)
├── freezetools/     # Freeze-Tools MCP server (DRAFT_PLAN - NEW)
├── agwclient/       # Agentgateway HTTP client (IMPLEMENTATION_PLAN)
├── replayengine/    # Experiment orchestration (IMPLEMENTATION_PLAN)
│   ├── engine.go
│   ├── workerpool.go
│   └── job.go
├── analyzer/        # Multi-dimensional analysis (DRAFT_PLAN - NEW)
│   ├── behavior.go  # Tool sequence, arg drift
│   ├── safety.go    # Risk class, policy changes
│   ├── quality.go   # Similarity, rubrics
│   └── efficiency.go # Tokens, cost, latency
├── evaluator/       # Evaluation framework (NEW)
│   ├── interface.go # Evaluator interface
│   ├── ruleBased.go # Rule-based evaluation
│   ├── llmJudge.go  # LLM-as-a-judge evaluation
│   ├── humanLoop.go # Human-in-the-loop evaluation
│   ├── rubric.go    # Rubric-based scoring
│   └── aggregate.go # Aggregate scores across evaluators
├── comparator/      # Orchestrates analyzers + evaluators (IMPLEMENTATION_PLAN + enhanced)
├── reporter/        # Report generation (DRAFT_PLAN - NEW)
└── api/             # REST API (IMPLEMENTATION_PLAN)
```

### Evaluation Framework (NEW)

The evaluation framework assesses model outputs beyond basic similarity, providing actionable scoring for model selection decisions.

#### What is Evaluation?

**Evaluation** answers: "Which model produced the **best** result for this task?"

While **analysis** compares *how* models behaved differently (tool sequences, tokens, latency), **evaluation** judges the *quality* of the final output against success criteria.

**Example**:
```
Task: "Draft an email to customer about refund"

Baseline (GPT-4 Output):
"Dear Customer, your refund has been processed..."

Variant 1 (Claude Output):
"Hi there! Great news - we've processed your refund..."

Variant 2 (GPT-3.5 Output):
"Refund processed. See account for details."

Analysis tells us:
- Token counts: GPT-4 (45), Claude (42), GPT-3.5 (12)
- Cost: GPT-4 ($0.002), Claude ($0.0015), GPT-3.5 ($0.0001)
- Similarity: Claude (0.89), GPT-3.5 (0.65)

Evaluation tells us:
- Tone appropriateness: GPT-4 (9/10), Claude (8/10), GPT-3.5 (4/10)
- Completeness: GPT-4 (10/10), Claude (9/10), GPT-3.5 (6/10)
- Professionalism: GPT-4 (10/10), Claude (7/10), GPT-3.5 (8/10)
→ Winner: GPT-4 (best balance), but Claude is viable alternative
```

#### Evaluation Strategies

**1. Rule-Based Evaluation**
Programmatic checks against output structure and content.

```go
type RuleBasedEvaluator struct {
    Rules []Rule
}

type Rule struct {
    Name        string
    Description string
    Check       func(output string) (bool, error)
    Weight      float64
}

// Example rules:
rules := []Rule{
    {
        Name: "contains_greeting",
        Check: func(output string) (bool, error) {
            return strings.Contains(strings.ToLower(output), "dear") ||
                   strings.Contains(strings.ToLower(output), "hi"), nil
        },
        Weight: 0.2,
    },
    {
        Name: "includes_refund_amount",
        Check: func(output string) (bool, error) {
            return regexp.MatchString(`\$\d+\.\d{2}`, output)
        },
        Weight: 0.3,
    },
    {
        Name: "professional_closing",
        Check: func(output string) (bool, error) {
            return strings.Contains(strings.ToLower(output), "sincerely") ||
                   strings.Contains(strings.ToLower(output), "best regards"), nil
        },
        Weight: 0.2,
    },
}
```

**Use cases**:
- Format validation (JSON structure, required fields)
- Content requirements (must include X, must not include Y)
- Length constraints (min/max words/tokens)
- Pattern matching (email format, code syntax)

**2. LLM-as-a-Judge Evaluation**
Use an LLM to evaluate outputs based on criteria.

```go
type LLMJudgeEvaluator struct {
    JudgeModel   string  // e.g., "claude-3-5-sonnet-20241022"
    Criteria     []string
    ScoreScale   int     // e.g., 1-10
    AgwClient    *agwclient.Client
}

// Example criteria:
criteria := []string{
    "Tone: Is the tone appropriate for customer service?",
    "Clarity: Is the message clear and easy to understand?",
    "Completeness: Does it address all necessary points?",
    "Professionalism: Is the language professional?",
}

// Evaluation prompt:
prompt := fmt.Sprintf(`
You are evaluating the quality of an AI assistant's output.

Task: %s
Output: %s

Evaluate the output on the following criteria (score 1-10 for each):
%s

Provide scores in JSON format:
{
  "scores": {
    "tone": <score>,
    "clarity": <score>,
    "completeness": <score>,
    "professionalism": <score>
  },
  "reasoning": "Brief explanation of scores"
}
`, task, output, strings.Join(criteria, "\n"))
```

**Use cases**:
- Subjective quality assessment (tone, style, appropriateness)
- Semantic correctness (factuality, reasoning quality)
- Creative tasks (humor, engagement, originality)
- Complex criteria (cultural sensitivity, nuance)

**3. Rubric-Based Evaluation**
Structured scoring against predefined rubrics (like grading rubrics in education).

```yaml
# rubric.yaml
rubric:
  name: "Customer Service Email Evaluation"
  description: "Evaluates quality of customer service emails"

  dimensions:
    - name: "tone"
      weight: 0.25
      levels:
        - score: 10
          description: "Warm, empathetic, professional"
          indicators:
            - "Uses customer name"
            - "Acknowledges concern"
            - "Positive language"
        - score: 7
          description: "Professional but neutral"
          indicators:
            - "Polite language"
            - "Clear statements"
        - score: 4
          description: "Cold or robotic"
          indicators:
            - "No personalization"
            - "Terse responses"
        - score: 1
          description: "Unprofessional or inappropriate"
          indicators:
            - "Rude language"
            - "Dismissive tone"

    - name: "completeness"
      weight: 0.35
      levels:
        - score: 10
          description: "Addresses all points thoroughly"
          required_elements:
            - "refund_amount"
            - "processing_timeframe"
            - "next_steps"
        - score: 7
          description: "Addresses main points"
          required_elements:
            - "refund_amount"
            - "processing_timeframe"
        - score: 4
          description: "Missing key information"
          required_elements:
            - "refund_amount"
        - score: 1
          description: "Does not address the issue"

    - name: "clarity"
      weight: 0.2
      criteria:
        - "Uses simple language"
        - "Avoids jargon"
        - "Logical structure"

    - name: "actionability"
      weight: 0.2
      criteria:
        - "Clear next steps"
        - "Contact information provided"
        - "Timeline specified"

  pass_threshold: 7.0  # Minimum overall score to pass
```

**Implementation**:
```go
type RubricEvaluator struct {
    Rubric *Rubric
}

type Rubric struct {
    Name          string
    Description   string
    Dimensions    []RubricDimension
    PassThreshold float64
}

type RubricDimension struct {
    Name   string
    Weight float64
    Levels []RubricLevel
}

type RubricLevel struct {
    Score       int
    Description string
    Indicators  []string
}

func (r *RubricEvaluator) Evaluate(output string) (*EvaluationResult, error) {
    scores := make(map[string]float64)

    for _, dim := range r.Rubric.Dimensions {
        score := r.scoreOnDimension(output, dim)
        scores[dim.Name] = score
    }

    overallScore := r.calculateWeightedScore(scores)

    return &EvaluationResult{
        Scores:       scores,
        OverallScore: overallScore,
        Passed:       overallScore >= r.Rubric.PassThreshold,
    }, nil
}
```

**Use cases**:
- Standardized evaluation across multiple experiments
- Consistent scoring criteria
- Weighted multi-dimensional assessment
- Pass/fail gates for model selection

**4. Human-in-the-Loop Evaluation**
Queue outputs for human review when automated evaluation is insufficient.

```go
type HumanLoopEvaluator struct {
    QueueService *EvaluationQueue
}

type HumanEvaluation struct {
    ID             string
    ExperimentID   string
    RunID          string
    Output         string
    Context        map[string]interface{}
    Status         string  // pending, reviewed, skipped
    ReviewerID     string
    Scores         map[string]float64
    Feedback       string
    ReviewedAt     time.Time
}

// Submit for human review
func (h *HumanLoopEvaluator) QueueForReview(ctx context.Context, output string, context map[string]interface{}) (string, error) {
    eval := &HumanEvaluation{
        ID:       uuid.New().String(),
        Output:   output,
        Context:  context,
        Status:   "pending",
    }

    return eval.ID, h.QueueService.Enqueue(ctx, eval)
}

// API endpoints:
// POST   /api/v1/evaluations/human/queue     - Submit for review
// GET    /api/v1/evaluations/human/pending   - Get pending reviews
// POST   /api/v1/evaluations/human/:id/review - Submit review
// GET    /api/v1/evaluations/human/:id       - Get review status
```

**Use cases**:
- Edge cases automated evaluation can't handle
- High-stakes decisions requiring human judgment
- Training data collection for future automated evaluators
- Spot-checking automated evaluation accuracy

**5. Ground Truth Comparison**
Compare outputs against known correct answers (for tasks with deterministic answers).

```go
type GroundTruthEvaluator struct {
    GroundTruthStore *GroundTruthStore
    SimilarityFunc   func(output, groundTruth string) float64
}

type GroundTruth struct {
    TaskID   string
    Answer   string
    Metadata map[string]interface{}
}

func (g *GroundTruthEvaluator) Evaluate(taskID, output string) (*EvaluationResult, error) {
    groundTruth, err := g.GroundTruthStore.Get(taskID)
    if err != nil {
        return nil, err
    }

    similarity := g.SimilarityFunc(output, groundTruth.Answer)

    return &EvaluationResult{
        Scores: map[string]float64{
            "accuracy": similarity,
        },
        OverallScore: similarity,
        Passed:       similarity >= 0.9,
    }, nil
}
```

**Use cases**:
- Code generation (compare against reference implementation)
- Data extraction (compare against labeled dataset)
- Math/logic problems (compare against correct answer)
- Translation (compare against reference translation)

#### Evaluation Interface

```go
// Common interface all evaluators implement
type Evaluator interface {
    Evaluate(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error)
    Name() string
    Type() EvaluatorType
}

type EvaluationRequest struct {
    ExperimentID string
    RunID        string
    TaskContext  map[string]interface{}
    Output       string
    Baseline     *string  // Optional baseline for comparison
}

type EvaluationResult struct {
    EvaluatorName string
    EvaluatorType EvaluatorType

    // Scores on various dimensions
    Scores map[string]float64

    // Overall score (0-1 normalized)
    OverallScore float64

    // Pass/fail determination
    Passed bool

    // Human-readable explanation
    Reasoning string

    // Additional metadata
    Metadata map[string]interface{}

    // Timestamp
    EvaluatedAt time.Time
}

type EvaluatorType string
const (
    EvaluatorTypeRule        EvaluatorType = "rule"
    EvaluatorTypeLLMJudge    EvaluatorType = "llm_judge"
    EvaluatorTypeRubric      EvaluatorType = "rubric"
    EvaluatorTypeHuman       EvaluatorType = "human"
    EvaluatorTypeGroundTruth EvaluatorType = "ground_truth"
)
```

#### Evaluation Configuration

Users can configure multiple evaluators for an experiment:

```yaml
# evaluation.yaml
evaluation:
  evaluators:
    # Rule-based checks
    - type: rule
      name: "format_validation"
      enabled: true
      rules:
        - name: "valid_json"
          check: "is_valid_json"
          weight: 1.0
        - name: "required_fields"
          check: "has_fields"
          params:
            fields: ["status", "message", "data"]
          weight: 1.0

    # LLM judge for quality
    - type: llm_judge
      name: "quality_assessment"
      enabled: true
      judge_model: "claude-3-5-sonnet-20241022"
      criteria:
        - "Clarity: Is the output clear and understandable?"
        - "Accuracy: Is the information correct?"
        - "Helpfulness: Does it address the user's needs?"
      score_scale: 10
      weight: 2.0

    # Rubric-based evaluation
    - type: rubric
      name: "customer_service_rubric"
      enabled: true
      rubric_file: "./rubrics/customer_service.yaml"
      weight: 1.5

    # Ground truth comparison (if available)
    - type: ground_truth
      name: "accuracy_check"
      enabled: false  # Only enable if ground truth exists
      similarity_threshold: 0.85
      weight: 2.0

    # Human review for edge cases
    - type: human
      name: "manual_review"
      enabled: true
      queue_conditions:
        - "overall_score < 0.7"
        - "rule_evaluator_failed"
        - "random_sample: 0.1"  # Review 10% randomly
      weight: 3.0

  # Aggregation strategy
  aggregation: "weighted_average"  # or "min", "max", "median"

  # Pass threshold
  pass_threshold: 0.75

  # Fail-fast conditions
  fail_fast:
    - "rule:format_validation < 1.0"
    - "safety_violation == true"
```

#### Evaluation Storage Schema

```sql
-- Evaluators configuration
CREATE TABLE evaluators (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE,
    type VARCHAR(50),  -- rule, llm_judge, rubric, human, ground_truth
    config JSONB,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Evaluation runs (one per experiment run)
CREATE TABLE evaluation_runs (
    id UUID PRIMARY KEY,
    experiment_run_id UUID REFERENCES experiment_runs(id),
    status VARCHAR(50),  -- pending, running, completed, failed
    started_at TIMESTAMP,
    completed_at TIMESTAMP
);

-- Individual evaluator results
CREATE TABLE evaluator_results (
    id SERIAL PRIMARY KEY,
    evaluation_run_id UUID REFERENCES evaluation_runs(id),
    evaluator_id INT REFERENCES evaluators(id),

    scores JSONB,  -- Dimension scores
    overall_score FLOAT,
    passed BOOLEAN,
    reasoning TEXT,
    metadata JSONB,

    evaluated_at TIMESTAMP
);

-- Human evaluation queue
CREATE TABLE human_evaluation_queue (
    id UUID PRIMARY KEY,
    evaluation_run_id UUID REFERENCES evaluation_runs(id),
    experiment_run_id UUID REFERENCES experiment_runs(id),

    output TEXT,
    context JSONB,

    status VARCHAR(50),  -- pending, in_review, completed, skipped
    assigned_to VARCHAR(255),

    -- Human-provided scores
    scores JSONB,
    feedback TEXT,

    created_at TIMESTAMP,
    assigned_at TIMESTAMP,
    reviewed_at TIMESTAMP
);

-- Ground truth data
CREATE TABLE ground_truth (
    id SERIAL PRIMARY KEY,
    task_id VARCHAR(255) UNIQUE,
    task_type VARCHAR(100),
    input JSONB,
    expected_output TEXT,
    metadata JSONB,
    created_at TIMESTAMP
);

-- Aggregate evaluation results (rollup)
CREATE TABLE evaluation_summary (
    id SERIAL PRIMARY KEY,
    experiment_id UUID REFERENCES experiments(id),
    experiment_run_id UUID REFERENCES experiment_runs(id),

    -- Aggregated scores
    overall_score FLOAT,
    passed BOOLEAN,

    -- Per-evaluator scores
    evaluator_scores JSONB,

    -- Winner determination
    rank INT,
    is_best BOOLEAN,

    created_at TIMESTAMP
);
```

#### Evaluation API Endpoints

```go
// REST API endpoints for evaluation
router.HandleFunc("/api/v1/evaluators", api.ListEvaluators).Methods("GET")
router.HandleFunc("/api/v1/evaluators", api.CreateEvaluator).Methods("POST")
router.HandleFunc("/api/v1/evaluators/{id}", api.GetEvaluator).Methods("GET")
router.HandleFunc("/api/v1/evaluators/{id}", api.UpdateEvaluator).Methods("PUT")
router.HandleFunc("/api/v1/evaluators/{id}", api.DeleteEvaluator).Methods("DELETE")

router.HandleFunc("/api/v1/experiments/{id}/evaluate", api.TriggerEvaluation).Methods("POST")
router.HandleFunc("/api/v1/experiments/{id}/evaluation", api.GetEvaluationResults).Methods("GET")
router.HandleFunc("/api/v1/experiments/{id}/evaluation/summary", api.GetEvaluationSummary).Methods("GET")

router.HandleFunc("/api/v1/evaluations/human/queue", api.QueueForHumanReview).Methods("POST")
router.HandleFunc("/api/v1/evaluations/human/pending", api.GetPendingReviews).Methods("GET")
router.HandleFunc("/api/v1/evaluations/human/{id}/review", api.SubmitHumanReview).Methods("POST")
router.HandleFunc("/api/v1/evaluations/human/{id}", api.GetHumanReview).Methods("GET")

router.HandleFunc("/api/v1/ground-truth", api.ListGroundTruth).Methods("GET")
router.HandleFunc("/api/v1/ground-truth", api.CreateGroundTruth).Methods("POST")
router.HandleFunc("/api/v1/ground-truth/{id}", api.UpdateGroundTruth).Methods("PUT")
```

#### CLI Commands for Evaluation

```bash
# Configure evaluators for experiment
cmdr eval config \
  --experiment <experiment_id> \
  --config evaluation.yaml

# Run evaluation on completed experiment
cmdr eval run \
  --experiment <experiment_id> \
  --evaluators rule_check,llm_judge,rubric

# Get evaluation results
cmdr eval results \
  --experiment <experiment_id> \
  --format json|table|markdown

# Queue for human review
cmdr eval human queue \
  --experiment <experiment_id> \
  --run <run_id>

# Submit human review
cmdr eval human review \
  --review-id <review_id> \
  --scores '{"clarity": 8, "accuracy": 9}' \
  --feedback "Good output but could be more concise"

# List pending reviews
cmdr eval human pending

# Get evaluation summary with winner
cmdr eval summary \
  --experiment <experiment_id>
```

#### Evaluation Report Output

```json
{
  "experiment_id": "exp-123",
  "baseline": {
    "run_id": "run-baseline",
    "model": "gpt-4",
    "evaluation": {
      "overall_score": 0.87,
      "passed": true,
      "evaluators": {
        "rule_check": {
          "score": 1.0,
          "passed": true,
          "details": {
            "valid_json": true,
            "required_fields": true
          }
        },
        "llm_judge": {
          "score": 0.85,
          "passed": true,
          "scores": {
            "clarity": 9,
            "accuracy": 8,
            "helpfulness": 8
          },
          "reasoning": "Clear and accurate response..."
        },
        "rubric": {
          "score": 0.80,
          "passed": true,
          "dimension_scores": {
            "tone": 9,
            "completeness": 8,
            "clarity": 7,
            "actionability": 8
          }
        }
      },
      "rank": 1,
      "is_winner": true
    }
  },
  "variants": [
    {
      "run_id": "run-variant-1",
      "model": "claude-3-5-sonnet-20241022",
      "evaluation": {
        "overall_score": 0.82,
        "passed": true,
        "evaluators": { /* ... */ },
        "rank": 2,
        "is_winner": false
      }
    },
    {
      "run_id": "run-variant-2",
      "model": "gpt-3.5-turbo",
      "evaluation": {
        "overall_score": 0.65,
        "passed": false,
        "evaluators": { /* ... */ },
        "rank": 3,
        "is_winner": false,
        "failure_reasons": [
          "Overall score below threshold (0.65 < 0.75)",
          "LLM judge clarity score too low (4/10)"
        ]
      }
    }
  ],
  "winner": {
    "run_id": "run-baseline",
    "model": "gpt-4",
    "overall_score": 0.87,
    "reasons": [
      "Highest overall score (0.87)",
      "Passed all evaluators",
      "Best clarity and completeness scores"
    ]
  },
  "summary": {
    "total_runs": 3,
    "passed": 2,
    "failed": 1,
    "best_model": "gpt-4",
    "cost_efficient_alternative": {
      "model": "claude-3-5-sonnet-20241022",
      "score": 0.82,
      "cost_savings": "25%",
      "trade_offs": "Slightly lower clarity but comparable accuracy"
    }
  }
}
```

#### Integration with Replay Flow

```
1. Capture Baseline (with Freeze-Tools)
   ↓
2. Replay Variants (frozen tools)
   ↓
3. Analysis (4D: behavior, safety, quality, efficiency)
   ↓
4. Evaluation (score outputs with multiple evaluators)
   ↓  - Rule-based (format, content validation)
   ↓  - LLM-judge (quality assessment)
   ↓  - Rubric (standardized scoring)
   ↓  - Ground truth (accuracy comparison)
   ↓  - Human review (edge cases, final judgment)
   ↓
5. Ranking (determine winner based on aggregated scores)
   ↓
6. Report (comprehensive scorecard with recommendations)
```

**Evaluation adds value by**:
- Converting analysis diffs into actionable scores
- Enabling apples-to-apples comparison across quality dimensions
- Providing pass/fail gates for automated decision-making
- Supporting human-in-the-loop for high-stakes scenarios
- Building ground truth datasets for future automation

#### Evaluation Best Practices

**1. Use Multiple Evaluators**
Combine different evaluation strategies for robust assessment:
- Rule-based for hard requirements
- LLM-judge for subjective quality
- Rubric for standardized scoring
- Human-in-loop for edge cases

**2. Weight Appropriately**
Assign weights based on importance:
```yaml
evaluators:
  - type: rule
    weight: 2.0  # Hard requirements are critical
  - type: llm_judge
    weight: 1.0  # Quality matters but flexible
  - type: human
    weight: 3.0  # Human judgment is final authority
```

**3. Set Clear Pass Thresholds**
Define minimum acceptable scores:
```yaml
pass_threshold: 0.75
fail_fast:
  - "rule:format_validation < 1.0"  # Must pass format checks
  - "safety_score < 0.9"             # High bar for safety
```

**4. Enable Human Review Strategically**
Queue for human review when:
- Automated scores are borderline (0.7-0.8 range)
- High-stakes decisions (production deployment)
- Building training data for future automation
- Random sampling for quality assurance (10%)

**5. Track Evaluation Metrics**
Monitor evaluation system health:
- Inter-evaluator agreement
- Human override rate
- Evaluation latency
- Cost per evaluation

### Updated Schema

**Enhanced schema** (combining both plans):

```sql
-- OTEL Traces (raw capture)
CREATE TABLE otel_traces (
    trace_id VARCHAR(255) PRIMARY KEY,
    span_id VARCHAR(255),
    parent_span_id VARCHAR(255),
    service_name VARCHAR(255),
    span_kind VARCHAR(50),
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    attributes JSONB,  -- All OTEL attributes
    events JSONB,      -- OTEL events
    status JSONB       -- OTEL status
);

-- Parsed Traces (replay-specific)
CREATE TABLE replay_traces (
    trace_id VARCHAR(255) PRIMARY KEY,
    run_id VARCHAR(255),
    created_at TIMESTAMP,
    provider VARCHAR(100),
    model VARCHAR(255),
    prompt JSONB,       -- Array of messages
    completion TEXT,
    parameters JSONB,   -- LLM params (temp, top_p, etc)
    prompt_tokens INT,
    completion_tokens INT,
    total_tokens INT,
    latency_ms INT
);

-- Captured Tool Calls (for Freeze-Tools)
CREATE TABLE tool_captures (
    id SERIAL PRIMARY KEY,
    trace_id VARCHAR(255),
    step_index INT,
    tool_name VARCHAR(255),
    args JSONB,              -- Normalized JSON
    args_hash VARCHAR(64),   -- SHA256 of normalized args (for fast lookup)
    result JSONB,
    error TEXT,
    latency_ms INT,
    risk_class VARCHAR(50),  -- read, write, destructive
    created_at TIMESTAMP,
    UNIQUE(trace_id, step_index)
);

-- Experiments (replay jobs)
CREATE TABLE experiments (
    id UUID PRIMARY KEY,
    name VARCHAR(255),
    baseline_trace_id VARCHAR(255) REFERENCES replay_traces(trace_id),
    status VARCHAR(50),  -- pending, running, completed, failed
    progress FLOAT,
    created_at TIMESTAMP,
    completed_at TIMESTAMP,
    config JSONB  -- Variant matrix, evaluation weights
);

-- Experiment Runs (one per variant)
CREATE TABLE experiment_runs (
    id UUID PRIMARY KEY,
    experiment_id UUID REFERENCES experiments(id),
    variant_config JSONB,  -- Model, prompt, policy overrides
    trace_id VARCHAR(255),
    status VARCHAR(50),
    created_at TIMESTAMP,
    completed_at TIMESTAMP
);

-- Analysis Results
CREATE TABLE analysis_results (
    id SERIAL PRIMARY KEY,
    experiment_id UUID REFERENCES experiments(id),
    baseline_run_id UUID,
    candidate_run_id UUID REFERENCES experiment_runs(id),

    -- Behavior analysis
    behavior_diff JSONB,  -- Tool sequence comparison
    first_divergence JSONB,  -- Where/why models diverged

    -- Safety analysis
    safety_diff JSONB,  -- Risk class changes

    -- Quality analysis
    similarity_score FLOAT,
    quality_metrics JSONB,

    -- Efficiency analysis
    token_delta INT,
    cost_delta FLOAT,
    latency_delta INT,

    created_at TIMESTAMP
);

-- Evaluators configuration (NEW)
CREATE TABLE evaluators (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE,
    type VARCHAR(50),  -- rule, llm_judge, rubric, human, ground_truth
    config JSONB,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Evaluation runs (NEW)
CREATE TABLE evaluation_runs (
    id UUID PRIMARY KEY,
    experiment_run_id UUID REFERENCES experiment_runs(id),
    status VARCHAR(50),  -- pending, running, completed, failed
    started_at TIMESTAMP,
    completed_at TIMESTAMP
);

-- Individual evaluator results (NEW)
CREATE TABLE evaluator_results (
    id SERIAL PRIMARY KEY,
    evaluation_run_id UUID REFERENCES evaluation_runs(id),
    evaluator_id INT REFERENCES evaluators(id),
    scores JSONB,  -- Dimension scores
    overall_score FLOAT,
    passed BOOLEAN,
    reasoning TEXT,
    metadata JSONB,
    evaluated_at TIMESTAMP
);

-- Human evaluation queue (NEW)
CREATE TABLE human_evaluation_queue (
    id UUID PRIMARY KEY,
    evaluation_run_id UUID REFERENCES evaluation_runs(id),
    experiment_run_id UUID REFERENCES experiment_runs(id),
    output TEXT,
    context JSONB,
    status VARCHAR(50),  -- pending, in_review, completed, skipped
    assigned_to VARCHAR(255),
    scores JSONB,
    feedback TEXT,
    created_at TIMESTAMP,
    assigned_at TIMESTAMP,
    reviewed_at TIMESTAMP
);

-- Ground truth data (NEW)
CREATE TABLE ground_truth (
    id SERIAL PRIMARY KEY,
    task_id VARCHAR(255) UNIQUE,
    task_type VARCHAR(100),
    input JSONB,
    expected_output TEXT,
    metadata JSONB,
    created_at TIMESTAMP
);

-- Aggregate evaluation results (NEW)
CREATE TABLE evaluation_summary (
    id SERIAL PRIMARY KEY,
    experiment_id UUID REFERENCES experiments(id),
    experiment_run_id UUID REFERENCES experiment_runs(id),
    overall_score FLOAT,
    passed BOOLEAN,
    evaluator_scores JSONB,  -- Per-evaluator breakdown
    rank INT,
    is_best BOOLEAN,
    created_at TIMESTAMP
);
```

### CLI Surface (from DRAFT_PLAN)

```bash
# Submit experiment
cmdr experiment run \
  --baseline-trace abc123 \
  --variants variants.yaml \
  --output report.json

# Check status
cmdr experiment status <experiment_id>

# Get results
cmdr experiment results <experiment_id> \
  --format json|markdown \
  --output report.md

# Compare two specific runs
cmdr diff \
  --baseline <run_id> \
  --candidate <run_id> \
  --analysis behavior,safety,quality,efficiency

# Evaluate experiment runs (NEW)
cmdr eval run \
  --experiment <experiment_id> \
  --config evaluation.yaml

# Get evaluation results (NEW)
cmdr eval results \
  --experiment <experiment_id> \
  --format json|table|markdown

# Get evaluation summary with winner (NEW)
cmdr eval summary \
  --experiment <experiment_id>

# Configure evaluators (NEW)
cmdr eval config \
  --experiment <experiment_id> \
  --evaluators rule_check,llm_judge,rubric

# Human review commands (NEW)
cmdr eval human queue \
  --experiment <experiment_id> \
  --run <run_id>

cmdr eval human pending

cmdr eval human review \
  --review-id <review_id> \
  --scores '{"clarity": 8, "accuracy": 9}' \
  --feedback "Good output"

# Ground truth management (NEW)
cmdr ground-truth add \
  --task-id task-123 \
  --input input.json \
  --expected-output expected.txt

cmdr ground-truth list

# Legacy eval command (kept for compatibility)
cmdr eval \
  --experiment <experiment_id> \
  --rubric rubric.yaml
```

**Variants YAML example**:
```yaml
baseline:
  trace_id: abc123

variants:
  - name: claude-sonnet
    model: claude-3-5-sonnet-20241022

  - name: gpt4
    model: gpt-4

  - name: prompt-variant
    model: claude-3-5-sonnet-20241022
    prompt_template: alternative_prompt.txt

analysis:
  weights:
    behavior: 0.3
    safety: 0.4
    quality: 0.2
    efficiency: 0.1

  fail_on:
    - new_destructive_tools
    - policy_violations

# NEW: Evaluation configuration
evaluation:
  evaluators:
    - type: rule
      name: format_validation
      enabled: true
      rules:
        - name: valid_json
          check: is_valid_json
          weight: 1.0

    - type: llm_judge
      name: quality_assessment
      enabled: true
      judge_model: claude-3-5-sonnet-20241022
      criteria:
        - "Clarity: Is the output clear?"
        - "Accuracy: Is the information correct?"
        - "Helpfulness: Does it help the user?"
      weight: 2.0

    - type: rubric
      name: customer_service
      enabled: true
      rubric_file: ./rubrics/customer_service.yaml
      weight: 1.5

    - type: human
      name: manual_review
      enabled: true
      queue_conditions:
        - "overall_score < 0.7"
        - "random_sample: 0.1"
      weight: 3.0

  aggregation: weighted_average
  pass_threshold: 0.75
```

### Implementation Priorities

**Phase 1: Foundation + OTEL + Freeze-Tools** (Week 1)
1. Initialize Go module, project structure
2. Implement OTLP receiver (consume OTEL traces)
3. Implement trace parser (OTEL spans → replay schema)
4. Implement storage layer (Postgres with evaluation tables)
5. Implement Freeze-Tools MCP server (capture + freeze modes)
6. Unit tests for all components

**Phase 2: Replay Engine + Agentgateway Client** (Week 2)
1. Implement agentgateway HTTP client
2. Implement experiment orchestration engine
3. Implement worker pool for concurrent replays
4. Integrate Freeze-Tools into replay flow
5. End-to-end replay test (baseline capture → variant replay)

**Phase 3: Analysis + Evaluation Framework** (Week 3)
1. Implement behavior analyzer (tool sequence, arg drift)
2. Implement safety analyzer (risk class tracking)
3. Implement quality analyzer (similarity scoring)
4. Implement efficiency analyzer (tokens, cost, latency)
5. **NEW**: Implement evaluation framework:
   - Evaluator interface and registry
   - Rule-based evaluator
   - LLM-judge evaluator
   - Rubric evaluator
   - Human-loop evaluator (queue + API)
   - Ground-truth evaluator
6. **NEW**: Implement evaluation aggregation and winner determination
7. Add first-divergence explanation logic

**Phase 4: Reporting + API + CLI + Hardening** (Week 4)
1. Implement report generator (JSON + Markdown with evaluation scorecards)
2. Implement REST API:
   - Experiment endpoints (job CRUD, results)
   - **NEW**: Evaluation endpoints (configure, run, results, summary)
   - **NEW**: Human review endpoints (queue, pending, submit review)
   - **NEW**: Ground truth endpoints (CRUD)
3. Implement CLI with cobra:
   - Experiment commands (run, status, results, diff)
   - **NEW**: Evaluation commands (run, results, summary, config)
   - **NEW**: Human review commands (queue, pending, review)
   - **NEW**: Ground truth commands (add, list)
4. Add authentication/authorization (optional)
5. Performance testing and optimization
6. Documentation:
   - API documentation with evaluation endpoints
   - Architecture docs with evaluation flow
   - Usage guides with evaluation examples
   - Rubric creation guide
7. Docker + Kubernetes deployment configs

### Success Criteria

✅ **Deterministic Replay**:
- Same baseline trace replayed with different models produces identical tool calls
- Freeze-Tools correctly matches tool calls by normalized arguments
- Zero tool re-execution during replay phase

✅ **Full Behavior Analysis**:
- Tool sequence diff accurately identifies added/removed/reordered calls
- Argument drift detection with normalized JSON comparison
- First divergence point correctly identified with explanation

✅ **Safety Analysis**:
- Risk class tracking (read/write/destructive)
- Detects when new model attempts risky operations baseline didn't

✅ **Quality Analysis**:
- Semantic similarity scoring between outputs
- Rubric-based evaluation (configurable criteria)

✅ **Efficiency Analysis**:
- Token count deltas calculated accurately
- Cost estimates using configurable pricing
- Latency comparisons (model + e2e)

✅ **Evaluation Framework** (NEW):
- Multiple evaluator types implemented (rule-based, LLM-judge, rubric, human-loop, ground-truth)
- Evaluators can be composed and weighted
- Pass/fail thresholds configurable per evaluator
- Human evaluation queue functional with pending/review workflow
- Evaluation results stored with full audit trail
- Winner determination based on aggregated scores
- Evaluation summary includes rankings and recommendations

✅ **API + CLI**:
- REST API accepts experiments, returns job IDs
- CLI submits experiments and retrieves results
- Results available in JSON and Markdown formats
- Evaluation API endpoints functional (configure, run, results, human review)
- CLI evaluation commands working (run, results, summary, human queue/review)

✅ **Performance**:
- Worker pool handles concurrent replays efficiently
- OTLP receiver handles trace ingestion without backpressure
- Postgres storage performs well with large trace volumes
- LLM-judge evaluations don't block experiment completion (async)
- Human evaluation queue scales to hundreds of pending reviews

✅ **Testing**:
- Unit test coverage > 80%
- Integration tests with real Jaeger + agentgateway
- E2E test: capture baseline → replay variants → analyze → evaluate → report
- Evaluation framework tests: each evaluator type has comprehensive tests
- Human-in-loop simulation tests (mock human reviews)
