# Contributing

Thanks for working on CMDR.

This repository is optimized around a few core workflows: OTLP ingestion, drift detection, replay-based gate checks, and demo reliability. Contributions should strengthen one of those paths rather than broaden the repo into a generic tracing product.

## Before You Start

- Read [README.md](README.md) for the project overview
- Use [docs/README.md](docs/README.md) to find the relevant implementation notes
- If you are changing the product surface, read [docs/GOVERNANCE_V1_CHECKLIST.md](docs/GOVERNANCE_V1_CHECKLIST.md)

## Local Setup

```bash
make setup-dev
cp .env.example .env
make run
```

Helpful commands:

```bash
make test
make test-storage
make lint
make fmt
make demo
```

## Development Expectations

- Keep changes focused on governance workflows: baseline selection, drift review, replay, and gate verdicts
- Preserve evidence fidelity for prompt and tool payloads
- Add or update tests when behavior changes
- Update docs when commands, workflows, or assumptions change
- Prefer explicit, typed contracts over loosely documented behavior

## API And Generated Code

If you change the OpenAPI contract, regenerate the derived files:

```bash
make generate
```

That updates:

- `pkg/api/openapi_generated.gen.go`
- `pkg/apiclient/client.gen.go`
- `ui/packages/api`

## Pull Request Checklist

- The change has a clear user or operator value
- Tests pass locally for the touched area
- Docs or demos are updated if behavior changed
- Generated files are refreshed when the API contract changed
- The branch does not include local-only artifacts or demo output

## Scope Guardrails

Changes are less likely to be accepted if they:

- turn CMDR into a generic observability backend
- add broad product surface with no governance workflow benefit
- hide uncertainty instead of documenting the current project boundary

If the change is primarily for the hackathon launch flow, keep the repo assets and documentation high signal and easy to navigate.
