# Documentation Guide

This directory is organized around a few distinct audiences: someone trying to run CMDR quickly, someone trying to understand the architecture, and someone trying to present or evaluate the project.

## Start Here

- [../README.md](../README.md) — repository overview and the shortest path to running CMDR
- [QUICKSTART.md](QUICKSTART.md) — local setup, health checks, and first commands
- [LOCAL_DEV_SETUP.md](LOCAL_DEV_SETUP.md) — deeper local development guidance

## Demo And Launch

- [DEMO.md](DEMO.md) — deterministic 2-3 minute demo flow
- [MIGRATION_DEMO.md](MIGRATION_DEMO.md) — full gateway-driven migration scenario
- [E2E_DEMO_PLAN.md](E2E_DEMO_PLAN.md) — how the demo maps to hackathon scoring
- [SUBMISSION_NOTES.md](SUBMISSION_NOTES.md) — hackathon framing, pitch, and judging alignment
- [BLOG_DRAFT.md](BLOG_DRAFT.md) — "Same Model, Different Instructions, CMDR Caught It" blog post draft

## Architecture And Data Flow

- [GATE_REPLAY_ARCHITECTURE.md](GATE_REPLAY_ARCHITECTURE.md) — replay and gate design (prompt-only + agent-loop)
- [AGENTGATEWAY_CAPTURE.md](AGENTGATEWAY_CAPTURE.md) — capture path through agentgateway
- [FREEZE_AGENT_LOOP.md](FREEZE_AGENT_LOOP.md) — initial plumbing proof for freeze-mcp agent loop
- [OTLP_RECEIVER.md](OTLP_RECEIVER.md) — OTLP ingestion and parsing details
- [DATABASE_LAYER.md](DATABASE_LAYER.md) — schema and persistence overview

## Governance Product Direction

- [GOVERNANCE_V1_CHECKLIST.md](GOVERNANCE_V1_CHECKLIST.md) — decision filter for the API surface
- [GOVERNANCE_V1_CONTRACT.md](GOVERNANCE_V1_CONTRACT.md) — semantic contract for V1 endpoints and objects

## Suggested Reading Order

If you are new to the repo:

1. Read [../README.md](../README.md)
2. Run [QUICKSTART.md](QUICKSTART.md)
3. Use [DEMO.md](DEMO.md) for the deterministic flow
4. Use [MIGRATION_DEMO.md](MIGRATION_DEMO.md) for the strongest end-to-end proof
5. Dive into the architecture docs only for the area you are changing
