# Documentation Guide

## Start Here

- [../README.md](../README.md) — Repository overview with three demo tiers (quick, full-stack, real-model)
- [QUICKSTART.md](QUICKSTART.md) — Local setup, health checks, and first commands
- [DEMO.md](DEMO.md) — Complete runbook for all three demo levels with expected outputs

## Demo & Submission

- [DEMO.md](DEMO.md) — **Primary demo runbook**: Level 1 (no API keys), Level 2 (agentgateway + freeze-mcp), Level 3 (real OpenAI)
- [MIGRATION_DEMO.md](MIGRATION_DEMO.md) — Deep-dive on the full-stack migration scenario
- [BLOG_DRAFT.md](BLOG_DRAFT.md) — "Same Model, Different Instructions, CMDR Caught It" — grounded in real GPT-4o-mini results
- [SUBMISSION_NOTES.md](SUBMISSION_NOTES.md) — Hackathon framing and scoring alignment

## Architecture

- [GATE_REPLAY_ARCHITECTURE.md](GATE_REPLAY_ARCHITECTURE.md) — Replay and gate design (prompt-only + agent-loop)
- [DATABASE_LAYER.md](DATABASE_LAYER.md) — Schema and persistence overview

## Suggested Reading Order

1. Read [../README.md](../README.md) — understand the three demo tiers
2. Run Level 1: `make dev-up && make demo` — see CMDR in 30 seconds
3. Read [DEMO.md](DEMO.md) — understand what each level proves
4. Run Level 2 or 3 if you have the prerequisites
5. Read [BLOG_DRAFT.md](BLOG_DRAFT.md) — the narrative version for non-technical audiences
