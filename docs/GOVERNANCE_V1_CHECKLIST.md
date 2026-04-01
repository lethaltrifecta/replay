# Governance V1 Checklist

This checklist applies the governance-first design to the current OpenAPI contract in [api/openapi.yaml](../api/openapi.yaml).

Use this document as the filter for further work on the branch.

## Classification Rules

- `core v1`: directly supports the approval workflow and should remain a first-class contract.
- `supporting evidence`: useful to explain a decision, but should not drive the overall product shape.
- `unnecessary for now`: not part of the governance product center; avoid investing beyond basic correctness.

## Approval Workflow

The V1 operator journey is:

1. find or select the right baseline
2. inspect candidate traces and drift results
3. review a gate verdict
4. inspect first divergence and risk escalation
5. compare baseline vs candidate side by side
6. decide whether the change is safe to approve

If an endpoint or schema does not materially support one of those steps, it is not primary.

## Endpoint Checklist

| Endpoint | Operation | Class | Decision | Next Action |
|---|---|---|---|---|
| `GET /baselines` | `listBaselines` | `core v1` | Keep | Use as the baseline selection surface. |
| `POST /baselines/{traceId}` | `createBaseline` | `core v1` | Keep | Required for approval workflow. |
| `DELETE /baselines/{traceId}` | `deleteBaseline` | `core v1` | Keep | Required to manage the approved set. |
| `GET /drift-results` | `listDriftResults` | `core v1` | Keep | Treat as the drift inbox. |
| `GET /drift-results/{traceId}` | `getDriftResult` | `core v1` | Keep | Keep pair-aware semantics; ambiguity handling is correct for review workflows. |
| `GET /experiments` | `listExperiments` | `core v1` | Keep | Treat as the gate run inbox. |
| `GET /experiments/{id}` | `getExperiment` | `supporting evidence` | Keep, but subordinate | Do not make this richer than the report unless UI proves it needs it. |
| `GET /experiments/{id}/report` | `getExperimentReport` | `core v1` | Keep | This is the primary gate review object. |
| `GET /traces` | `listTraces` | `core v1` | Keep | Needed to choose baselines/candidates. |
| `GET /traces/{traceId}` | `getTrace` | `supporting evidence` | Keep | Preserve fidelity, but do not let this endpoint drive product scope. |
| `GET /compare/{baselineTraceId}/{candidateTraceId}` | `compareTraces` | `core v1` | Keep | Core review workflow. |
| `POST /gate/check` | `createGateCheck` | `core v1` | Keep | Required to trigger a governance decision. |
| `GET /gate/status/{id}` | `getGateStatus` | `supporting evidence` | Keep | Needed for polling, but not a primary review object. |
| `GET /health` | `getHealth` | `unnecessary for now` | Keep as infrastructure only | Do not spend governance-design time here. |

## Schema Checklist

| Schema | Class | Decision | Next Action |
|---|---|---|---|
| `Error` | `supporting evidence` | Keep | Fine as generic infrastructure. |
| `GateCheckRequest` | `core v1` | Keep | Maintain as minimal gate trigger input only. |
| `GateCheckResponse` | `core v1` | Keep | Fine as minimal async kickoff response. |
| `GateStatusResponse` | `supporting evidence` | Keep | Polling-only; keep thin. |
| `Baseline` | `core v1` | Keep | First-class governance object. |
| `DriftResult` | `core v1` | Keep | First-class governance object. |
| `DriftDetails` | `core v1` | Keep | Compact decision summary; keep typed. |
| `VariantConfig` | `core v1` | Keep | Relevant to approval context; do not broaden casually. |
| `ReplayRequestHeaders` | `supporting evidence` | Keep | Explicit and constrained is correct; avoid turning this back into a generic map. |
| `Experiment` | `core v1` | Keep | Good gate-run list item. |
| `ExperimentDetail` | `supporting evidence` | Keep | Useful, but secondary to the report. |
| `ExperimentRun` | `supporting evidence` | Keep | Keep lean; add fields only if they improve review. |
| `ExperimentReport` | `core v1` | Keep | Primary gate review object. |
| `AnalysisResult` | `core v1` | Keep | Good typed review summary container. |
| `BehaviorDiff` | `core v1` | Keep | Summary object; do not overcomplicate yet. |
| `FirstDivergence` | `core v1` | Keep | One of the most important governance objects. |
| `SafetyDiff` | `core v1` | Keep | One of the most important governance objects. |
| `TraceSummary` | `core v1` | Keep | Needed to select evidence. |
| `PromptMessage` | `supporting evidence` | Keep loose | Preserve fidelity; do not over-tighten. |
| `PromptContent` | `supporting evidence` | Keep loose | Preserve fidelity; do not over-tighten. |
| `TraceStep` | `supporting evidence` | Keep | Useful evidence envelope. |
| `TraceDetail` | `supporting evidence` | Keep | Evidence surface only; avoid product bloat. |
| `ToolCapture` | `supporting evidence` | Keep | Good now that it carries `stepIndex` and `error`; keep focused on review value. |
| `TraceComparison` | `core v1` | Keep | Core approval surface. |

## Current Direction Checks

### Keep Doing

- strongly typing governance objects such as `Baseline`, `DriftResult`, `ExperimentReport`, `FirstDivergence`, and `SafetyDiff`
- preserving prompt and tool payloads without lossy normalization
- using OpenAPI-generated clients to stabilize the review contract
- designing around baseline approval, drift review, and gate verdict review

### Stop Doing

- expanding the API because the backend happens to store more data
- turning `replay` into a generic observability or trace-explorer product
- over-modeling provider-specific evidence payloads
- spending contract effort on non-governance infrastructure surfaces

## Immediate Implementation Priorities

1. Keep `ExperimentReport`, `DriftResult`, and `TraceComparison` as the center of the UI contract.
2. Add governance-flow contract tests that exercise:
   - baseline listing and selection
   - drift inbox and drift detail
   - gate kickoff and canonical experiment report
   - baseline vs candidate comparison
   Status:
   completed via [`TestGovernanceWorkflowContract`](../pkg/api/server_test.go), [`TestGovernanceWorkflowContract_FailVerdict`](../pkg/api/server_test.go), and [`FuzzGetTracePreservesPromptToolFields`](../pkg/api/fuzz_test.go).
3. Avoid broadening `TraceDetail` unless the UI can prove a review need.
4. Treat `GET /experiments/{id}/report` as the canonical review surface and avoid reintroducing duplicate report routes.
5. Treat any future schema work on prompt/tool payloads as evidence-preservation work, not domain modeling.

## Test Notes

The governance-first contract is currently documented by three high-signal tests:

- [`TestGovernanceWorkflowContract`](../pkg/api/server_test.go): proves the happy-path operator flow across baseline selection, drift review, compare, gate kickoff, polling, and canonical experiment report retrieval.
- [`TestGovernanceWorkflowContract_FailVerdict`](../pkg/api/server_test.go): proves the rejection-path semantics that matter to the UI: `status=completed`, `verdict=fail`, and typed first-divergence evidence.
- [`FuzzGetTracePreservesPromptToolFields`](../pkg/api/fuzz_test.go): protects prompt/tool evidence fidelity so flexible provider payloads are not silently narrowed again.

This is the minimum contract test bar for future schema changes. If a contract change invalidates one of these tests, the branch should treat that as a product-level review event, not just a test fix.

## Decision Filter

Before changing the contract, ask:

- Does this help an operator approve or reject a change?
- Does this make drift or gate outcomes easier to understand?
- Is this a governance concept or just stored evidence?
- Are we preserving truth, or are we forcing a cleaner but less faithful shape?

If the answer is “this mostly mirrors storage” or “this looks like a generic tracing API,” it is probably the wrong direction.
