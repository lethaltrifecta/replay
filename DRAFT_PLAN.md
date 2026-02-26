# CMDR Draft Implementation Plan

## Product Definition
CMDR is a **replay-backed behavior analysis lab** for LLM agents.

Replay is the control layer that keeps environment conditions fixed.
The core product is comparative analysis across models and scenarios.

## Mission
Help engineers answer:
1. How does model behavior change across the same task context?
2. Why do results differ between models or policy/prompt/tool scenarios?
3. Which scenarios are robust vs brittle?

## Primary Use Cases
1. Compare model behavior on the same real captured run.
2. Run scenario experiments:
   - prompt variants
   - policy variants
   - tool availability variants
   - retrieval context variants
3. Produce explainable model diffs and evaluation scorecards.

## Scope Decision
Use a hybrid architecture:
1. OTEL for trace discovery and debugging.
2. Replay store as deterministic source of truth for analysis.

## Why This Is Distinct
1. Deterministic environment pinning (`freeze-tools`) as a first-class primitive.
2. Protocol/gateway-native replay for MCP/tool graphs without SDK assumptions.
3. Behavior-first diffing (tool graph + risk class + first divergence), not only output scoring.
4. Scenario replay lab: same trace, swap model/prompt/policy/tool availability and explain causality.

## Adjacent Options in Market
1. LangSmith: strong production tracing, dataset/experiment management, online evals, and trace comparison workflows for rapid iteration.
2. Langfuse: strong open observability foundation (OTEL-friendly), prompt/version tracking, and experiment/eval data modeling for analytics-first teams.
3. Braintrust: strong scorer-based evaluation pipelines, pairwise/model comparisons, and optimization loops for improving prompts and model choices.
4. Promptfoo: strong adversarial testing and compliance-oriented red-team workflows with configurable attack/eval plugins.
5. OpenAI eval stack: strong trace grading, agent eval APIs, and prompt optimization workflows tightly integrated with model APIs.
6. Inspect (UK AISI): strong high-assurance eval architecture with explicit sandboxing, tool control, and agent-task reproducibility focus.
7. LangGraph time travel: strong checkpointed execution, stateful replay/fork debugging, and failure forensics for graph-based agents.

## Non-Negotiable MVP Requirements
1. Zero agent SDK changes.
2. Deterministic Freeze-Tools replay mode.
3. Per-run comparison across multiple models/variants.
4. Multi-dimensional analysis output:
   - tool-graph behavior
   - argument and policy deltas
   - response quality deltas
   - latency and cost deltas
5. First-divergence explanation in every diff report.
6. Runtime overhead target: p95 added latency under 5%.

## Analysis Dimensions
### 1) Behavior
- Tool-call sequence drift.
- Missing/extra tool calls.
- Argument drift (normalized JSON diff).

### 2) Safety/Policy
- New risky tool behaviors (`write`, `destructive`).
- Policy decision changes (`allow`, `deny`, `transform`).

### 3) Result Quality
- Semantic similarity to baseline.
- Optional rubric score for scenario-specific expectations.

### 4) Efficiency
- End-to-end latency delta.
- Model latency and token/cost deltas.

## Minimal Gateway Changes
1. Feature-flagged replay capture sink.
2. Canonical monotonic `step_index`.
3. Stable correlation IDs: `run_id`, `trace_id`, `experiment_id`.
4. Per-run override support for:
   - model/provider
   - policy bundle
   - tool endpoint (Replay MCP)

## Replay Schema (v1)
### runs
- `run_id`, `trace_id`, `experiment_id`
- `scenario_id`, `variant_id`, `tenant_id`, `app_id`
- `start_ts`, `end_ts`, `status`
- `schema_version`

### steps
- `run_id`, `step_index` (canonical order)
- `step_type` (`llm`, `tool`, `policy`, `io`)
- `ts_start`, `ts_end`, `summary`

### llm_calls
- `run_id`, `step_index`
- `provider`, `model`, `params`
- `request_ref`, `response_ref`
- token and latency metrics

### tool_calls
- `run_id`, `step_index`
- `tool_name`, `args_ref`, `result_ref`
- `latency_ms`, `error`
- `risk_class` (`read`, `write`, `destructive`)

### policy_events
- `run_id`, `step_index`
- `policy_name`, `decision` (`allow`, `deny`, `transform`)
- `reason_ref`, transformed refs

## Core Components
1. Capture sink in gateway.
2. Replay store (SQLite MVP, pluggable later).
3. Freeze-Tools replay MCP server.
4. Experiment orchestrator (baseline + N variants).
5. Differential analysis engine.
6. Evaluation score engine.
7. Reporting layer (JSON + Markdown).

## CLI Surface (MVP)
1. `cmdr experiment run --baseline <RUN> --variant <SPEC>...`
2. `cmdr diff --baseline <RUN> --candidate <RUN>`
3. `cmdr eval --baseline <RUN> --candidate <RUN> --scenario <YAML>`
4. `cmdr report --experiment <ID> --format json,md`

## Scenario Configuration (YAML)
Each scenario defines:
1. Dataset or trace selection.
2. Variant matrix:
   - model/provider params
   - prompt bundle
   - policy bundle
   - tool constraints
3. Evaluation weights and thresholds.
4. Optional hard-fail checks for risky behaviors.

## Implementation Plan
### Phase 1 (Week 1): Capture + Schema
1. Finalize replay schema.
2. Implement async sink and storage.
3. Add redaction hooks before persistence.

### Phase 2 (Week 2): Replay + Experiment Orchestration
1. Implement Freeze-Tools replay MCP server.
2. Build experiment runner for baseline + variant matrix.
3. Verify deterministic replays.

### Phase 3 (Week 3): Diff + Eval + Reports
1. Implement behavior/policy diffs.
2. Add quality and efficiency scoring.
3. Add first-divergence explanations and reports.

### Phase 4 (Week 4): Hardening
1. Load/perf validation.
2. Retention and cleanup.
3. Documentation and reproducible demo workflows.

## Potential Nice-to-Haves
1. Full web dashboard.
2. Minimal repro shrinker.
3. Full-Freeze mode.
4. Enterprise multi-region tenancy features.

## Implementation Risks and Mitigations
1. OTEL-only data gaps.
   - Mitigation: replay store as source of truth.
2. Sensitive payload handling.
   - Mitigation: redaction profiles and retention TTL.
3. Capture backpressure.
   - Mitigation: bounded queue and prioritized dropping.
4. Tool nondeterminism.
   - Mitigation: strict Freeze-Tools for experiments.

## Demo Narrative
1. Capture one baseline multi-step workflow.
2. Run the same scenario across 3 models.
3. Show first divergence for each model with behavior diffs.
4. Show scorecard: quality, safety, latency, and cost tradeoffs.
5. Export a report to choose the best model for that scenario.
