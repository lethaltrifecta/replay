# Replay Governance UI

Sibling-style Next.js app for the first governance review surface.

## Core Surfaces

- `selection`: baseline and candidate trace selection
- `drift`: inbox and pair-specific drift review
- `compare`: raw side-by-side evidence
- `experiments`: canonical experiment report review

## Local Development

Install dependencies:

```bash
make ui-install
```

Regenerate the client after contract changes:

```bash
make ui-generate
```

Start the app:

```bash
make ui-dev
```

The app uses:

- `REPLAY_API_ORIGIN` in the repo root for Next rewrites
- `NEXT_PUBLIC_REPLAY_API_BASE` for the in-app client base path

See [`.env.example`](/Users/kevin/git/lethaltrifecta/replay/.claude/worktrees/ui-phase-2/ui/.env.example).

## Quality Checks

Run the full local UI loop:

```bash
make ui-lint
make ui-typecheck
make ui-build
make ui-test-e2e
```

The Playwright suite mocks the API at the network boundary, so smoke tests do not depend on a running backend.
