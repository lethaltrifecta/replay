# CMDR Draft Implementation Plan

## Goal
Build a production-usable **model migration gate** for agents that replays real runs against candidate models and blocks unsafe regressions.

## Scope Decision
- Use a **hybrid architecture**:
- OTEL for observability, trace lookup, and debugging.
- Replay store for deterministic execution and CI-grade diffing.
- Prioritize **Freeze-Tools replay** first.
- Defer UI and advanced tooling until the gate is reliable.

## MVP Requirements
1. Zero SDK changes for agent apps.
2. Deterministic replay mode where tool outputs are pinned to baseline.
3. Baseline vs candidate diff for:
- tool graph and call order
- tool argument drift
- policy decision drift
- final output similarity
- latency and token/cost deltas
4. CI gate output:
- machine-readable JSON
- human-readable Markdown
- non-zero exit on failure
5. Safety hard-fails:
- new destructive tool calls
- missing required tool calls
- unknown tool calls in strict replay mode
6. Capture overhead target: p95 added latency under 5%.

## Minimal Gateway Changes
1. Replay-grade capture sink with feature flag.
2. Canonical `step_index` per run.
3. Per-run override support for:
- candidate model
- candidate policy bundle
- replay tool endpoint
4. Stable run correlation IDs:
- `run_id`
- `baseline_run_id`
- `trace_id`

## Data Model (v1)
### runs
- `run_id`, `baseline_run_id`, `trace_id`
- `variant_id`, `app_id`, `tenant_id`
- `start_ts`, `end_ts`, `status`
- `schema_version`

### steps
- `run_id`, `step_index` (canonical ordering)
- `step_type` (`llm`, `tool`, `policy`, `io`)
- `ts_start`, `ts_end`

### llm_calls
- `run_id`, `step_index`
- `provider`, `model`, `params`
- request payload ref, response payload ref
- token and latency fields

### tool_calls
- `run_id`, `step_index`
- `tool_name`, `args_ref`, `result_ref`
- `latency_ms`, `error`
- `risk_class` (`read`, `write`, `destructive`)

### policy_events
- `run_id`, `step_index`
- `policy_name`, `decision` (`allow`, `deny`, `transform`)
- reason ref and transformed payload refs

## Components
1. **Capture sink (gateway)**: writes structured replay events.
2. **Replay store**: SQLite first, pluggable later.
3. **Freeze-Tools mock MCP server**: serves recorded tool responses.
4. **Replay orchestrator CLI**:
- `replay run --baseline ... --variant ...`
5. **Diff/gate engine CLI**:
- `replay diff --baseline ... --candidate ...`
- `replay gate --baseline ... --candidate ...`

## Milestones
### Phase 1: Capture Foundation (Week 1)
- Implement schema and sink.
- Add redaction hooks before persistence.
- Verify end-to-end capture for multi-step runs.

### Phase 2: Deterministic Replay (Week 2)
- Implement Freeze-Tools mock MCP server.
- Add strict mode mismatch handling.
- Replay baseline successfully against candidate model.

### Phase 3: Diff + CI Gate (Week 3)
- Tool sequence and argument drift.
- Safety hard-fail checks.
- Output similarity + latency/cost deltas.
- JSON + Markdown report + exit code.

### Phase 4: Hardening (Week 4)
- Load/perf validation.
- Data retention and cleanup.
- Docs and rollout playbook.

## Gate Defaults
- Fail if candidate introduces any new `destructive` tool call.
- Fail if candidate misses required workflow tools.
- Fail if strict replay sees unknown tool calls.
- Warn/fail based on configured thresholds for:
- tool sequence similarity
- argument drift
- output similarity
- latency/cost budget

## Explicit Non-Goals (MVP)
- Full web dashboard.
- Minimal repro shrinker.
- Full-Freeze replay mode.
- Advanced policy backtesting engine.

## Risks and Mitigations
1. OTEL payload incompleteness for replay.
- Mitigation: replay store is source of truth.
2. Sensitive payload leakage.
- Mitigation: field-level redaction, retention TTL.
3. Capture overhead under load.
- Mitigation: async bounded queue, drop payload blobs before metadata.
4. Tool nondeterminism.
- Mitigation: Freeze-Tools strict mode for migration gating.

## Demo Story
1. Replay a real baseline workflow.
2. Candidate A fails due to risky tool-graph divergence.
3. Candidate B passes with lower cost and acceptable latency.
4. CI gate blocks A, allows B.
