---
name: verify
description: >-
  Verify a VibeXP feature end to end against a locally running stack. Takes the
  feature to verify as an argument (an issue/PR number, an endpoint, or a plain
  description); asks the user which feature to verify when none is given.
  Decides backend-only vs full-stack, checks prerequisites first, starts what
  is needed, bootstraps auth via dev login + API key, executes a concrete
  checklist, and reports pass/fail with evidence. Use when asked to "verify
  <feature>", "test this end to end", or to confirm a merged change actually
  works in the running app.
argument-hint: [feature to verify — issue/PR number, endpoint, or description]
---

# Verify a feature end to end

Drive a real verification of one feature against the local dev stack. The
outcome is a pass/fail report with evidence, not a code review — read code only
to learn what "working" means, then prove it against the running system.

## Phase 1 — Pin down the target

- If an argument was given, resolve it: an issue/PR number → `gh issue view` /
  `gh pr view` (+ diff) to extract the intended behavior; an endpoint or
  feature name → find it in `backend/openapi.yaml` (`paths/`, `schemas/`) and
  the relevant `internal/services`/`internal/server` code, or
  `frontend/src` for UI features.
- If no argument was given, or it is too vague to derive concrete expectations,
  use the **AskUserQuestion** tool: which feature, and (if unclear) what
  observable behavior counts as success. Offer the most recently merged PRs'
  features as options when that helps.
- Write down a **concrete checklist** of observable behaviors before touching
  anything: happy path(s), key validation failures, authorization boundaries,
  and — for documented endpoints — response-shape conformance with the spec.
  This checklist is the contract for the rest of the run.

## Phase 2 — Decide scope: backend-only or full-stack

Most features are verifiable through the REST API alone — services, endpoints,
importers, MCP tools, migrations, background jobs. Prefer **backend-only**: it
is faster and every behavior is authorizable server-side anyway. Include the
frontend only when the feature *is* UI (a page, component, or flow in
`frontend/src`) or the user explicitly wants it seen in the browser; then
verify UI via the Chrome browser tools on top of the API checks.

## Phase 3 — Prerequisites and blockers (stop, don't improvise)

Check before starting anything; on a missing prerequisite STOP and tell the
user exactly what is missing and how to move forward:

- **Toolchain:** `docker`, `make`, `air` (backend), `node`/npm deps installed
  (`make frontend-install`) if the frontend is needed.
- **Already running?** Check `http://localhost:8080/health` and
  `http://localhost:5173` first — the user often runs the dev servers
  themselves; never start a second copy on a busy port.
- **Feature-specific configuration.** Derive from the feature what
  `backend/config.yaml` / `backend/.env` must contain (all keys are documented
  in `backend/config.example.yaml` / `.env.example`). Examples: GitHub features
  need the GitHub App keys configured (see the `verify-github-integration`
  skill, which covers that domain end to end); attachments need
  `storage.attachments_bucket`; email flows use Mailpit (started by
  `make backend-run-dev`, UI on :8025). Check presence of secrets, never print
  them, and never ask the user to paste them into chat — they belong in the
  gitignored `.env`/`config.yaml`.
- **Dev-DB schema sanity.** If the feature touched migrations, confirm the
  expected columns/tables actually exist (`docker exec backend-postgres-1 psql
  -U vibexp_app -d vibexp_io ...`). A dev DB migrated before a migration
  renumbering can report an up-to-date `schema_migrations` while missing DDL;
  the fix (with user consent) is deleting the `backend_postgres_data` volume
  (keep `backend_tei_data` — ~670MB embeddings model) and restarting.

## Phase 4 — Start what's needed

- Backend: `make backend-run-dev` in the background (starts Postgres, Mailpit,
  embeddings + air hot-reload); poll `/health` until up. Migrations run at
  boot.
- Frontend (only if Phase 2 said so): `make frontend-run-dev` in the
  background; poll `http://localhost:5173`.
- Verify uncommitted changes simply by having them in the checkout air/Vite
  watch — but remember config.yaml/.env changes need a backend restart (they
  are read at boot, air only watches `.go` files).

## Phase 5 — Auth bootstrap (backend-only needs no browser)

- **Dev login works with any email**: `POST /api/v1/auth/dev/login` with
  `{"email":"<anything>@test.com","name":"..."}` creates/fetches the user and
  sets a session cookie. Quirk: on a fresh DB the first call may return
  `default_team_id: null` — call it twice. This also means multi-user
  scenarios (roles, invitations, authz boundaries) are easy: dev-login several
  emails and act as each in turn.
- **API key**: with the session cookie, `POST /api/v1/api-keys` with
  `{"name":"verify","integration_codes":["cli","ai_tools"]}` → use the
  returned `full_key` as `Authorization: Bearer` for everything. Mint fresh —
  keys from previous databases fail with `AUTH_INVALID`.
- The user id / `default_team_id` from the login response are the team-scoped
  path parameters most endpoints need.
- Frontend login: the sign-in page shows a **Development login** card in local
  dev — same email, no provider needed.

## Phase 6 — Execute the checklist

- Run every item from Phase 1 against the live system; capture evidence as you
  go: response bodies for API checks, screenshots for UI checks, and backend
  log lines (the `make backend-run-dev` output) for anything that 500s.
- Include the unhappy paths: expected 400s with meaningful validation
  messages, 401 without auth, 403/404 across team boundaries (verify with a
  second dev-login user), 409 on documented conflicts.
- For documented endpoints, sanity-check the response shape against
  `backend/openapi.yaml` — response conformance is progressive (#122) and
  drift ships silently, so a shape mismatch is a real finding, not noise.
- If a targeted Go/frontend test suite exists for the feature, run it too
  (`go test ./internal/<pkg>/ -run <Feature> -count=1`, `make frontend-test`)
  — it is corroborating evidence, not a substitute for the live check.
- If an item fails, keep going through the remaining items (collect all
  findings), then investigate the failures with the backend logs before
  reporting.

## Phase 7 — Report and clean up

- Report per-checklist-item pass/fail with the evidence, leading with the
  overall verdict. Distinguish "feature broken" from "environment/prerequisite
  problem" explicitly.
- Offer cleanup: the run's created users/teams/resources/API keys stay in the
  dev DB — either delete what was created via the API, or (user consent) reset
  the DB volume for a clean slate. Leave servers in the state you found them:
  if the user's own dev servers were already running, do not stop them; stop
  only what this run started, and say so.
