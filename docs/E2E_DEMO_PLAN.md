# Phase 4: Hackathon Implementation Roadmap

## Goal

Ship one credible, end-to-end governance demo for the `Secure & Govern MCP` track:

1. A real agent run goes through `agentgateway`
2. CMDR captures the run from OTLP and stores replay + tool data
3. CMDR replays the same scenario with frozen MCP tool responses via `freeze-mcp`
4. CMDR blocks an unsafe deployment and explains the first divergence

This document replaces the earlier exploratory plan with a build roadmap aligned to the official scoring on the submissions page as of March 7, 2026:

- Incorporation of Open Source Projects: 40
- Usefulness: 20
- Product Readiness: 20
- Launch Bucket: 20

Reference: <https://aihackathon.dev/submissions/>

## Status Snapshot

Completed on the current branch:

- real `agentgateway` capture into CMDR
- full-loop replay proof with `freeze-mcp`
- CMDR-backed baseline capture for frozen replay
- MCP replay routed through `agentgateway`, not just AI traffic
- a purpose-built database migration demo agent with safe and unsafe replay paths
- a runnable verification harness in `make test-migration-demo-full-loop`
- a native CMDR verdict command for the migration demo traces
- a first-class `cmdr demo migration run` entrypoint with saved report artifacts
- launch-oriented artifacts (`judge-highlight.md`, `demo-script.md`) emitted on each full run

Current next step:

- tighten the final launch collateral and decide how much of this demo path should graduate into the broader gate surface

## Winning Thesis

CMDR should not present itself as a generic eval tool. The winning story is:

> "CMDR is the missing deployment safety layer for MCP agents. It captures real agent behavior from agentgateway, freezes the tool environment with freeze-mcp, replays the same scenario with a candidate model or prompt, and blocks unsafe rollout when tool behavior or risk profile changes."

That framing maps directly to the official `Secure & Govern MCP` track:

- `secure`: catch risky tool escalation before deploy
- `monitor`: detect behavioral drift in production traces
- `manage`: enforce pass/fail deployment gates using agentgateway telemetry

## Scoring Strategy

| Bucket | What judges need to see | What we must build |
|---|---|---|
| Open Source Projects (40) | Real, deep use of `agentgateway` in the main flow | agentgateway must be in the primary capture + replay demo, not just mentioned |
| Usefulness (20) | A practical problem teams actually have | show a realistic agent change that could cause production damage |
| Product Readiness (20) | One-command setup, predictable demo, clear docs, saved reports | runnable stack, smoke test, clear CLI output, deterministic enough for video |
| Launch (20) | Good blog post, demo video, repo, screenshots, explanation | plan launch assets as a workstream, not an afterthought |

## Scope Decisions

### Must ship

- One primary end-to-end demo path with full components
- `agentgateway` in the critical path for both capture and replay
- `freeze-mcp` in the critical path for replayed tool calls
- One safe pass case and one unsafe fail case
- First-divergence explanation in CLI output and saved report
- Quickstart, smoke script, and launch assets

### Nice to have

- Second provider route (Anthropic) after OpenAI path is stable
- Remote async gate API for video polish
- HTML or Markdown report rendering

### Out of scope for the hackathon submission

- Generic evaluator platform features (`eval`, `ground-truth`, human review queue)
- Broad multi-track integrations just to mention more projects
- General-purpose UI
- Scale work beyond the demo path

## Demo We Are Shipping

### Primary demo

One agent scenario, one baseline, two candidate runs:

1. **Baseline capture**
   - A sample MCP-enabled agent performs a cloud-native ops task through `agentgateway`
   - Real tool calls execute against real MCP tools
   - CMDR ingests OTLP traces and stores replay steps plus tool captures

2. **Safe gate**
   - CMDR replays the exact scenario through `agentgateway`
   - `freeze-mcp` returns recorded tool responses for the baseline environment
   - Candidate model or prompt behaves acceptably
   - Gate returns `PASS`

3. **Unsafe gate**
   - Same frozen environment
   - Candidate model or prompt escalates to a higher-risk tool choice
   - Gate returns `FAIL`
   - Output highlights first divergence and risk escalation

### Fallback demo

Keep the current deterministic `cmdr demo` flow as a private backup until the full-stack video is recorded. Do not use it as the main submission story.

## Recommended Scenario

Replace the current auth-refactor scenario with a cloud-native operations scenario that is more aligned to the judges and track.

### Preferred scenario

`Database migration governance`

Safe baseline:

- `inspect_schema`
- `check_backup`
- `create_backup`
- `run_migration`

Unsafe candidate:

- skips the backup path
- attempts `drop_table`
- gets blocked by frozen replay because the dangerous call was never part of the approved baseline

### Why this scenario

- clearly fits "secure, monitor, manage AI agent deployments"
- makes risk classes intuitive to judges
- demonstrates why frozen tools matter
- produces a compact demo that is easier to record reliably than a full Kubernetes control loop

### Scenario contract

- The task must be ambiguous enough that model or prompt differences matter
- The toolset must include both safe and dangerous operations
- The failure case must be reproducible enough to record twice
- The safe case must pass with the same frozen environment

## Target Architecture

```
                    +---------------------------+
                    | Sample Agent Driver       |
                    | - OpenAI-compatible API   |
                    | - MCP tool use            |
                    | - run_id / scenario_id    |
                    +-------------+-------------+
                                  |
                                  v
                    +---------------------------+
                    | agentgateway              |
                    | - provider routing        |
                    | - MCP/tool routing        |
                    | - OTLP export             |
                    +------+------+-------------+
                           |      |
                           |      +----------------------+
                           |                             |
                           v                             v
                +-------------------+         +-------------------+
                | LLM provider       |         | MCP tools         |
                | OpenAI first       |         | baseline or       |
                | Anthropic optional |         | freeze-mcp        |
                +-------------------+         +-------------------+

                           OTLP
                            |
                            v
                    +---------------------------+
                    | CMDR                      |
                    | - OTLP receiver           |
                    | - replay trace store      |
                    | - tool capture store      |
                    | - drift + gate diff       |
                    | - first divergence report |
                    +-------------+-------------+
                                  |
                                  v
                         +----------------+
                         | PostgreSQL     |
                         +----------------+
```

## Execution Phases

## Phase 0: Validate Reality Before Building More

Objective: confirm the external contracts we are depending on.

### Tasks

1. Validate agentgateway OTEL output against CMDR parser expectations
2. Validate agentgateway request/response shape for tool-calling through its OpenAI-compatible API
3. Validate how the sample agent will pass a stable `run_id` or scenario identifier
4. Validate what `freeze-mcp` currently supports:
   - capture mode
   - freeze mode
   - session binding or baseline selection
   - stdio vs SSE transport

### Deliverables

- one mapping doc of actual `agentgateway` span attributes to CMDR parser fields
- one captured raw trace that proves CMDR can ingest the real shape
- one decision note on how replay will bind to a baseline in `freeze-mcp`

### Acceptance criteria

- a single real call through `agentgateway` appears in `replay_traces`
- tool events, if emitted, appear in `tool_captures`
- we know whether parser changes are required before continuing

## Phase 1: Build the Real Baseline Capture Path

Objective: capture one real, tool-using agent run and turn it into a baseline.

### CMDR changes

1. Adapt the OTEL parser to the actual `agentgateway` attribute format if needed
2. Persist correlation metadata needed for demo lookup:
   - `run_id`
   - scenario name
   - provider/model
   - tool call metadata
3. Ensure tool captures are stored with enough information to replay deterministically

### Sample agent changes

1. Build a small driver script for the chosen scenario
2. Use real tool-calling through `agentgateway`
3. Attach a stable run identifier to every request
4. Keep the script short enough to be inspectable on video

### Acceptance criteria

- baseline run can be generated from a script, not by hand
- baseline trace can be looked up deterministically
- `cmdr drift baseline set <trace-id>` works on a real captured trace

## Phase 2: Make Replay Tool-Aware and Freeze-Aware

Objective: turn the current prompt replay into actual MCP-governed replay.

### Current gap

Today `gate check` replays `messages` only. It does not reconstruct tool definitions or force tools through `freeze-mcp`. That is the main limitation preventing the strongest submission story.

### CMDR changes

1. Extend `pkg/agwclient/client.go`
   - add request support for tool definitions
   - add optional request headers or metadata used for replay correlation
   - preserve tool call responses exactly enough for diffing

2. Extend `pkg/replay/engine.go`
   - reconstruct a replay request from the captured baseline, not only its messages
   - pass tool definitions when available
   - support a "frozen tools" replay mode
   - preserve per-step metadata needed for first-divergence reporting

3. Extend the OTEL/storage model if needed
   - store enough baseline data to rebuild tool-aware requests
   - if tool schemas are not present in current traces, store them in `metadata`

### freeze-mcp integration changes

1. Define an explicit baseline selection contract
   - do not rely on "latest matching tool call"
   - all frozen lookups must be scoped to the intended baseline trace or replay session

2. Add or use a deterministic lookup key
   - preferred: `baseline_trace_id + call_ordinal`
   - acceptable fallback: `baseline_trace_id + tool_name + args_hash`

3. Ensure the replay path can switch tools from live mode to frozen mode without changing the sample agent logic

### Acceptance criteria

- replayed runs go through `agentgateway`
- replayed tool calls hit `freeze-mcp`
- `freeze-mcp` returns the baseline-recorded result for the selected baseline
- the replay result is deterministic enough to record a demo

## Phase 3: Strengthen the Diff and Governance Story

Objective: make the output feel like governance tooling rather than a generic score dump.

### Changes

1. Print first divergence in `cmdr gate check`
   - show step index or tool index
   - show baseline action vs candidate action
   - explain why it matters

2. Print risk escalation clearly
   - baseline risk profile
   - candidate risk profile
   - the exact tool causing escalation

3. Tighten `cmdr gate report`
   - render the saved analysis cleanly
   - include the frozen baseline trace ID
   - include tool/risk/response dimensions

4. Consider a compact Markdown report artifact
   - useful for the blog post
   - useful for screenshots

### Acceptance criteria

- a judge can understand the failure in under 10 seconds
- the unsafe run shows more than just a low similarity score
- the output includes first divergence and risk escalation

## Phase 4: Product Readiness and Reproducibility

Objective: turn the build into a judge-safe submission.

### Required work

1. One-command local stack bring-up
   - `postgres`
   - CMDR
   - `agentgateway`
   - `freeze-mcp`
   - sample agent config

2. One smoke script
   - baseline capture
   - baseline mark
   - safe gate
   - unsafe gate

3. One short quickstart doc
   - exact env vars
   - exact startup commands
   - expected output checkpoints

4. Health and debugging
   - health endpoints where possible
   - readable startup failures
   - SQL snippets for verifying captured traces and tool rows

5. End-to-end test coverage for the demo path
   - script-based is acceptable if full automation is too expensive
   - target one "capture to fail" test and one "capture to pass" test

### Acceptance criteria

- a teammate can run the happy path from docs without tribal knowledge
- the demo can be rerun after a DB reset
- failure states are actionable

## Phase 5: Launch Assets

Objective: collect the 20-point launch bucket while the engineering work is still fresh.

### Required assets

1. Blog post
   - clear project description
   - "why this matters" paragraph
   - one judge highlight box
   - architecture diagram
   - demo video embed
   - GitHub link

2. Demo video
   - 3-4 minutes
   - baseline capture
   - safe gate
   - unsafe gate
   - explanation of first divergence and frozen tools

3. Submission screenshots
   - captured baseline trace
   - `freeze-mcp` replay in action
   - failed gate output
   - passed gate output

4. Social thread
   - short, technical, screenshot-heavy
   - links back to blog and repo

### Acceptance criteria

- all submission assets can be generated from the shipping branch
- screenshots and narration match the actual implementation

## Concrete Workstreams by Repository

## CMDR Repository

### Telemetry and storage

- `pkg/otelreceiver/parser.go`
  - adapt to actual agentgateway OTEL attributes
  - preserve replay-relevant metadata
  - capture correlation IDs and tool payloads needed for replay

- `pkg/storage/postgres.go`
  - add baseline-scoped frozen lookup method
  - avoid global "latest row wins" semantics for freeze replay
  - store any new metadata required for tool-aware replay

### Replay and gate

- `pkg/agwclient/client.go`
  - add `tools` request support
  - add optional replay headers/metadata
  - preserve tool call response structure

- `pkg/replay/engine.go`
  - rebuild tool-aware requests
  - add frozen replay mode
  - keep current prompt-only path as fallback only

- `pkg/diff/diff.go`
  - keep `FirstDivergence`
  - make its output specific enough for CLI rendering

- `cmd/cmdr/commands/gate.go`
  - render first divergence
  - render risk escalation details
  - optionally add a flag for frozen replay mode if configuration cannot fully infer it

### Demo and docs

- `scripts/`
  - add full-stack smoke script
  - keep deterministic fallback script separate

- `docs/`
  - add quickstart for the full-stack demo
  - add troubleshooting for agentgateway and freeze-mcp integration

## freeze-mcp Repository

This repo is not present locally in this workspace, so the exact file list must be validated there. The required behavior is:

1. capture mode can proxy to real MCP tools and record results
2. freeze mode can serve recorded results for a selected baseline
3. replay session can bind to one baseline trace deterministically
4. logs make it obvious which frozen result was returned

If the current implementation differs, adapt the roadmap but preserve the contract above.

## Suggested File-Level Changes in This PR Series

### PR A: Telemetry contract and baseline capture

- parser compatibility changes
- sample agent script
- docs proving real capture path

### PR B: Tool-aware replay through agentgateway

- `agwclient` request model updates
- replay engine support for tools
- baseline trace metadata persistence

### PR C: freeze-mcp baseline-scoped replay

- CMDR storage support for scoped lookups
- freeze-mcp integration wiring
- end-to-end smoke path

### PR D: Diff/report polish and launch assets

- first divergence output
- screenshots
- blog post draft
- final demo script

## Milestone Plan

## Milestone 1: Real capture working

Exit when:

- a real agent run is visible in CMDR
- baseline trace and tool captures are queryable
- no manual DB surgery is required

## Milestone 2: Frozen replay working

Exit when:

- the replay path goes through `agentgateway`
- tool calls route to `freeze-mcp`
- the correct baseline tool results are returned

## Milestone 3: Safe pass + unsafe fail recorded

Exit when:

- one candidate passes
- one candidate fails
- the failure reason is visible from CLI output alone

## Milestone 4: Submission-ready

Exit when:

- quickstart is accurate
- smoke script is stable
- demo video and blog post are drafted

## Risks and Mitigations

### Risk 1: agentgateway OTEL format does not match CMDR parser

Mitigation:

- treat Phase 0 as a blocker
- adapt parser additively
- do not build more replay logic until real capture is proven

### Risk 2: tool-aware replay through agentgateway is harder than expected

Mitigation:

- add tool request support first with a minimal sample agent
- validate one tool call end-to-end before full scenario work
- keep prompt-only replay as fallback but not as the submission headline

### Risk 3: freeze-mcp selection is nondeterministic

Mitigation:

- require explicit baseline scoping
- avoid global "latest tool capture" semantics
- add logs and verification queries

### Risk 4: live model behavior is too noisy for the fail case

Mitigation:

- use model-or-prompt rollout as the unit of change, not model-only if needed
- pick a scenario with strong behavioral separation
- record the demo once stable rather than relying on a live cold run

### Risk 5: scope drifts back into "generic eval platform"

Mitigation:

- every new task must strengthen the primary demo path or the official scoring buckets
- defer platform features that judges will not see

## Definition of Done

The submission is done when all of the following are true:

1. `agentgateway` is in the primary demo path
2. `freeze-mcp` is in the replay path, not a side demo
3. CMDR captures a real baseline and gates a real candidate run
4. The unsafe run fails because of tool or risk divergence, not because the stack is broken
5. The CLI explains the first divergence clearly
6. A teammate can reproduce the happy path from docs
7. The blog post, video, and screenshots all reflect the real implementation

## Immediate Next Steps

1. Run Phase 0 and capture the actual `agentgateway` OTEL shape
2. Decide the replay baseline binding contract for `freeze-mcp`
3. Replace the demo scenario with the cloud-native ops scenario
4. Start PR A with telemetry and sample-agent work only
