# Governance V1 Plan

## Goal

Keep CMDR focused on its actual purpose: helping an operator decide whether an agent change is safe to approve.

The question to optimize for is:

> Can I safely approve this change?

This plan treats governance and review workflows as the product center, and treats lower-level trace payloads as supporting evidence.

## Plan

1. Define the V1 governance workflow.

   Write down the exact operator journey CMDR must support:
   baseline selection, drift review, gate verdict review, side-by-side comparison, approval decision.

   If a schema or endpoint does not support that flow, it is not primary.

2. Freeze the core V1 contract.

   Treat these as the only first-class UI/domain objects for now:

   - `Baseline`
   - `DriftResult`
   - `ExperimentReport`
   - `TraceComparison`
   - `FirstDivergence`
   - `SafetyDiff`
   - `TraceSummary`

   Everything else is either supporting evidence or internal plumbing.

3. Classify every endpoint and schema.

   For each route/object, mark it as:

   - `core v1`
   - `supporting evidence`
   - `unnecessary for now`

   This gives us a hard filter for further work.

4. Tighten only decision-critical summaries.

   Improve the fields that help answer “can I approve this change?”:
   clear verdicts, reason text, risk escalation, first divergence, baseline/candidate identity, experiment/run status.

   Avoid polishing low-value internals.

5. Keep evidence payloads high-fidelity but loose.

   Prompt/tool/tool-choice payloads should be preserved accurately, but not over-modeled.

   They are evidence for review, not the center of the product contract.

6. Fix contract mismatches that distort operator meaning.

   Anything that silently drops data, hides ambiguity, or exposes storage-shaped semantics should be corrected.

   The API should reflect review semantics, not DB convenience.

7. Add governance-flow contract tests.

   Cover the real path:
   ingest or seed baseline, compare candidate, inspect drift result, run gate, inspect the canonical experiment report, inspect comparison evidence.

   Assert JSON shape and meaning, not just status codes.

   Status:
   completed in [`pkg/api/server_test.go`](../pkg/api/server_test.go) and [`pkg/api/fuzz_test.go`](../pkg/api/fuzz_test.go).

   Current contract coverage:
   - `TestGovernanceWorkflowContract`: operator happy path from baseline selection through canonical experiment report review
   - `TestGovernanceWorkflowContract_FailVerdict`: governance rejection path where replay completes but the verdict is `fail`
   - `FuzzGetTracePreservesPromptToolFields`: prompt/tool evidence fidelity across the `GET /traces/{id}` surface

8. Defer non-governance expansion.

   Do not spend more time broadening the API into a generic observability surface.

   If it does not improve baseline approval or rollout safety review, push it out of scope.

9. Produce a short V1 contract document.

   One doc that names the core objects, endpoints, field meanings, and intended UI usage.

   This becomes the decision reference for the rest of the branch.

10. Re-review the branch against this plan.

   After the contract is documented, re-check existing changes and trim or rewrite anything that violates the governance-first direction.

## Working Rule

Use this filter for future changes:

- If a field helps an approval or rejection decision, promote it.
- If a field exists only because it exists in storage, demote it.
- If a payload is provider-specific evidence, preserve it loosely.
- If an object is a governance concept, type it hard.
