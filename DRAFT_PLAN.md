# CMDR Draft Implementation Plan

## Product Definition
CMDR is a **replay-backed evaluation and model-diff system** for safe agent model upgrades.

Replay is the deterministic substrate, not the final product.

Primary user value:
1. Compare candidate models/prompts/policies on real traffic.
2. Score behavior quality and safety, not just output text.
3. Gate rollout in CI/CD with explicit pass/fail reasons.

## Mission
Enable safe, auditable, and automated model migration for MCP/agent systems by running baseline vs candidate evaluations over captured runs.

## Scope Decision
Use a hybrid architecture:
1. OTEL for observability, search, and correlation.
2. Replay store for deterministic re-execution and evaluation-grade data.

Ship this as an **evaluation platform** with three core engines:
1. Replay engine.
2. Differential analysis engine.
3. Evaluation and gating engine.

## Non-Negotiable MVP Requirements
1. Zero SDK changes for agent applications.
2. Deterministic Freeze-Tools mode for reliable candidate comparison.
3. Multi-dimension evaluation per run:
   - behavior/tool-graph correctness
   - safety/policy outcomes
   - response quality (semantic + rubric)
   - latency and cost
4. Actionable model diff output:
   - first divergence step
   - what changed
   - severity and rollout impact
5. CI-first operation:
   - `json` artifact
   - `markdown` summary
   - exit code for gate pass/fail
6. Runtime overhead target: p95 added latency under 5%.

## Evaluation Dimensions (Core)
### 1) Behavioral Correctness
- Tool-call graph drift (order, insertions, missing required calls).
- Argument drift against normalized and policy-aware constraints.

### 2) Safety and Policy
- New destructive/write calls are high severity.
- Policy decision regressions (allow/deny/transform deltas).
- Forbidden-tool or forbidden-data exposure checks.

### 3) Output Quality
- Embedding similarity to baseline response.
- Optional deterministic rubric judge for workflow-specific expectations.

### 4) Efficiency
- Latency delta (end-to-end + model latency).
- Token/cost delta.

## Minimal Gateway Changes
1. Feature-flagged replay-grade capture sink.
2. Canonical monotonic `step_index` per run.
3. Stable run identity fields:
   - `run_id`
   - `baseline_run_id`
   - `trace_id`
4. Per-run candidate override support for:
   - model
   - policy bundle
   - tool endpoint (Replay MCP)

## Replay Schema (v1)
### runs
- `run_id`, `baseline_run_id`, `trace_id`
- `tenant_id`, `app_id`, `variant_id`
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
- token and latency fields

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
3. Freeze-Tools mock MCP server (strict mode first).
4. Replay orchestrator (baseline vs N candidates).
5. Differential engine (behavior + policy + output + perf).
6. Evaluation/gate engine (rules, thresholds, severity).

## CLI Surface (MVP)
1. `cmdr replay run --baseline <RUN> --variant <SPEC>`
2. `cmdr diff --baseline <RUN> --candidate <RUN>`
3. `cmdr eval --baseline <RUN> --candidate <RUN> --workflow <YAML>`
4. `cmdr gate --baseline <RUN> --candidate <RUN> --workflow <YAML>`

## Gate Policy Defaults
Hard fail:
1. Any new `destructive` tool call.
2. Missing required tool calls for protected workflows.
3. Unknown tool calls in strict replay mode.
4. Increased policy-denied actions on protected workflows.

Soft threshold fail (configurable):
1. Tool sequence similarity below threshold.
2. Argument drift outside configured limits.
3. Output quality score below threshold.
4. Latency/cost budget breach.

## Implementation Plan
### Phase 1 (Week 1): Capture + Schema
1. Define schema and storage contract.
2. Implement sink with async bounded queue and metrics.
3. Add redaction hooks before persistence.

### Phase 2 (Week 2): Replay Foundation
1. Implement Freeze-Tools replay MCP server.
2. Build replay orchestrator for candidate runs.
3. Verify deterministic replays.

### Phase 3 (Week 3): Diff + Eval + Gate
1. Implement behavior and policy diffing.
2. Add output-quality and efficiency scoring.
3. Emit report artifacts and CI exit codes.

### Phase 4 (Week 4): Hardening
1. Load/perf validation.
2. Retention and cleanup.
3. Rollout documentation and operator playbook.

## Explicit Non-Goals (MVP)
1. Full dashboard UI.
2. Minimal repro shrinker.
3. Full-Freeze mode.
4. Historical policy backtesting.

## Risks and Mitigations
1. Replay data gaps from OTEL-only.
   - Mitigation: replay store as source of truth.
2. Sensitive payload handling.
   - Mitigation: redaction profiles + retention TTL.
3. Capture path pressure.
   - Mitigation: bounded queue, priority drop strategy, telemetry.
4. Tool nondeterminism.
   - Mitigation: strict Freeze-Tools mode for migration decisions.

## Demo Narrative
1. Baseline refund workflow is captured once.
2. Candidate A is cheaper but diverges at step N and fails safety gate.
3. Candidate B passes behavior + safety thresholds and reduces cost.
4. CI blocks A and allows B with auditable reports.
