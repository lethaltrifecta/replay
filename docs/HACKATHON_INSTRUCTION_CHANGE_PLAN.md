# Hackathon Instruction-Change Plan

This note captures the smallest shippable version of the idea we discussed:

> CMDR should tell a story about governing behavior-affecting changes, not just model swaps.

For hackathon scope, the concrete proof is:

> Same model. Same tools. Different instructions. CMDR caught it.

## Product Reframe

The stronger story is not "we compare models."

The stronger story is:

- We govern any change that can alter agent behavior
- That includes model changes, prompt changes, role files, policy files, and tool configuration

This is more useful in practice because teams change prompt and instruction files more often than they swap foundation models.

## What Exists Today

The repo already has the right review flow:

- `Launchpad` for choosing baseline and candidate traces
- `Divergence` for verdict-first review
- `Shadow Replay` for raw side-by-side evidence
- `Gauntlet` for canonical experiment reports

The current gap is not the overall flow. The gap is that the system does not yet make the changed instruction bundle explicit in the UI story.

## Smallest Credible Ship

Do not build:

- a generic runner platform
- repo checkout automation
- broad OTLP provenance plumbing
- new UI routes
- experiment-config modeling for change context

Do ship:

- one concrete instruction-file change type
- one concrete demo scenario
- one clean UI callout that says what changed

## Recommended Scope

### Slice 1: Demo Story

Update the seeded demo so the divergence is clearly caused by instruction changes instead of looking arbitrary.

Specifically:

- keep the same model on baseline and candidate traces
- change the system prompt between baseline and candidate
- frame that change as a `role.md` update
- store a `change_context` object in `ReplayTrace.Metadata`

Suggested metadata shape:

```json
{
  "change_context": {
    "kind": "instruction_file",
    "target": "role.md",
    "baseline_label": "safe (v1.2)",
    "candidate_label": "aggressive (v1.3)",
    "summary": "Changed agent role to prioritize cleanup over safety"
  }
}
```

Suggested narrative:

- baseline: cautious role instructions
- candidate: aggressive cleanup instructions
- same task
- same model
- candidate takes a riskier tool action such as `delete_database`

### Slice 2: UI Surfacing

Use the existing screens. No new routes are needed.

Priority order:

1. `Divergence detail`
2. `Gauntlet report`
3. `Shadow Replay`

What to add:

- a small "What Changed" callout
- target file, baseline label, candidate label, and summary
- in `Shadow Replay`, small passive chips are enough

The money shot is one screen that shows:

- FAIL verdict
- `What Changed: role.md`
- `safe (v1.2) -> aggressive (v1.3)`

### Slice 3: Demo Script And Assets

Write the demo backwards from the screenshots:

1. Divergence screen shows FAIL
2. "What Changed" shows `role.md` safe -> aggressive
3. Shadow Replay shows changed behavior and riskier tool use
4. One-line takeaway:
   `Same model. Same tools. Different instructions. CMDR caught it.`

## One Minimal Technical Correction

We originally tried to cut all contract work. That is almost right, but one small API exposure is still worth doing.

Today, the trace/compare responses used by the UI do not cleanly expose replay metadata. The smallest clean fix is:

- add optional metadata exposure on trace steps, or
- add a small trace-level `changeContext` derived from the first step

This is intentionally much smaller than redesigning the API. It is only enough to let the UI read the seeded `change_context` object without hacks.

If time gets extremely tight, the emergency fallback is to infer the change from the system prompt text instead of exposing metadata, but that should be treated as a last resort.

## What To Cut

For the hackathon version, explicitly defer:

- OTLP parser support for `cmdr.change.*` attributes
- new `gate` CLI fields
- generic OpenClaw or external-agent orchestration
- broader schema or repository refactors
- experiment-level change-context modeling

These are good follow-ons, but they are not needed to prove the idea.

## Effort Estimate

Target the following:

| Slice | Effort |
|---|---:|
| Demo seed and story change | 2-3 hours |
| Minimal API exposure + UI surfacing | 2-3 hours |
| Demo script, screenshots, short writeup | 2-3 hours |
| Total | about 1 day |

## Cut Line

If time gets tight, the minimum credible version is:

- Slice 1
- `Divergence detail` only from Slice 2
- demo script / screenshots

One screen showing a FAIL verdict next to `What Changed: role.md` is enough to prove the concept.

## After The Hackathon

If there is time later, the next step is to make instruction changes more first-class beyond the seeded demo:

- accept provenance from real traces
- optionally carry change context through gate runs
- add one concrete adapter for a real external agent such as OpenClaw

But those are phase-two improvements. They are not required for the hackathon story to land.
