# CLAUDE.md

Guidance for AI agents working in this repo.

## What this is

**VibeXP**: open-source, self-hostable AI command center (prompts, memories, artifacts, agents, MCP integrations for Claude Code, Cursor, VS Code, etc.). Monorepo with two independently deployable components:

- **`backend/`**: Go REST API (module `github.com/vibexp/vibexp`). Spec-first OpenAPI, PostgreSQL + pgvector, MCP endpoint, pluggable identity-provider auth (Google/GitHub/generic OIDC).
- **`frontend/`**: Vite + React + TypeScript SPA, served by nginx in production.

The frontend consumes two npm packages that are NOT in this repo: `@vibexp/api-client` (typed client generated from `backend/openapi.yaml`) and `@vibexp/design-system` (shared UI).

## Related repos

Only `backend/` and `frontend/` live here. If a task needs another VibeXP project, clone it into this repo's **parent directory** (as a sibling, e.g. `$HOME/Projects/<name>` when this repo is `$HOME/Projects/vibexp`):

| Repo | Repo URL | Live site | What |
|---|---|---|---|
| website | https://github.com/vibexp/website | https://vibexp.io | Marketing site |
| docs | https://github.com/vibexp/docs | https://docs.vibexp.io | Docs site |
| blog | https://github.com/vibexp/blog | https://blog.vibexp.io | Blog |
| cli | https://github.com/vibexp/cli | - | VibeXP CLI |
| api-client-js | https://github.com/vibexp/api-client-js | - | Source of `@vibexp/api-client` |

## Layout

```
backend/   Go API: cmd/ (cobra), internal/ (server, services, repositories, auth, container=wire DI),
           migrations/, openapi.yaml (+paths/ +schemas/ = API source of truth), Dockerfile
frontend/  SPA: src/ (pages, components, features, hooks, services, lib, utils), Dockerfile,
           nginx.conf.template (reverse-proxies /api -> BACKEND_ORIGIN)
Makefile   all dev/CI tasks (backend-*, frontend-*)
docker-compose.yml  runs PUBLISHED ghcr.io/vibexp/* images + Postgres (self-host, NOT for dev)
.github/workflows/  ci-backend, ci-frontend, release-backend, release-frontend
```

## Local development: use `make`

The root `docker-compose.yml` runs the published images (self-host), not for developing. Develop via the Makefile:

- Backend: `make backend-run-dev` (Postgres + air hot-reload), plus `backend-test`, `backend-lint`, `backend-check`, `backend-generate-openapi-server`, `backend-wire-gen`, `backend-mock-generate`.
- Frontend: `make frontend-run-dev` (Vite, http://localhost:5173), plus `frontend-install`, `frontend-lint`, `frontend-type-check`, `frontend-test`, `frontend-build`.
- Go toolchain is pinned: the Makefile sets `GOTOOLCHAIN=go1.25.11`. Keep `GO_VERSION` in sync with `go-version` in `ci-backend.yml`.

## Pre-commit hooks are MANDATORY

`.pre-commit-config.yaml` gates every commit on the CI checks (gofmt, golangci-lint, govulncheck, gosec, OpenAPI validation, frontend lint/type-check/test/build, gitleaks, policy hooks).

- Install once per clone: `pre-commit install` (needs `pre-commit`: `pipx install pre-commit` or `brew install pre-commit`).
- Before committing, verify it is installed. If `pre-commit` is missing, STOP and ask the user to install it. Do not proceed.
- Never bypass the hooks (`git commit`/`push --no-verify` or `-n` is forbidden). If a hook fails, fix the cause.

## Conventions & gotchas

- **Spec-first backend.** `backend/openapi.yaml` (bundled from `paths/` + `schemas/`) is the source of truth. Generated code (oapi-codegen server, Wire, mocks) is committed: regenerate via `make`, never hand-edit `*_gen.go` / `mock_*.go` / `wire_gen.go`. Generated files must stay `gofmt -s` clean (CI enforces).
- **API change flow.** Update `backend/openapi.yaml` -> release a new `@vibexp/api-client` (from `api-client-js`) -> bump the frontend dep.
- **Frontend is deployment-agnostic.** Built with relative `VITE_API_BASE_URL=/api/v1`; nginx proxies `/api/` to `BACKEND_ORIGIN` (default `http://backend:8080`). Don't hardcode a backend origin.
- **No service worker.** No PWA/precaching SW (only on-demand `firebase-messaging-sw.js` for push). `src/utils/serviceWorker.ts` + `public/{sw,dev-sw}.js` evict legacy workers. Don't reintroduce a precaching SW without a cleanup story.
- **No frontend telemetry.** The frontend bundles no error-tracking/maintainer telemetry (Sentry was removed in #58). `ErrorBoundary` and other error paths log via `console.error` only, and the build emits no source maps (`vite.config.ts` `build.sourcemap: false`). A self-hoster wanting error tracking can wire their own.
- **Secrets.** `.env` is gitignored, never commit it; only `.env.example` is tracked, with neutral values (e.g. `example.com`) since this is public and self-hosted.

## CI & releases

- CI runs the same `make` targets, so run them locally first. Backend: download-deps -> format -> build -> test, plus lint and OpenAPI validation. Frontend: install -> lint -> type-check -> test -> build.
- Released independently via prefixed Git tags: a GitHub Release `backend-vX.Y.Z` builds `ghcr.io/vibexp/backend:X.Y.Z` (+ `:latest`); `frontend-vX.Y.Z` likewise.

## Working agreement

- Branch off `main`, match existing style, run the relevant `make ...-lint` / `...-test` / build targets before committing, open a PR, let CI pass.
- Pre-commit hooks are mandatory (see above); never use `--no-verify`.
- Don't commit secrets or gitignored generated artifacts.
