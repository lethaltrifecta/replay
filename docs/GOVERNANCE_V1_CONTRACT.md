# Governance V1 Contract

This document is the human-readable contract for CMDR Phase 1 governance review.

Use it as the semantic reference for:

- backend changes
- UI work
- contract tests
- PR review

The question this contract serves is:

> Can I safely approve this agent change?

## Product Center

CMDR is a governance and review product, not a generic trace browser.

The contract should optimize for:

1. baseline selection
2. drift review
3. gate verdict review
4. first-divergence and risk-escalation review
5. baseline-vs-candidate comparison
6. approval or rejection decision

## Canonical Endpoints

These are the first-class V1 endpoints:

- `GET /baselines`
- `POST /baselines/{traceId}`
- `DELETE /baselines/{traceId}`
- `GET /traces`
- `GET /drift-results`
- `GET /drift-results/{traceId}`
- `POST /gate/check`
- `GET /experiments`
- `GET /experiments/{id}/report`
- `GET /compare/{baselineTraceId}/{candidateTraceId}`

Supporting evidence endpoints:

- `GET /traces/{traceId}`
- `GET /experiments/{id}`
- `GET /gate/status/{id}`

Canonical rule:

- `GET /experiments/{id}/report` is the review object.
- Do not reintroduce duplicate report routes.

## Core Objects

These are the core governance-domain objects:

- `Baseline`
- `DriftResult`
- `DriftDetails`
- `Experiment`
- `ExperimentReport`
- `AnalysisResult`
- `FirstDivergence`
- `SafetyDiff`
- `TraceComparison`
- `TraceSummary`

These are supporting evidence objects:

- `TraceDetail`
- `TraceStep`
- `PromptContent`
- `PromptMessage`
- `ToolCapture`
- `ReplayRequestHeaders`
- `ExperimentDetail`
- `ExperimentRun`

Rule:

- Governance objects should stay strongly typed.
- Evidence payloads should stay faithful, even when loosely typed.

## Semantic Guarantees

### Review Status

The UI and tests should rely on this distinction:

- `status=completed` and `verdict=pass`
  The replay completed and CMDR approved the candidate behavior.

- `status=completed` and `verdict=fail`
  The replay completed and CMDR rejected the candidate behavior.
  This is a governance decision, not a system failure.

- `status=failed` and `error` is present
  The system could not complete evaluation successfully.
  This is an execution or pipeline failure, not a governance rejection.

### Drift Semantics

- `GET /drift-results` is the drift inbox.
- `GET /drift-results/{traceId}` is pair-sensitive.
- If multiple baselines exist for the same candidate trace, the caller must provide `baselineTraceId`.
- Ambiguous drift lookup is a contract error, not silent fallback behavior.

### Comparison Semantics

- `GET /compare/{baseline}/{candidate}` is the side-by-side review surface.
- It should expose baseline evidence, candidate evidence, and compact diff meaning.
- If no drift result exists for that pair, the comparison may still succeed, but drift-derived summary fields such as similarity may be absent.

### Evidence Fidelity

- Prompt/tool/tool-choice payloads are supporting evidence.
- They must be preserved without lossy normalization.
- They must not be narrowed just to make the schema look cleaner.

## Field Meanings That Must Not Drift

- `ExperimentReport.verdict`
  Governance outcome of the completed evaluation.

- `ExperimentReport.error`
  Operational failure detail only. This should not be used for normal governance rejection.

- `AnalysisResult.firstDivergence`
  Primary typed explanation of where behavior first diverged.

- `SafetyDiff.riskEscalation`
  Summary signal that the candidate crossed into a more dangerous risk profile.

- `TraceDetail`
  Supporting evidence only. It should not become the product center.

## Request Header Contract

Replay request headers are intentionally constrained.

The public contract is:

- `freezeTraceId`
- `freezeSpanId`
- `freezeStepIndex`

Rule:

- Do not expose replay headers as an arbitrary free-form map again.

## Test Bar

These tests currently define the minimum governance contract bar:

- [`TestGovernanceWorkflowContract`](../pkg/api/server_test.go)
- [`TestGovernanceWorkflowContract_FailVerdict`](../pkg/api/server_test.go)
- [`FuzzGetTracePreservesPromptToolFields`](../pkg/api/fuzz_test.go)

Interpretation:

- if one of these fails, treat it as a product-contract event
- do not “just update the test” without checking whether the governance semantics changed

## Change Filter

Before changing the contract, ask:

- Does this help an operator approve or reject a change?
- Does this make drift or gate results easier to understand?
- Is this a governance object or only supporting evidence?
- Are we preserving truth, or just making the schema prettier?

If the answer is “this mostly mirrors storage” or “this looks like a generic tracing API,” it is probably the wrong change.
