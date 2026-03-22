# Replay UI Findings And Plan

Last updated: 2026-03-21

## Executive Summary

The right move is to build `replay` as a standalone UI first, but design it as a portable domain that can later become the `Governance` section of a broader console spanning `kagent`, `agentregistry`, and `agentgateway`.

This should not be a generic AI observability dashboard. The strongest product surface in this repo is governance:

- baseline approval
- behavioral drift review
- deployment gate verdict review
- trace comparison
- tool-risk escalation inspection

The most idiomatic frontend stack for that direction is:

- `Next.js` App Router
- `React`
- `TypeScript`
- `Tailwind CSS`
- `Storybook`
- OpenAPI-generated TypeScript clients

The main architectural constraint is portability. The replay UI should be able to run:

- as its own standalone app
- later inside a shared console shell

That means the domain logic, components, and API client should be reusable and not tightly coupled to one app shell.

## What The Codebase Is Today

Based on the current repo, `replay` is a governance-oriented backend for agent behavior, not a generic observability platform.

Implemented core capabilities:

- OTLP ingestion and parsing into normalized trace storage
- replay step storage in `replay_traces`
- tool capture with deterministic argument hashing
- risk classification for tools
- drift fingerprinting and verdicting
- deployment gate replay with similarity scoring

Core references:

- `README.md`
- `pkg/otelreceiver/parser.go`
- `pkg/drift/fingerprint.go`
- `pkg/drift/compare.go`
- `pkg/diff/diff.go`
- `pkg/replay/engine.go`
- `pkg/api/handlers.go`
- `pkg/storage/models.go`
- `docs/GATE_REPLAY_ARCHITECTURE.md`

Important product reality:

- prompt-only replay is implemented today
- full deterministic agent-loop replay with `freeze-mcp` is the target architecture, not the default path yet
- eval / experiment / ground-truth surfaces exist, but parts of that surface are still placeholder-level

Implication for UI:

The UI should be shaped around what the backend truly supports now, not around a future full observability suite.

## Why A UI Makes Sense

The current product is not just a compute engine. It is a review and approval system.

The CLI is fine for running commands. It is weak for:

- choosing the right approved baseline
- reviewing why a drift check failed
- inspecting first divergence
- comparing candidate vs baseline behavior side by side
- understanding where risk escalated
- keeping an audit trail of approvals and rejected changes

That makes a UI useful, but only if the UI is centered on governance workflows.

## Product Positioning

The replay UI should be framed as a governance console, not a dashboard.

Primary objects:

- `Baseline`
- `Candidate Trace`
- `Drift Result`
- `Gate Run`
- `Experiment`
- `Tool Capture`
- `Risk Change`
- `First Divergence`

Primary user question:

> Can I safely approve this agent change?

That is the wedge. Competing directly with broad observability platforms on trace browsing, prompt playgrounds, and generic eval management would be a mistake at this stage.

## Findings From The Broader Platform Goal

The long-term goal is a master UI that combines:

- `kagent`
- `agentregistry`
- `agentgateway`
- `replay`

That goal should influence the architecture now, but it should not drive the first delivery scope.

### Recommended Product Mapping

- `Agents` -> `kagent`
- `Catalog` -> `agentregistry`
- `Connectivity` -> `agentgateway`
- `Governance` -> `replay`

This is the cleanest long-term mental model.

### Why Replay Should Stay Standalone First

- replay itself is still being defined by the backend
- the governance workflow is already specific enough to justify a standalone product surface
- premature console-wide abstraction will slow down product learning
- a clean standalone replay domain is easier to embed later than a replay feature built directly inside another app

## Current UI Ecosystem Constraint

The sibling repos already use similar frontend tooling, but they are not version-aligned enough to share rendered UI packages blindly today.

Observed versions:

| Product | Next.js | React | Tailwind |
|---|---:|---:|---:|
| `kagent/ui` | `^16.1.5` | `^19.2.4` | `^3.4.17` |
| `agentgateway/ui` | `^15.5.9` | `^19.2.4` | `^4.1.3` |
| `agentregistry/ui` | `^16.0.9` | `^18.2.0` | `^3.3.0` |

Implication:

- yes to aligning replay with the same ecosystem
- no to prematurely extracting a shared rendered component library across all apps before version convergence

## Frontend Stack Recommendation

Use:

- `Next.js` App Router
- `React`
- `TypeScript`
- `Tailwind CSS`
- `Radix`-style primitives for accessible UI building blocks
- `Storybook` for component development and visual regression workflow
- OpenAPI-based client generation for typed service contracts

### Why This Stack

It fits the current ecosystem and keeps the eventual master UI path open:

- `kagent`, `agentgateway`, and `agentregistry` already use Next-based apps
- App Router is now the idiomatic Next architecture for layouts and route organization
- replay’s workload is read-heavy and review-oriented, which fits Server Components well
- Tailwind keeps delivery fast while still allowing a strong, intentional design system
- Storybook gives a durable shared UI development workflow before full console unification

## Next.js Architecture Guidance

Use Next idiomatically, not as a generic SPA wrapper.

### Recommended defaults

- Server Components by default for initial page rendering
- Client Components only for interactive leaves
- fetch from the Go backend directly in Server Components when possible
- avoid routing all backend traffic through Next Route Handlers unless there is a real BFF need

### Good Client Component use cases

- step-by-step trace compare panes
- split views
- interactive filters
- polling gate run status
- drawers, tabs, and expand/collapse panels

### Route Handler use cases

- auth/session mediation
- file export / report download
- webhook receivers
- cross-service aggregation that should not live in the browser

## Architecture Recommendation

Build one replay domain that can be mounted in two shells.

### Principle

One domain, two entrypoints:

- standalone `replay-web`
- later `console-web` under `Governance`

### Practical implication

Do not put replay business logic directly in `app/` page files.

Instead, separate:

- shell app
- replay domain pages/components
- replay API client
- shared primitives

## Proposed Repo Shape For Replay UI V1

Recommended initial structure:

```text
ui/
  apps/
    replay-web/
  packages/
    replay-sdk/
    replay-domain/
    ui/
    eslint-config/
    typescript-config/
```

Suggested package responsibilities:

- `apps/replay-web`
  - Next app shell
  - routes
  - app-level layout
  - auth/session wiring if needed

- `packages/replay-sdk`
  - generated API types
  - typed API client
  - request helpers

- `packages/replay-domain`
  - baselines page modules
  - drift inbox modules
  - gate review modules
  - trace compare modules
  - domain-specific view models and formatters

- `packages/ui`
  - reusable primitives
  - layout helpers
  - badges
  - tables
  - split panes
  - timeline primitives

## Future Master Console Shape

Later, when the shared console becomes real:

```text
console/
  apps/
    console-web/
    replay-web/
  packages/
    domain-replay/
    domain-kagent/
    domain-gateway/
    domain-registry/
    api-replay/
    api-kagent/
    api-gateway/
    api-registry/
    ui/
```

That gives:

- a standalone replay product
- a single integrated console later
- one implementation of replay domain screens

## UX Scope For Replay UI V1

The first UI should be a review console.

### Core screens

1. `Baselines`
   - baseline list
   - model/provider metadata
   - tool profile summary
   - latest drift status
   - promote / remove baseline actions

2. `Drift Inbox`
   - pass / warn / fail queue
   - drift score
   - changed dimensions
   - tool order / frequency changes
   - risk escalation indicator

3. `Gate Runs`
   - experiment list
   - baseline vs variant config
   - verdict
   - started / completed state
   - threshold and summary score

4. `Gate Review`
   - verdict-first page
   - dimension breakdown
   - first divergence
   - response similarity
   - tool and risk diffs

5. `Trace Compare`
   - baseline left pane
   - candidate right pane
   - center divergence rail
   - step-by-step compare
   - expandable raw messages / tool payloads

### Design direction

This should feel closer to:

- code review
- deploy review
- incident timeline

It should not look like:

- a generic metrics dashboard
- a prompt playground
- a copy of LangSmith / Langfuse / Phoenix

## API Work Needed Before A Serious UI

The storage layer already suggests the right product objects, but the HTTP layer is still too narrow for a real review console.

Needed APIs:

- list traces
- trace detail
- list baselines
- create / remove baseline
- list drift results
- drift result detail
- list experiments
- experiment detail
- gate run detail
- tool captures by trace
- compare baseline vs candidate detail payload

Prefer a clean JSON API with stable identifiers and typed response models before building a large UI surface.

## V1 Review Workflow State Model

The plan should be explicit about what the operator is reviewing and what decisions exist in V1.

### Primary review objects

- `Baseline`
  - approved reference trace used for comparison
  - can be promoted or removed

- `Drift Result`
  - automated comparison of a candidate trace against a baseline
  - shows pass / warn / fail plus changed dimensions

- `Gate Run`
  - experiment created from replaying a baseline against a variant configuration
  - has running / completed / failed lifecycle

- `Gate Review`
  - operator review surface for a completed gate run
  - answers: what changed, where, and how risky is it

- `Trace Comparison`
  - raw side-by-side evidence for baseline vs candidate behavior

### V1 decision model

Phase 2 should support review and operational decision-making, but it should not over-promise workflow persistence that the product does not yet implement.

- `Approve baseline`
  - persisted in Phase 2 via baseline promote action

- `Remove baseline`
  - persisted in Phase 2 via baseline removal action

- `Approve or reject a candidate change`
  - supported as a human decision in Phase 2 UI flow
  - persisted approval notes, reviewer attribution, and durable audit artifacts belong to Phase 3

### Explicit V1 cut line

Phase 2 is a review console, not a full workflow engine.

Phase 2 must provide:

- enough context to decide whether a candidate is safe
- stable links between baselines, drift, gate runs, and trace compare
- clear verdict-first summaries with raw evidence available on demand

Phase 2 does not need to provide:

- reviewer notes
- approval history
- exportable reports
- multi-step approval workflows
- RBAC-heavy collaboration features

## Recommended Delivery Plan

### Phase 0: Lock Product Shape

Goals:

- confirm replay UI is a governance console, not generic observability
- define V1 screen list
- define the minimum JSON API needed for those screens

Deliverables:

- product IA
- API contract list
- wireframe spec

### Phase 1: Backend Surface For UI

Goals:

- expose list/detail APIs for baselines, drift, experiments, and trace compare
- stabilize payloads around the review workflow

Deliverables:

- OpenAPI spec
- generated TS client
- seeded demo data path for UI development

### Phase 2: Standalone Replay UI

Goals:

- ship the standalone governance console
- prove the workflow on real replay data

Assumptions:

- one strong frontend engineer full-time
- one backend engineer available part-time for contract fixes and seed data adjustments
- design support is light but real, at least for wireframes and visual review
- backend Phase 1 contract is stable enough that Phase 2 should discover refinements, not large missing surfaces

Deliverables:

- `Baselines`
- `Drift Inbox`
- `Gate Runs`
- `Gate Review`
- `Trace Compare`

Success criteria:

- a user can choose a baseline
- inspect a failed candidate
- understand why it failed
- decide whether to approve or reject a change

### Phase 2 Execution Sequence

Phase 2 should not be executed as “build five screens in parallel.” The right sequencing is workflow-first.

#### Phase 2A: App shell and UI foundation

Deliver:

- `ui/apps/replay-web`
- routing and top-level layout
- shared query/fetch infrastructure
- `packages/replay-domain`
- `packages/ui`
- demo dataset loading path and local developer workflow

Acceptance criteria:

- app boots locally against seeded data
- typed SDK is consumed through one domain access layer, not page-level fetch sprawl
- routes, layout, and shared primitives exist for the five planned screens

#### Phase 2B: Baselines and Drift Inbox

Deliver:

- baseline list
- promote / remove baseline actions
- drift queue with pass / warn / fail segmentation
- deep links from drift items into trace compare

Acceptance criteria:

- operator can identify the currently approved baseline set
- operator can see latest drift status per baseline
- operator can open a failing or warning candidate from the inbox in one click
- empty, loading, and error states exist for both screens

#### Phase 2C: Gate Runs

Deliver:

- experiment list
- running / completed / failed states
- baseline vs variant config summary
- threshold and verdict summary

Acceptance criteria:

- operator can distinguish in-flight from terminal runs
- operator can see which baseline and variant config produced a run
- operator can open a run into Gate Review

#### Phase 2D: Gate Review

Deliver:

- verdict-first summary panel
- dimension breakdown
- first divergence summary
- response, tool, and risk sections
- deep link into full trace compare

Acceptance criteria:

- operator can explain why the run passed or failed without reading raw payloads first
- operator can identify first divergence and changed dimensions in under a minute
- failed and incomplete runs are visually distinct

#### Phase 2E: Trace Compare

Deliver:

- split-pane baseline/candidate view
- divergence rail
- step-by-step comparison
- expandable raw messages
- expandable tool payloads

Acceptance criteria:

- operator can navigate step-by-step differences without losing context
- raw evidence is available without leaving the compare screen
- large payloads remain readable and collapsible

#### Phase 2F: Demo completion pass

Deliver:

- seeded end-to-end demo flow
- navigation polish across the five screens
- ready-to-show review path from drift or gate run into trace compare

Acceptance criteria:

- demo operator can start at inbox or runs and complete the review workflow without CLI help
- all five screens work against the same coherent seeded scenario

### Phase 2 Scope Tiers

#### Must-have in Phase 2

- all five core screens
- seeded data path
- stable route structure
- verdict-first summaries
- error / loading / empty states on every screen
- navigation between baselines, drift, runs, review, and compare

#### Nice-to-have in Phase 2 if time allows

- sortable tables
- saved filters in URL state
- richer tool diff visualizations
- optimistic promote / remove baseline interactions
- keyboard-friendly compare navigation

#### Explicitly deferred to Phase 3

- reviewer notes
- audit trail
- export
- Storybook visual regression coverage
- full accessibility pass

### Phase 2 Acceptance Criteria By Screen

#### `Baselines`

- list renders approved baselines with model/provider context
- promote and remove actions are available and confirmed
- latest drift status is visible without opening detail

#### `Drift Inbox`

- pass / warn / fail are visually distinguishable
- each row shows score, changed dimensions, and risk escalation signal
- opening a row leads directly to evidence

#### `Gate Runs`

- status, threshold, summary score, and variant config are visible in list view
- failed runs are not visually mixed with completed-pass runs

#### `Gate Review`

- verdict is visible above the fold
- first divergence and dimension breakdown are visible without expanding raw payloads
- raw evidence is available by one interaction

#### `Trace Compare`

- baseline and candidate remain aligned while scrolling or stepping
- divergence rail points to the first meaningful difference
- raw prompts and tool payloads can be expanded in-place

### Phase 2 Quality Bar

The plan should not defer basic product quality into a later hardening phase.

Required in Phase 2:

- empty, loading, and error states on all five screens
- deterministic seeded demo data
- URL-addressable review screens
- no broken navigation between related review objects
- mobile-safe layout for list/detail workflows, even if compare is desktop-optimized
- basic keyboard accessibility for navigation and expandable panels

Performance targets:

- list screens should render useful content in under 2 seconds on the seeded dataset
- review-to-compare navigation should feel instant on local/dev data
- compare interactions should avoid full-page reloads

Testing targets:

- typed API client generation is part of normal dev workflow
- domain formatters and view-model mappers have unit coverage
- critical screen flows have component or integration coverage

### Staffing And Timeline

Realistic estimate:

- `1 frontend engineer + part-time backend support`: 3 to 5 engineer-weeks
- `2 frontend engineers + part-time backend support`: 2 to 3 calendar weeks

Suggested ownership split:

- Engineer 1
  - app shell
  - navigation
  - baselines
  - drift inbox
  - shared UI primitives

- Engineer 2
  - gate runs
  - gate review
  - trace compare
  - domain view models
  - review-state interaction polish

Backend support owns:

- payload refinements discovered during UI build
- demo seed consistency
- compare payload stability

### Phase Gates

Do not call Phase 2 complete until all three are true:

1. The five core screens are implemented.
2. A seeded end-to-end review demo works without CLI assistance.
3. A user can move from list -> review -> evidence -> decision without ambiguity.

### Phase 3: Hardening

Goals:

- improve operational readiness
- reduce ambiguity in review flows

Deliverables:

- empty / error / loading states
- approval notes
- audit trail
- exportable review artifacts
- accessibility pass
- visual regression coverage in Storybook

### Phase 4: Master Console Integration

Goals:

- embed replay into the shared console without rewriting the replay domain

Deliverables:

- mount replay under `Governance`
- adopt shared navigation, auth, and environment selectors
- add cross-links from agents, gateway routes, and registry artifacts into replay views

## Explicit Non-Goals Right Now

Do not build first:

- a generic trace explorer
- a broad prompt playground
- a full eval-suite manager
- microfrontend/module-federation infrastructure
- Next Multi-Zones
- a giant unified backend before the UI proves the workflows

These add complexity without strengthening replay’s actual differentiator.

## Top-Tier Criteria

For this product, top-tier means:

- verdict-first information hierarchy
- clear explanation of why a run passed or failed
- fast side-by-side comparison
- risk-forward visuals
- strong auditability
- reusable architecture that survives console integration

It does not mean feature-matching every existing observability platform.

For delivery, top-tier also means:

- the plan has explicit cut lines
- each phase has measurable acceptance criteria
- the workflow is sequenced by user value, not by screen count
- the quality bar starts in Phase 2, not after it
- UI architecture supports later console integration without premature abstraction

## Recommended Immediate Next Steps

1. Convert the five core screens into wireframes with above-the-fold modules, empty states, and navigation links.
2. Create the real `ui/` workspace with `apps/replay-web`, `packages/replay-domain`, and `packages/ui`.
3. Add one domain access layer over the generated SDK so screens do not call generated services directly.
4. Build `Baselines` and `Drift Inbox` first, including the link into evidence views.
5. Build `Gate Runs`, then `Gate Review`, then `Trace Compare`.
6. Freeze one seeded demo scenario and use it as the completion gate for Phase 2.

## External Research References

- Next.js App Router: https://nextjs.org/docs/app
- Next.js Project Structure: https://nextjs.org/docs/app/getting-started/project-structure
- Next.js Route Groups: https://nextjs.org/docs/app/api-reference/file-conventions/route-groups
- Next.js Backend for Frontend Guide: https://nextjs.org/docs/app/guides/backend-for-frontend
- Next.js `transpilePackages`: https://nextjs.org/docs/app/api-reference/config/next-config-js/transpilePackages
- Next.js Multi-Zones: https://nextjs.org/docs/pages/guides/multi-zones
- React Server Components: https://react.dev/reference/rsc/server-components
- Turborepo Core Concepts: https://turborepo.dev/docs/core-concepts
- Turborepo Internal Packages: https://turborepo.dev/docs/core-concepts/internal-packages
- Storybook: https://storybook.js.org/
- Storybook Composition: https://storybook.js.org/docs/sharing/storybook-composition
- Storybook Interaction Testing: https://storybook.js.org/docs/9/writing-tests/interaction-testing
- Storybook Accessibility Testing: https://storybook.js.org/docs/writing-tests/accessibility-testing
- OpenAPI TypeScript: https://openapi-ts.dev/introduction
- OpenAPI React Query: https://openapi-ts.dev/openapi-react-query/
- TanStack Query Advanced SSR: https://tanstack.com/query/latest/docs/react/guides/advanced-ssr

## Final Recommendation

Build replay first.

Build it as a standalone governance UI.

Build it in the same frontend ecosystem as the sibling products.

But structure it as a portable domain package so that, when the master console is ready, replay can become `Governance` without a rewrite.
