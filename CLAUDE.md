# CLAUDE.md

Guidance for AI agents working in this repo.

## What this is

**VibeXP**: open-source, self-hostable AI command center (prompts, memories, artifacts, agents, MCP integrations for Claude Code, Cursor, VS Code, etc.). Monorepo shipped as a **single combined Docker image** (issue #61): the Go backend embeds the built frontend SPA and serves it *and* the API from one port — `docker run ghcr.io/vibexp/vibexp`. Two source components, one deployable artifact:

- **`backend/`**: Go REST API (module `github.com/vibexp/vibexp`). Spec-first OpenAPI, PostgreSQL + pgvector, MCP endpoint, pluggable identity-provider auth (Google/GitHub/generic OIDC). Also embeds + serves the SPA (`internal/server/spa.go`, behind the `embedfrontend` build tag) and renders runtime `/config.js`.
- **`frontend/`**: Vite + React + TypeScript SPA. Built to `frontend/dist`, embedded into the backend for release; served by the Vite dev server in local development.

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
           migrations/, openapi.yaml (+paths/ +schemas/ = API source of truth), Dockerfile (combined image),
           internal/server/spa.go (embed + serve SPA, /config.js)
frontend/  SPA: src/ (pages, components, features, hooks, services, lib, utils); built to dist/ and
           embedded into the backend for release (no standalone image / nginx anymore)
Makefile   all dev/CI tasks (backend-*, frontend-*); build-combined = frontend build -> embed -> backend build
docker-compose.yml  runs the PUBLISHED ghcr.io/vibexp/vibexp combined image + Postgres (self-host, NOT for dev)
.github/workflows/  ci-backend, ci-frontend, release (single combined image)
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
- **Single combined image / same-origin.** The backend embeds and serves the SPA from the same origin as the API (relative `VITE_API_BASE_URL=/api/v1`), so there is **no nginx proxy, no `BACKEND_ORIGIN`, and no CORS** in production. The SPA is registered as the chi `NotFound` catch-all (`internal/server/spa.go`) so it never shadows API/MCP/OAuth routes and stays invisible to the OpenAPI drift/coverage gates. The embed is behind the `embedfrontend` build tag (`spa_embed.go`); the **default build has no frontend** (`spa_noembed.go`) so the backend compiles/tests/runs with no `frontend/dist` (local dev + CI).
- **Runtime frontend config (`/config.js`).** Deploy-time, non-secret frontend values (branding/MCP endpoint/optional GTM) are env vars on the backend (`Config.RuntimeFrontendEnv`), served as `window.__VIBEXP_ENV__` via `/config.js` and read by the SPA through `getEnv()` (`src/lib/runtimeEnv.ts`), with build-time `import.meta.env` as the fallback. Self-hosters reconfigure with an env var + restart, no rebuild. Local dev (no backend `config.js`) uses the build-time fallback.
- **Embedded MCP Authorization Server is HTTPS-only.** The AS endpoints (`/oauth2/*`, `/.well-known/oauth-authorization-server`) are wrapped by `requireHTTPSMiddleware` (`internal/server/middleware_https.go`) in `setupOAuthASRoutes`: a non-HTTPS request (no `r.TLS`, `X-Forwarded-Proto != https`) is rejected with 403 unless `Config.IsLocalDevelopment()`. VibeXP is deploy-anywhere, so this assumes TLS terminates upstream and the proxy forwards `X-Forwarded-Proto: https` (document this for self-hosters). Local dev + the e2e stack (localhost) are exempt.
- **No service worker.** No PWA/precaching SW (only on-demand `firebase-messaging-sw.js` for push). `src/utils/serviceWorker.ts` + `public/{sw,dev-sw}.js` evict legacy workers. Don't reintroduce a precaching SW without a cleanup story.
- **No frontend telemetry.** The frontend bundles no error-tracking/maintainer telemetry (Sentry was removed in #58). `ErrorBoundary` and other error paths log via `console.error` only, and the build emits no source maps (`vite.config.ts` `build.sourcemap: false`). A self-hoster wanting error tracking can wire their own.
- **Secrets.** `.env` is gitignored, never commit it; only `.env.example` is tracked, with neutral values (e.g. `example.com`) since this is public and self-hosted.

## CI & releases

- CI runs the same `make` targets, so run them locally first. Backend: download-deps -> format -> build -> test, plus lint and OpenAPI validation. Frontend: install -> lint -> type-check -> test -> build. (Backend CI's `make backend-build` is the default no-embed build, which also guards "the backend builds with no `frontend/dist`".)
- **One artifact, one workflow.** A GitHub Release `vX.Y.Z` (workflow `release.yml`) builds the single combined image `ghcr.io/vibexp/vibexp:X.Y.Z` (+ `:latest`, which docker-compose tracks) — the Dockerfile (`backend/Dockerfile`, built from the repo root) builds the frontend, embeds `dist`, and builds the Go binary with `-tags embedfrontend`. There is no separate frontend image.

## Working agreement

- Branch off `main`, match existing style, run the relevant `make ...-lint` / `...-test` / build targets before committing, open a PR, let CI pass.
- Pre-commit hooks are mandatory (see above); never use `--no-verify`.
- Don't commit secrets or gitignored generated artifacts.
