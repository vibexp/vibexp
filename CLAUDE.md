# CLAUDE.md

Guidance for AI agents (and humans) working in this repository.

## What this is

**VibeXP** — an open-source, self-hostable "personal AI command center": manage
prompts, memories, artifacts, agents, and MCP integrations across tools like
Claude Code, Cursor, and VS Code. This repository is the open-source home of the
product; it is a monorepo containing two independently deployable components:

- **`backend/`** — Go REST API (module `github.com/vibexp/vibexp`). Spec-first
  OpenAPI, PostgreSQL + pgvector, MCP endpoint, WorkOS-based auth.
- **`frontend/`** — standalone Vite + React + TypeScript SPA, served by nginx in
  production.

Two supporting packages are **published to the public npm registry** and consumed
by the frontend (they are NOT in this repo):

- `@vibexp/api-client` — typed API client generated from `backend/openapi.yaml`;
  source lives in the separate repo `vibexp/api-client-js`.
- `@vibexp/design-system` — shared UI/design tokens and components.

## Repository layout

```
backend/            Go API service
  cmd/              CLI entrypoints (cobra)
  internal/         app code: server, services, repositories, auth, container (wire DI), …
  migrations/       SQL migrations
  openapi.yaml      OpenAPI spec (+ paths/, schemas/) — source of truth for the API
  Dockerfile        production image (ghcr.io/vibexp/backend)
  docker-compose.yml local dev Postgres (used by `make backend-run`)
frontend/           Vite/React SPA
  src/              app code: pages, components, features, hooks, services, lib, utils
  Dockerfile        production image: builds the SPA, serves via nginx with an /api proxy
  nginx.conf.template runtime nginx config (reverse-proxies /api → BACKEND_ORIGIN)
Makefile            all dev/CI tasks (backend-* and frontend-* targets)
docker-compose.yml  runs the PUBLISHED images (ghcr.io/vibexp/*) + Postgres for self-hosting
.github/workflows/  ci-backend, ci-frontend (make-driven), release-backend, release-frontend
```

## Local development — use `make`, not docker-compose

Local development uses the Makefile. The root `docker-compose.yml` is for
*running the published images* (self-host/deploy), not for developing.

Backend:
- `make backend-run-dev` — Postgres (via `backend/docker-compose.yml`) + hot-reload API (air)
- `make backend-test` / `make backend-lint` / `make backend-check`
- `make backend-generate-openapi-server` — regenerate server bindings from the spec
- `make backend-wire-gen` — regenerate Wire DI; `make backend-mock-generate` — regenerate mocks

Frontend:
- `make frontend-run-dev` — Vite dev server (http://localhost:5173)
- `make frontend-install` / `frontend-lint` / `frontend-type-check` / `frontend-test` / `frontend-build`

### Pre-commit hooks are MANDATORY

This repo ships a `.pre-commit-config.yaml` that gates every commit on the same
quality checks CI runs (gofmt, golangci-lint, govulncheck, gosec, OpenAPI
validation, frontend lint/type-check/test/build, gitleaks, and policy hooks).
Local development **must** have these hooks installed — skipping them lets
quality regressions slip in unnoticed.

- Install once per clone: `pre-commit install` (requires the `pre-commit` tool —
  `pipx install pre-commit` or `brew install pre-commit`).
- **Agents:** before committing, verify the hook is installed (e.g.
  `.git/hooks/pre-commit` exists and `command -v pre-commit` succeeds). If
  `pre-commit` is not installed, **stop and ask the user to install it** — do not
  proceed without it. This is non-negotiable; a missing hook means missed code
  quality.
- **Never bypass the hooks.** Using `git commit --no-verify` / `-n` (or
  `git push --no-verify`) to skip pre-commit is forbidden. If a hook fails, fix
  the underlying problem rather than evading the check.

### Go toolchain is pinned
The Makefile sets `GOTOOLCHAIN=go1.25.11` (matching CI) so local builds use the
exact same Go as CI — this keeps `govulncheck`/`staticcheck`/analyzers
reproducible. Keep `GO_VERSION` in the Makefile in sync with the `go-version`
in `.github/workflows/ci-backend.yml`.

## Conventions & gotchas

- **Spec-first backend.** `backend/openapi.yaml` (bundled from `paths/` + `schemas/`)
  is the source of truth. Generated server code (oapi-codegen) and Wire/mocks are
  committed; regenerate via the `make` targets above rather than hand-editing
  `*_gen.go` / `mock_*.go` / `wire_gen.go`. After the module rename, generated
  files must stay `gofmt -s` clean (CI enforces it).
- **Frontend ↔ API client.** The frontend imports `@vibexp/api-client` from npm.
  Changing the backend API means: update `backend/openapi.yaml` → release a new
  `@vibexp/api-client` (from `vibexp/api-client-js`) → bump the frontend dep.
- **Frontend is deployment-agnostic.** It is built with a relative
  `VITE_API_BASE_URL=/api/v1`; the production nginx image reverse-proxies `/api/`
  to `BACKEND_ORIGIN` (default `http://backend:8080`). Don't hardcode a backend
  origin into the build.
- **No service worker.** The app ships no PWA/workbox service worker (only an
  on-demand `firebase-messaging-sw.js` for push). `src/utils/serviceWorker.ts`
  evicts stale/legacy workers on boot, and `public/{sw,dev-sw}.js` are
  self-destruct kill-switches for browsers still holding an old worker. Do not
  reintroduce a precaching service worker without a clear upgrade/cleanup story.
- **Secrets.** `.env` files are gitignored and must never be committed; only
  `.env.example` is tracked. Keep example/config values neutral (e.g.
  `example.com`) — this is open source and self-hosted by third parties.

## Testing & CI

- CI mirrors local exactly because every CI step calls a `make` target.
  - `ci-backend.yml`: `backend-download-deps` → `backend-format` → `backend-build`
    → `backend-test`; plus `backend-lint` (golangci-lint) and OpenAPI validation.
  - `ci-frontend.yml`: `frontend-install` → `frontend-lint` → `frontend-type-check`
    → `frontend-test` → `frontend-build`.
- Run the same targets locally before pushing.

## Releases & deployment

Backend and frontend are versioned **independently** via prefixed Git tags:

- Create a GitHub **Release** tagged `backend-vX.Y.Z` → builds & pushes
  `ghcr.io/vibexp/backend:X.Y.Z` (+ `:latest` for non-prereleases).
- `frontend-vX.Y.Z` → `ghcr.io/vibexp/frontend:X.Y.Z` (+ `:latest`).

Self-hosters run the published images:

```sh
docker compose up -d   # uses ghcr.io/vibexp/{backend,frontend}:latest + Postgres (pgvector)
```

`docker-compose.yml` persists data in the `pgdata` volume, runs on a dedicated
`vibexp` network, enables dev-login for local evaluation, and includes an
optional GCS-emulator service for persistent file attachments.

## Working agreement for agents

- Prefer the `make` targets; match the existing code's style and conventions.
- Before committing: run the relevant `make ...-lint` / `...-test` / build targets.
- Pre-commit hooks are mandatory — ensure they are installed (`pre-commit install`)
  and **never** bypass them with `--no-verify` / `-n`. If `pre-commit` isn't
  installed, ask the user to install it before committing. See
  "Pre-commit hooks are MANDATORY" above.
- Don't commit secrets or generated artifacts that are gitignored.
- Branch off `main`, open a PR, and let CI pass before merging.
