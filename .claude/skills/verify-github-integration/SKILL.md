---
name: verify-github-integration
description: End-to-end verification of the GitHub App integration against a locally running VibeXP backend. Currently covers blueprint import (epic #334) — import fidelity, provenance, multi-file skill companions, re-import semantics, and path lifecycle. Use when asked to verify the GitHub integration, after changing the importer or blueprint-sync code, or before a release that touches GitHub features.
---

# Verify GitHub integration (blueprints)

Run the live verification that closed epic #334 against a local backend and a
real GitHub App. Everything is driven through the public REST API — no browser
is required, including the App-to-team connection.

Work through the phases in order. Report a per-phase pass/fail summary at the
end; on any failure, include the failing assertion and the relevant backend log
lines (`Failed to import blueprint` entries carry the underlying error).

## Phase 0 — Prerequisites (check ALL before doing anything else)

Stop and tell the user exactly what is missing rather than improvising:

1. **GitHub App configuration present.** The backend needs a GitHub App the
   user controls (a dedicated test App is fine). Check — values must be
   non-empty, do not print them:
   - `backend/config.yaml`: `github.app_id` and `github.app_slug`
   - `backend/.env`: `GITHUB_APP_PRIVATE_KEY` (base64-encoded PEM) and
     `GITHUB_WEBHOOK_SECRET`

   If any are missing, STOP and ask the user to supply their test App's
   credentials. Never ask them to paste secrets into the chat or commit them;
   they belong only in the gitignored `backend/.env` / `config.yaml`.
2. **The App is installed somewhere useful.** The installation must cover at
   least one repository containing AI config files (`CLAUDE.md`, `.claude/`,
   `.cursor/`, `AGENTS.md`). Ideally one repo has a **multi-file Agent Skill**
   (a `.claude/skills/<name>/` directory with `SKILL.md` plus companion files,
   nested subdirectories being a bonus) — that is what exercises the
   companion-import path. Phase 2 discovers this via the API; if no suitable
   repo exists, ask the user to pick or create one.
3. **Toolchain:** `docker`, `air`, `make`, `python3`, `openssl` on PATH, and
   `pre-commit` if you will end up committing anything.
4. **Backend up and healthy:** `make backend-run-dev` (run it in the
   background), then poll `http://localhost:8080/health`.
5. **Dev-DB schema sanity.** A dev database migrated before a migration
   renumbering can claim to be current while missing columns. Verify:

   ```sh
   docker exec backend-postgres-1 psql -U vibexp_app -d vibexp_io -tAc \
     "select count(*) from information_schema.columns where table_name='blueprints' \
      and column_name in ('path','raw_content','content_sha','source_repo', \
      'source_commit_sha','source_blob_sha','imported_at','source_content_sha');"
   ```

   Expect `8`. If not (symptom during import: `column "path" of relation
   "blueprints" does not exist`), the dev DB predates the current migration set
   — with the user's consent, stop the stack, delete the `backend_postgres_data`
   volume (keep `backend_tei_data`; it holds the ~670MB embeddings model), and
   restart so migrations run from scratch.

## Phase 1 — Session and API key

- Dev login: `POST /api/v1/auth/dev/login` with
  `{"email":"test@test.com","name":"Test User"}` (note: `dev/login`, not
  `dev-login`). On a **fresh** DB the first response may have
  `default_team_id: null` — call it twice and use the second response. Capture
  the session cookie and `default_team_id` (the team for everything below).
- Mint an API key with the cookie: `POST /api/v1/api-keys` with
  `{"name":"github-verify","integration_codes":["cli","ai_tools"]}` →
  `full_key` is the bearer token for all later calls. Do not rely on any
  pre-existing key from the environment; stale keys fail with `AUTH_INVALID`.

## Phase 2 — Connect the App to the team (headless)

1. Mint an App JWT (RS256, `iss` = app id, ~9 min expiry) from the configured
   app id + private key. `openssl dgst -sha256 -sign` over
   `base64url(header).base64url(payload)` is enough — no libraries needed.
2. `GET https://api.github.com/app/installations` with the JWT → pick the
   installation; note its numeric `id`. Mint an installation token
   (`POST /app/installations/{id}/access_tokens`) — used later to fetch ground
   truth from GitHub.
3. `GET /api/v1/{team_id}/integrations/github/install-url` and extract the
   `state` query parameter from the returned URL (it is HMAC-signed
   server-side; you cannot fabricate it).
4. `POST /api/v1/{team_id}/integrations/github/callback` with
   `{"installation_id": <id>, "state": "<state>"}`.
5. `GET .../integrations/github/status` must now report `installed: true`.

## Phase 3 — Import

1. `GET .../integrations/github/repositories`; choose the target repo
   (prefer one with a multi-file skill; confirm with the user if ambiguous).
2. `POST .../integrations/github/repositories/{repo_id}/import-project`.
3. `POST .../integrations/github/import-blueprints` with
   `{"repository_id": <repo_id>}` and keep the returned
   `BlueprintImportReport`.

Report sanity: `total_failed == 0`; non-markdown files appear in
`skipped_items` with a reason; every companion file of each skill appears in
`companion_items` — with attachment storage unconfigured (default local dev)
each is `skipped` with reason "Attachment storage is not configured", which is
the documented behavior, not a failure. If storage *is* configured, companions
must import as attachments keyed by `relative_path` (subdirectory paths like
`templates/x.md` preserved).

## Phase 4 — Fidelity assertions (every successful item)

Blueprint detail is `GET /api/v1/{team_id}/blueprints/{project_id}/{slug}`
(map report `blueprint_id`s to slugs via the list endpoint,
`GET /api/v1/{team_id}/blueprints?project_id=...`). Fetch GitHub ground truth
with the installation token: raw file bytes (`Accept:
application/vnd.github.raw+json` on the contents API), branch head SHA
(`GET .../branches/{default}`), and the recursive git tree for blob SHAs.

For each imported blueprint assert:

- `path` == the repo-relative file path from the report, verbatim.
- `raw_content` is **byte-identical** to the GitHub file.
- `content_sha` == SHA-256 hex of `raw_content`.
- `source.repo` is the repository URL; `source.commit_sha` == branch head;
  `source.blob_sha` == that path's blob in the git tree; `source.imported_at`
  set. (`source` is server-set — no request field can influence it.)
- The **list** response omits `raw_content`; the detail response includes it.

## Phase 5 — Re-import semantics

1. Re-import unchanged → every item `up_to_date`; nothing rewritten.
2. Edit one imported blueprint through the API (append a marker line), then
   re-import → still `up_to_date` (upstream unchanged is up-to-date even if
   locally edited — by design), and the local edit survives.
3. Simulate an upstream change by perturbing stored provenance for two
   blueprints — one edited, one untouched:

   ```sh
   docker exec backend-postgres-1 psql -U vibexp_app -d vibexp_io -c \
     "UPDATE blueprints SET source_blob_sha = repeat('0',40) WHERE slug IN ('<edited>','<untouched>');"
   ```

   Re-import → the untouched one is `updated` (raw + provenance refreshed,
   real blob SHA restored, id/slug preserved); the edited one is a `conflict`
   ("edited in VibeXP; re-import skipped...") and keeps the local edit and the
   perturbed provenance untouched.

## Phase 6 — Path lifecycle (VibeXP-authored)

Create via `POST /api/v1/{team_id}/blueprints` (body needs `project_id`;
`content` is the **body only** — frontmatter-style metadata goes in the
`metadata` field, or it ends up duplicated in the regenerated raw):

- New `type: claude-code, subtype: skills` blueprint without `path` → derived
  default `.claude/skills/<slug>/SKILL.md`; the regenerated `raw_content`
  frontmatter has `name:` synced to the directory name and preserves nested
  metadata (maps/lists) faithfully.
- Slug change on a derived-path blueprint → path recomputes.
- Explicit `path` on create → accepted and **frozen** across slug changes.
- Invalid paths rejected 400: `../evil.md`, `/abs.md`, `a\b.md`, `./rel.md`,
  `a/../../etc/passwd`. Duplicate path in the same project → 409.
- Version restore: `POST .../versions/{n}/restore` reverts content and keeps
  `raw_content` populated.

## Phase 7 — Test suites

From `backend/`: `go test ./internal/services/ -run Materializer -count=1` and
`go test ./internal/blueprintpath/ -count=1` must pass (byte-identical
materializer round-trip and the path-mapping property tests).

## Cleanup

Offer to remove what the run created: the imported project + blueprints, the
lifecycle-test blueprints, and the `github-verify` API key — or reset the dev
DB entirely (same volume procedure as Phase 0.5) if the user prefers a clean
slate. Leave the App configuration in place; it is reusable.
