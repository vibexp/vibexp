---
name: release
description: >-
  Cut a VibeXP release end to end — preflight checks, trigger + wait on the CI
  E2E suite, generate curated release notes, publish the GitHub Release (which
  builds & pushes the combined image), then post-release run two tracks in
  parallel — smoke-test the published image locally AND sync the vibexp/docs
  site to the new version — and hand back a test URL plus the docs PR. Use when
  asked to "create a release", "cut vX.Y.Z", "do a release", or "release VibeXP".
---

# VibeXP Release

Drive a full VibeXP release from one invocation. VibeXP ships as a **single
combined image** `ghcr.io/vibexp/vibexp` (Go backend embeds + serves the SPA);
a published GitHub Release with a `vX.Y.Z` tag triggers `release.yml`, which
builds and pushes `:X.Y.Z` (+ `:latest` for non-prereleases).

## Repo-specific facts (do not re-derive)

- **Release tag scheme:** `vX.Y.Z` (e.g. `v0.3.0`). NOT `backend-v*`/`frontend-v*`
  (that was the pre-combined-image scheme; those old tags still exist for compare links).
- **Release workflow:** `.github/workflows/release.yml`, trigger `release: published`
  on a `v*` tag. Pushes `ghcr.io/vibexp/vibexp:<version>` and `:latest`.
- **E2E workflow:** `.github/workflows/ci-e2e.yml` — on-demand only
  (`workflow_dispatch`, input `branch`). Not wired to PRs. Must be triggered and
  awaited explicitly (this is a required pre-release gate).
- **Fast CI:** one consolidated `ci.yml` workflow named **`CI`** (backend +
  frontend + Sonar in one run, #390/#391) runs on push/PR — NOT the old split
  `ci-backend.yml`/`ci-frontend.yml`.
- **Docs site:** `vibexp/docs` (sibling checkout `../docs`) tracks the latest
  published release, not `main`. Its own `update-docs` skill
  (`../docs/.claude/skills/update-docs/SKILL.md`) drives the sync and records the
  last-synced core version in `../docs/.vibexp-release`.
- **Compose for self-host / smoke test:** root `docker-compose.yml` tracks `:latest`.

## Inputs

- **version** (e.g. `0.3.0`, with or without leading `v`). If not given, propose
  the next version from the last `v*` tag and confirm with the user.
- **release ref** — default `main`. The release is cut from this branch's tip.

## Procedure

Track the phases with the task tools. Do NOT publish anything until the user
approves the notes (Phase 3). Never bypass a failing gate — fix or stop.

### Phase 0 — Preflight

1. Confirm the release ref (`main`) is **clean** and **synced with origin**:
   `git rev-parse HEAD` == `git rev-parse origin/main` (fetch first), `git status -s` empty.
2. Resolve the version. Normalize to `X.Y.Z` (strip a leading `v`); the tag is `vX.Y.Z`.
   Ensure the tag does not already exist: `git tag -l vX.Y.Z` and `gh release view vX.Y.Z`
   must both be empty/not-found. If it exists, STOP.
3. Determine the previous release tag for the changelog range
   (`git tag --sort=-creatordate | grep -E '^v?[0-9]' | head`). For the first
   `v*` release, use the newest `backend-v*` tag as the compare base.

### Phase 1 — Pre-release checks (E2E gate)

1. Verify the fast CI is green on the release commit:
   `gh run list --branch main -L 8 --json workflowName,conclusion,status,headSha`
   — the consolidated **`CI`** workflow must be `completed`/`success` on HEAD
   (it may still be `in_progress` right after the last merge; watch it to
   success before proceeding). Ignore the repo-hygiene workflows (`Stale issue
   lifecycle`, `Project board hygiene`) — they are not build/test gates and
   `Project board hygiene` fails until an org admin grants the Projects scope.
2. **Trigger the E2E suite and wait for it to pass** (required):

   ```bash
   gh workflow run ci-e2e.yml -f branch=main
   ```

   Then find the run it created and watch it to completion. The dispatch does not
   return a run id, so poll for the newest `ci-e2e.yml` run created just after
   dispatch:

   ```bash
   sleep 6
   RUN_ID=$(gh run list --workflow ci-e2e.yml -L 1 --json databaseId --jq '.[0].databaseId')
   gh run watch "$RUN_ID" --exit-status --interval 20
   ```

   `gh run watch --exit-status` exits non-zero if the run fails — if it fails,
   STOP, surface the Playwright report / failing job, and do not release.
   (The suite can take ~15–30 min; be patient, keep watching.)

### Phase 2 — Generate release notes

1. Collect the changes since the previous tag:
   `git log --oneline <prev-tag>..HEAD` and, for PR context,
   `gh pr list --state merged --base main -L 50 --json number,title,mergedAt`.
2. Write **curated, categorized** notes to
   `<scratchpad>/release-notes-vX.Y.Z.md`. Structure:
   - A 1–2 sentence summary lead.
   - **⚠️ Breaking changes & migration notes** — pull out anything that changes
     how self-hosters deploy/configure (config format, tag scheme, removed
     integrations, auth changes). This matters most; do not bury it.
   - **✨ Features**, **🐛 Fixes**, **🔧 Chores & infra** — grouped, each line
     ending with its PR number `(#NN)`.
   - **🐳 Image** block with `docker pull ghcr.io/vibexp/vibexp:X.Y.Z` and `:latest`.
   - **Full changelog** compare link `…/compare/<prev-tag>...vX.Y.Z`.
   Do NOT just dump PR titles — categorize and lift out migration impact.
3. Show the notes to the user and get explicit approval before publishing.

### Phase 3 — Publish the release

1. On approval, create the release from the branch (NOT a raw SHA — the GitHub
   API rejects `target_commitish` as a bare SHA with
   `Release.target_commitish is invalid`; use the branch name, whose tip is the
   release commit):

   ```bash
   gh release create vX.Y.Z --target main --title "vX.Y.Z" \
     --notes-file "<scratchpad>/release-notes-vX.Y.Z.md"
   ```

   Add `--prerelease` for pre-releases (they will NOT get `:latest`).
2. Watch the triggered `release.yml` run to success:

   ```bash
   sleep 8
   RID=$(gh run list --workflow release.yml -L 1 --json databaseId --jq '.[0].databaseId')
   gh run watch "$RID" --exit-status --interval 15
   ```

3. Verify the image tags landed in GHCR:

   ```bash
   for t in X.Y.Z latest; do
     docker manifest inspect ghcr.io/vibexp/vibexp:$t >/dev/null 2>&1 \
       && echo "OK   :$t" || echo "MISS :$t"
   done
   ```

### Phase 4 — Post-release: smoke test + docs sync (in parallel)

Once the image is published, run **two independent tracks concurrently** and
report both when they finish:

- **Track A — smoke test** the published image (below).
- **Track B — docs sync**: bring `vibexp/docs` up to the new release (below).

The two tracks touch different repos and never conflict, so kick them off
together (e.g. launch Track B via a background Agent, or interleave the steps)
rather than serially. Track A ends with a live URL; Track B ends with an
unmerged docs PR. The release is not "done" until both are reported.

#### Track A — smoke test

Run the **published** image (pinned to the new version) in an isolated compose
project (separate name + fresh volume so it never touches dev data), wait for
health, curl the key surfaces, then hand the user a URL.

1. Write `<scratchpad>/smoke-vX.Y.Z.yml` from the template below (bump the image
   tag and the `pgdata`/project name to the version).
2. `docker compose -f <scratchpad>/smoke-vX.Y.Z.yml up -d`
3. Poll `docker inspect -f '{{.State.Health.Status}}' vibexp-smokeXYZ-app-1`
   until `healthy` (migrations run on boot); tail app logs to confirm
   "Database migrations completed" and "Authorization Server enabled".
4. Smoke the HTTP surfaces (all must be `200`):
   `/ping`, `/` (SPA index — expect `<title>VibeXP`, `<div id="root">`,
   `__VIBEXP_ENV__`), `/config.js`, a SPA client route like `/prompts`
   (catch-all → index), `/api/v1/auth/providers`,
   `/.well-known/oauth-authorization-server`.
5. Report a results table and give the user **http://localhost:8080** to test.
   `FRONTEND_BASE_URL=http://localhost:8080` puts it in local mode (dev-login
   bypass on), so they can sign in without configuring a provider.
6. Offer teardown: `docker compose -f <scratchpad>/smoke-vX.Y.Z.yml down -v`.

#### Smoke compose template

```yaml
name: vibexp-smokeXYZ            # replace XYZ, e.g. smoke030
services:
  postgres:
    image: pgvector/pgvector:pg16
    restart: unless-stopped
    environment:
      POSTGRES_DB: vibexp
      POSTGRES_USER: vibexp
      POSTGRES_PASSWORD: smoke-postgres-password
    volumes: [pgdata:/var/lib/postgresql/data]
    networks: [vibexp]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -d vibexp -U vibexp"]
      interval: 5s
      timeout: 5s
      retries: 10
  app:
    image: ghcr.io/vibexp/vibexp:X.Y.Z     # pin to the released version
    restart: unless-stopped
    ports: ["8080:8080"]
    environment:
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: vibexp
      DB_PASSWORD: smoke-postgres-password
      DB_NAME: vibexp
      ENCRYPTION_KEY: "changemechangemechangeme32bytes!"
      SESSION_ENCRYPTION_KEY: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
      EMBEDDING_MODEL: "gemini-embedding-001"
      FRONTEND_BASE_URL: "http://localhost:8080"   # localhost => local mode / dev-login
      LOG_LEVEL: info
    depends_on:
      postgres: { condition: service_healthy }
    networks: [vibexp]
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/ping"]
      interval: 5s
      timeout: 5s
      retries: 10
      start_period: 15s
volumes: { pgdata: {} }
networks: { vibexp: { driver: bridge } }
```

#### Track B — docs sync

Bring the documentation site up to the release you just published. The docs
site (`vibexp/docs`) **tracks the latest published release, never `main`**, so
a fresh `vX.Y.Z` is exactly when it needs syncing. This runs concurrently with
Track A.

1. Ensure the docs checkout exists as a sibling (`../docs`, relative to this
   repo). If missing, clone it:
   `git clone https://github.com/vibexp/docs.git ../docs`.
2. Delegate the whole sync to the docs repo's own **`update-docs`** skill
   (`../docs/.claude/skills/update-docs/SKILL.md`) scoped to `core`. That skill
   owns the real work: it audits every in-scope doc page against the vibexp
   source **at the `vX.Y.Z` tag** with file:line evidence, fixes/adds/removes
   content, bumps `../docs/.vibexp-release` to the new tag, validates the build
   (`npm run build/lint/typecheck/test`), opens a PR, and runs its review loop.
   Do not re-implement that flow here — invoke it. (Prefer running it as a
   background Agent so Track A proceeds in parallel; the audit + review loop can
   take a while.)
   - **Source-tree note:** the freshly published `vX.Y.Z` tag == current `main`
     HEAD, so if the sibling `../vibexp` checkout is clean on `main` at that
     commit, its working tree already *is* the v-tag source — the audit can read
     it in place with no `git checkout` (which avoids disturbing a running
     hot-reload dev server). Only check out the tag if the checkout has drifted.
3. Per its own hard rule, `update-docs` **stops at an unmerged, review-approved
   PR** — never merge it here. The human merges the docs PR separately.

If the docs are already in sync (marker already at `vX.Y.Z`), `update-docs`
reports that and makes no PR — pass that through.

## Guardrails

- STOP (do not publish) if: working tree dirty / not synced, fast CI not green,
  the **E2E run fails**, the tag/release already exists, or the user has not
  approved the notes.
- Never `git commit/push --no-verify`. This skill makes **no commit to
  `vibexp/vibexp`** — it operates on an already-merged `main`. The only writes
  it produces are in Track B, and those land as an **unmerged PR in
  `vibexp/docs`** (opened by the delegated `update-docs` skill).
- **Never merge the docs PR.** Track B ends at a review-approved, unmerged PR;
  the human merges it. Do not merge even with admin rights.
- The isolated smoke project must use a version-specific name + its own volume;
  never reuse the self-host `docker-compose.yml` project or its data.
- Keep temp files (notes, smoke compose) in the scratchpad, not the repo.
- The release is complete only after **both** tracks are reported: the smoke-test
  URL (Track A) and the docs PR URL / "already in sync" (Track B).
