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
| api-client-js | https://github.com/vibexp/api-client-js | - | Source of `@vibexp/api-client` (npm) |
| api-client-go | https://github.com/vibexp/api-client-go | - | Generated Go client (`github.com/vibexp/api-client-go`) |

## Layout

```
backend/   Go API: cmd/ (cobra), internal/ (server, services, repositories, auth, container=wire DI),
           migrations/, openapi.yaml (+paths/ +schemas/ = API source of truth), Dockerfile (combined image),
           internal/server/spa.go (embed + serve SPA, /config.js)
frontend/  SPA: src/ (pages, components, features, hooks, services, lib, utils); built to dist/ and
           embedded into the backend for release (no standalone image / nginx anymore)
Makefile   all dev/CI tasks (backend-*, frontend-*); build-combined = frontend build -> embed -> backend build
docker-compose.yml  runs the PUBLISHED ghcr.io/vibexp/vibexp combined image + Postgres (self-host, NOT for dev)
.github/workflows/  ci (consolidated backend+frontend+sonar, #390), ci-e2e, release (single combined image)
```

## Local development: use `make`

The root `docker-compose.yml` runs the published images (self-host), not for developing. Develop via the Makefile:

- Backend: `make backend-run-dev` (Postgres + air hot-reload), plus `backend-test`, `backend-lint`, `backend-check`, `backend-generate-openapi-server`, `backend-wire-gen`, `backend-mock-generate`.
- Frontend: `make frontend-run-dev` (Vite, http://localhost:5173), plus `frontend-install`, `frontend-lint`, `frontend-type-check`, `frontend-test`, `frontend-build`.
- Go toolchain is pinned: the Makefile sets `GOTOOLCHAIN=go1.25.11`. Keep `GO_VERSION` in sync with `go-version` in `ci.yml`.

## Pre-commit hooks are MANDATORY

`.pre-commit-config.yaml` gates every commit on the CI checks (gofmt, golangci-lint, govulncheck, gosec, OpenAPI validation, frontend lint/type-check/test/build, gitleaks, policy hooks).

- Install once per clone: `pre-commit install` (needs `pre-commit`: `pipx install pre-commit` or `brew install pre-commit`).
- Before committing, verify it is installed. If `pre-commit` is missing, STOP and ask the user to install it. Do not proceed.
- Never bypass the hooks (`git commit`/`push --no-verify` or `-n` is forbidden). If a hook fails, fix the cause.

## Conventions & gotchas

- **Spec-first backend.** `backend/openapi.yaml` (bundled from `paths/` + `schemas/`) is the source of truth. Generated code (oapi-codegen server, Wire, mocks) is committed: regenerate via `make`, never hand-edit `*_gen.go` / `mock_*.go` / `wire_gen.go`. Generated files must stay `gofmt -s` clean (CI enforces).
- **Authorization goes through `internal/authz` — never re-implement the matrix (epic #220).** That package **is** the permission spec: the role→permission table (owner/admin/member) lives in `matrix.go` and the 14 dot-namespaced `Permission` constants in `permission.go`. Rules: (1) a service method that **mutates team-scoped data** must authorize via `services.AuthorizationService` (`Can` / `Authorize` / `CanActOnResource` for own-vs-any) rather than checking a role inline — prefer `Authorize`, which hands back the resolved role so you can echo it without a second lookup; (2) **no role predicates in repository SQL** (decision D3) — repositories are tenancy-only, and `TestNoRolePredicatesInRepositorySQL` fails the build if a `role IN (...)`-style predicate reappears; (3) the permission strings are **published API surface** (the `permissions` array on team payloads, #224) and are pinned to the spec enum by `TestTeamPermissionsEnumMatchesAuthzConstants` — renaming one is a breaking change, so change the constant and `schemas/teams.yaml` together; (4) clients (incl. the SPA) **gate on the server's `permissions` array, never on `role`** — the matrix lives here, and UI gating is convenience only since every write is authorized server-side regardless.
- **API change flow.** Update the spec (`backend/openapi.yaml` / `backend/paths/**` / `backend/schemas/**`) -> merge to `main` -> **both** generated clients publish **automatically** -> bump the frontend dep (and any Go consumer). The two clients are `@vibexp/api-client` (npm, source `vibexp/api-client-js`) and `github.com/vibexp/api-client-go` (Go module, source `vibexp/api-client-go`). **Do NOT hand-publish/hand-tag either client** (no manual `workflow_dispatch`, no GitHub Release, no `git tag` for a spec change): merging a spec change to `main` fires `.github/workflows/publish-api-client.yml`, which dispatches *both* downstream workflows — the api-client-js Publish and the api-client-go Release — and each **auto-minor-bumps** its own version off its latest release (every spec change = one new minor per client, breaking or not). The Go client "publishes" by committing the regenerated `vibexp.gen.go` and pushing a `vX.Y.Z` git tag (Go resolves modules from source at the tag — there is no registry); it stays on **v0.x** deliberately, since Go's semantic import versioning would force a `/vN` module-path suffix at v2+. Publishing by hand on top of the automation races it and can waste/duplicate a version. Only the frontend dependency bump remains manual (out of scope for the automation). Cross-repo trigger uses the "VibeXP Bot" GitHub App (installed on both client repos, **Actions: read/write** only — the Go tag push uses api-client-go's own in-repo `GITHUB_TOKEN`, not the app; secrets `VIBEXP_BOT_APP_CLIENT_ID` / `VIBEXP_BOT_APP_PRIVATE_KEY` on this repo). Tracking: #303 (JS), #329/#330 (Go).
- **Response conformance is progressive (tracked in #122).** The backend is spec-first for routing/request types, but response bodies are still largely hand-marshaled (`map[string]interface{}` / bespoke `models.*` structs) and **not checked against the schema**, so spec↔backend response drift ships silently (it has crashed the frontend three times: #105 / #121 / #132). We are closing this **domain-by-domain, not in one campaign**. **Whenever you touch a documented domain, leave it more conformant than you found it:** (1) return `oapi-codegen` strict-server generated response types, not `map[string]interface{}` / bespoke structs; (2) add a spec-validated response assertion (`specconformance.AssertConformsToSpec`) in the handler test and delete that op's entry from the payload-coverage ledger (`internal/server/openapi_payload_coverage_test.go`); (3) never serialize a required array as `null` — for a documented response with a *required* array field, declare it `models.JSONArray[T]` (a `[]T` whose `MarshalJSON` emits `[]` for nil), which guarantees `[]` by construction regardless of test coverage (issue #125, "Layer C"). `models.JSONArray[T]` is assignable to/from `[]T`, so only the field declaration changes; keep a plain `[]T` for arrays the spec marks nullable (`x | null`, e.g. `Prompt.labels`, `AgentStatsResponse.recent_activities`). The invariant is CI-enforced by `TestRequiredResponseArraysNeverNull` (`internal/server/required_array_null_test.go`), which derives every required-array response field from the spec (`specconformance.RequiredArrayFields`) and fails if one is not shim-protected — a new such field must be added to that test's registry (and use `JSONArray[T]`) or the documented ad-hoc allowlist. **New endpoints are non-negotiable:** they must ship strict-server-typed and ledger-covered (both gates are already CI-enforced), so the backlog never grows. Update the per-domain checklist in #122 as you go.
- **Single combined image / same-origin.** The backend embeds and serves the SPA from the same origin as the API (relative `VITE_API_BASE_URL=/api/v1`), so there is **no nginx proxy, no `BACKEND_ORIGIN`, and no CORS** in production. The SPA is registered as the chi `NotFound` catch-all (`internal/server/spa.go`) so it never shadows API/MCP/OAuth routes and stays invisible to the OpenAPI drift/coverage gates. The embed is behind the `embedfrontend` build tag (`spa_embed.go`); the **default build has no frontend** (`spa_noembed.go`) so the backend compiles/tests/runs with no `frontend/dist` (local dev + CI).
- **Runtime frontend config (`/config.js`).** Deploy-time, non-secret frontend values (branding/MCP endpoint/optional GTM) are env vars on the backend (`Config.RuntimeFrontendEnv`), served as `window.__VIBEXP_ENV__` via `/config.js` and read by the SPA through `getEnv()` (`src/lib/runtimeEnv.ts`), with build-time `import.meta.env` as the fallback. Self-hosters reconfigure with an env var + restart, no rebuild. Local dev (no backend `config.js`) uses the build-time fallback.
- **Config is a required `config.yaml` (koanf, epic #68).** The backend loads a nested `config.Config` from a **required** `config.yaml` (`internal/config`), resolved from `--config` → `$VIBEXP_CONFIG_FILE` → `./config.yaml`, with `${VAR}` / `${VAR:-default}` / `$${literal}` interpolation of string scalars from the environment; a missing file fails fast. Two committed surfaces: **`backend/config.example.yaml`** is the DEV-TUNED documented surface (localhost, Mailpit; copied to a gitignored `config.yaml` by `scripts/sync-env.sh` for `make backend-run*`), and **`backend/.env.example`** holds only the `${VAR}` secrets. `backend/config.schema.json` is generated (`make backend-generate-config-schema`, drift-gated). Adding a field: edit the `Config` struct + `defaults()` + `config.example.yaml`, regenerate the schema. Fields are optional in the schema — anything omitted from a config file inherits `defaults()`.
- **Combined image config is baked + env-driven (deploy-anywhere, #71).** The published image bakes **`backend/config.docker.yaml`** at `/app/config.yaml` (`VIBEXP_CONFIG_FILE` points there): a PRODUCTION-NEUTRAL counterpart to `config.example.yaml` where every operator knob is a `${VAR:-default}` reference, so `docker run -e ENCRYPTION_KEY=... -e DB_HOST=...` configures a container with env alone (one-command experience preserved). Operators mount their own file over `/app/config.yaml` for full control. It is schema-validated **and** load-tested (`config_docker_test.go`). The dev-login bypass / auto-enabled local MCP are gated by `IsLocalDevelopment()` (localhost `frontend.base_url`), so `FRONTEND_BASE_URL` is the real production switch — not a separate `DEV_LOGIN_ENABLED` env. `docker-compose.yml` sets secrets + non-default knobs as env consumed by those `${VAR}` references (no flat env the loader can't read).
- **Embedded MCP Authorization Server is HTTPS-only.** The AS endpoints (`/oauth2/*`, `/.well-known/oauth-authorization-server`) are wrapped by `requireHTTPSMiddleware` (`internal/server/middleware_https.go`) in `setupOAuthASRoutes`: a non-HTTPS request (no `r.TLS`, `X-Forwarded-Proto != https`) is rejected with 403 unless `Config.IsLocalDevelopment()`. VibeXP is deploy-anywhere, so this assumes TLS terminates upstream and the proxy forwards `X-Forwarded-Proto: https` (document this for self-hosters). Local dev + the e2e stack (localhost) are exempt.
- **No service worker.** No PWA/precaching SW (only on-demand `firebase-messaging-sw.js` for push). `src/utils/serviceWorker.ts` + `public/{sw,dev-sw}.js` evict legacy workers. Don't reintroduce a precaching SW without a cleanup story.
- **No frontend telemetry.** The frontend bundles no error-tracking/maintainer telemetry (Sentry was removed in #58). `ErrorBoundary` and other error paths log via `console.error` only, and the build emits no source maps (`vite.config.ts` `build.sourcemap: false`). A self-hoster wanting error tracking can wire their own.
- **chi hands back percent-encoded path params — decode with `url.PathUnescape`, never `QueryUnescape` (#251/#257).** chi routes on `r.URL.RawPath` whenever the request path contains percent-encoding (`go-chi/chi/v5` mux), so `chi.URLParam` returns the **still-encoded** segment. Any path param that can carry reserved characters (invitation/share tokens, `slug` — generated from user names so a literal `+` is plausible — anything non-opaque) must be explicitly decoded before use, or an exact-match lookup misses on every request (the #251 invitation-accept bug). Decode with **`url.PathUnescape`**, not `url.QueryUnescape`: the two differ only on `+`, which `QueryUnescape` turns into a space (correct for query strings, silently corrupting for a path segment). Keep the existing 400-on-decode-error behavior. Opaque tokens you mint should be **unpadded** URL-safe by construction (`base64.RawURLEncoding`) so there is nothing to encode in the first place. Pure-UUID params can't contain reserved chars and don't need decoding.
- **Secrets.** `.env` is gitignored, never commit it; only `.env.example` is tracked, with neutral values (e.g. `example.com`) since this is public and self-hosted.

## CI & releases

- CI runs the same `make` targets, so run them locally first. Backend: download-deps -> format -> build -> test, plus lint and OpenAPI validation. Frontend: install -> lint -> type-check -> test -> build. (Backend CI's `make backend-build` is the default no-embed build, which also guards "the backend builds with no `frontend/dist`".)
- **One artifact, one workflow.** A GitHub Release `vX.Y.Z` (workflow `release.yml`) builds the single combined image `ghcr.io/vibexp/vibexp:X.Y.Z` (+ `:latest`, which docker-compose tracks) — the Dockerfile (`backend/Dockerfile`, built from the repo root) builds the frontend, embeds `dist`, and builds the Go binary with `-tags embedfrontend`. There is no separate frontend image.

## Working agreement

- Branch off `main`, match existing style, run the relevant `make ...-lint` / `...-test` / build targets before committing, open a PR, let CI pass.
- **Branch naming:** `feature|patch|minor/{GITHUB_ISSUE}-{slug}` — prefix is one of `feature` (new functionality), `minor` (small improvements), `patch` (fixes/docs/chores); `{GITHUB_ISSUE}` is the number of the open issue the PR is linked to (every PR must link one); `{slug}` is a short kebab-case description. Example: `feature/416-stale-issue-automation`.
- Pre-commit hooks are mandatory (see above); never use `--no-verify`.
- Don't commit secrets or gitignored generated artifacts.
