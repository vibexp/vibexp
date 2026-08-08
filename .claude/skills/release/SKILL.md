---
name: release
description: >-
  Cut a VibeXP release end to end — preflight checks, trigger + wait on the CI
  E2E suite, generate curated release notes, publish the GitHub Release (which
  builds & pushes the combined image), then post-release run three tracks in
  parallel — smoke-test the published image locally, sync the vibexp/docs site,
  and verify vibexp/cli compatibility (e2e + gap-analysis issues) — and hand back
  a test URL, the docs PR, and the CLI verdict. Handles both a normal release
  cut from main and a hotfix/patch release cut from a release/X.Y.x line branch.
  Use when asked to "create a release", "cut vX.Y.Z", "do a release", "release
  VibeXP", or "cut a hotfix/patch release".
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
  on a `v*` tag. Pushes `ghcr.io/vibexp/vibexp:<version>`, plus `:latest` when
  the version is the **highest published non-prerelease semver** — not merely
  when it is not a prerelease (#747), so a backport never drags `:latest`
  backwards onto an older image.
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
- **CLI compat:** `vibexp/cli` (sibling `../cli`) ships a self-contained e2e job
  (`.github/workflows/ci.yml`, job `e2e`) with a `workflow_dispatch` input
  `platform_image_tag` that boots `ghcr.io/vibexp/vibexp:<tag>` and runs the CLI
  against it. Track C verifies the latest CLI *release* (not `main`) this way, and
  files CLI catch-up issues. Publishing a release **auto-dispatches** that e2e —
  `release.yml` job `dispatch-cli-e2e` (#448) — and links the run in its summary.
- **Compose for self-host / smoke test:** root `docker-compose.yml` tracks `:latest`.

## Inputs

- **version** (e.g. `0.3.0`, with or without leading `v`). If not given, propose
  the next version from the last `v*` tag and confirm with the user.
- **release ref** (`$REF` below) — default `main`. The release is cut from this
  branch's tip. A `release/X.Y.x` ref selects **hotfix mode** (see below).

## Two modes

Everything below is written in terms of `$REF`. Resolve it FIRST — several
phases behave differently, and reading `main` when the release is a hotfix is
the failure this parameterization exists to prevent (issue #747).

| | **Normal release** | **Hotfix / patch release** |
|---|---|---|
| `$REF` | `main` | `release/X.Y.x` |
| Version bump | minor (`0.9.0` → `0.10.0`) | patch (`0.9.0` → `0.9.1`) |
| Contents | everything merged since the last tag | cherry-picks only |
| May touch spec / migrations | yes | **no** — see the Phase 0 gate |
| Docs source tree | `main` HEAD == the tag | **must check out the tag** |

**Hotfix mode assumes the branch already exists and already carries the
cherry-picks.** Creating it is a human step done before invoking this skill:

```bash
git fetch --tags origin
git checkout -b release/0.9.x v0.9.0     # lazily, first patch on this line only
git push -u origin release/0.9.x
# then: PR each `git cherry-pick -x <sha-from-main>` into release/0.9.x
```

Fixes land on `main` FIRST and are cherry-picked from there — never the reverse.
If asked to hotfix a fix that is not yet on `main`, stop and say so: a fix that
exists only on the release line is one the next minor silently regresses.

## Procedure

Track the phases with the task tools. Do NOT publish anything until the user
approves the notes (Phase 3). Never bypass a failing gate — fix or stop.

### Phase 0 — Preflight

1. Confirm `$REF` is **clean** and **synced with origin**: `git rev-parse HEAD`
   == `git rev-parse origin/$REF` (fetch first), `git status -s` empty. Make
   sure the checkout is actually ON `$REF` — a hotfix cut while sitting on
   `main` builds the wrong tree.
2. Resolve the version. Normalize to `X.Y.Z` (strip a leading `v`); the tag is `vX.Y.Z`.
   Ensure the tag does not already exist: `git tag -l vX.Y.Z` and `gh release view vX.Y.Z`
   must both be empty/not-found. If it exists, STOP.
3. Determine the previous release tag for the changelog range
   (`git tag --sort=-creatordate | grep -E '^v?[0-9]' | head`). For the first
   `v*` release, use the newest `backend-v*` tag as the compare base. In hotfix
   mode the compare base is the previous tag **on this line** (`v0.9.0` for
   `v0.9.1`), not whatever tag is newest overall.
4. **Hotfix mode only — three gates. Any failure STOPS the release.**

   a. **No spec or migration changes.** Both are structural, not stylistic: a
      spec change auto-publishes both API clients off `main` merges only, and a
      migration on a side line permanently forks the schema lineage for
      self-hosters. If either fires, this is a **minor**, not a patch — say so
      and stop.

      ```bash
      git diff --name-only "$PREV_TAG..HEAD" \
        | grep -E '^backend/(openapi\.yaml|paths/|schemas/|migrations/)' \
        && echo "STOP: patch releases must not touch the spec or migrations"
      ```

   b. **Every commit on the line exists on `main`.** A `+` line means a fix
      lives only here, and the next minor will regress it. Report the offending
      commits and stop until they are forward-ported.

      ```bash
      git fetch --no-tags origin main:refs/remotes/origin/main
      git cherry origin/main HEAD      # expect: no lines starting with '+'
      ```

   c. **The branch is the previous tag plus cherry-picks — nothing else.**
      Skim `git log --oneline "$PREV_TAG..HEAD"`; every commit should be a
      deliberate backport. An unexpected merge commit means someone merged
      `main` into the release line, which defeats the whole point — stop and
      surface it.

### Phase 1 — Pre-release checks (E2E gate)

1. Verify the fast CI is green on the release commit:
   `gh run list --branch "$REF" -L 8 --json workflowName,conclusion,status,headSha`
   — the consolidated **`CI`** workflow must be `completed`/`success` on HEAD
   (it may still be `in_progress` right after the last merge; watch it to
   success before proceeding). Ignore the repo-hygiene workflows (`Stale issue
   lifecycle`, `Project board hygiene`) — they are not build/test gates and
   `Project board hygiene` fails until an org admin grants the Projects scope.
   On a `release/**` push the `Sonar Scan` job is **skipped by design** (#747 —
   main is the coverage baseline, a release line is not); a skipped Sonar job
   does not make the CI run anything other than green, so do not chase it.
2. **Trigger the E2E suite and wait for it to pass** (required):

   ```bash
   gh workflow run ci-e2e.yml -f branch="$REF"
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
   `gh pr list --state merged --base "$REF" -L 50 --json number,title,mergedAt`.
   In hotfix mode the list is short by construction — a handful of cherry-picks.
   Cite the **original** `main` PR number for each (the `cherry picked from
   commit <sha>` trailer that `-x` leaves is how you find it), not the backport
   PR: the original is where the discussion lives.
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
   gh release create vX.Y.Z --target "$REF" --title "vX.Y.Z" \
     --notes-file "<scratchpad>/release-notes-vX.Y.Z.md"
   ```

   Add `--prerelease` for pre-releases (they will NOT get `:latest`).
   `release.yml` moves `:latest` only when this version is the **highest
   published semver** (#747), so a backport — v0.9.1 published after v0.10.0 is
   already out — deliberately leaves `:latest` alone. That is correct, not a
   failure: check the workflow's job summary, which states the decision and why,
   and carry it into the report so nobody goes hunting for a missing tag.
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

   `:X.Y.Z` must exist — that is the release. `:latest` is only expected to
   point HERE when this version is the highest published semver; on a backport
   it still resolves, but to the newer image, which is the intended outcome.

### Phase 4 — Post-release: smoke test + docs sync + CLI compat (in parallel)

Once the image is published, run **three independent tracks concurrently** and
report all three when they finish:

- **Track A — smoke test** the published image (below).
- **Track B — docs sync**: bring `vibexp/docs` up to the new release (below).
- **Track C — CLI compatibility**: verify the latest `vibexp/cli` release still
  works against the new platform image, and file follow-up issues for any CLI
  catch-up the release implies (below).

The three tracks touch different repos and never conflict, so kick them off
together (launch B and C via background Agents, or interleave the steps) rather
than serially. Track A ends with a live URL; Track B ends with an unmerged docs
PR; Track C ends with a CLI e2e verdict plus any filed CLI issues. The release
is not "done" until all three are reported.

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
    image: pgvector/pgvector:pg17
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
   - **Source-tree note — normal releases only.** On a release cut from `main`,
     the freshly published `vX.Y.Z` tag == current `main` HEAD, so if the sibling
     `../vibexp` checkout is clean on `main` at that commit, its working tree
     already *is* the v-tag source — the audit can read it in place with no
     `git checkout` (which avoids disturbing a running hot-reload dev server).
     Only check out the tag if the checkout has drifted.
   - **In hotfix mode this shortcut is FALSE and must not be taken.** The tag
     points at the release line, while `main` has moved on — possibly by weeks.
     Reading the working tree in place would audit unreleased `main` code while
     claiming to describe `vX.Y.Z`, which is precisely the "docs describe
     features users cannot run" failure the docs repo's tracking rule exists to
     prevent. Check the tag out explicitly (a worktree keeps any running dev
     server undisturbed):

     ```bash
     git -C ../vibexp fetch --tags origin
     git -C ../vibexp worktree add /tmp/vibexp-vX.Y.Z vX.Y.Z
     ```

     and point the audit at that path. Remove the worktree when Track B ends.
3. Per its own hard rule, `update-docs` **stops at an unmerged, review-approved
   PR** — never merge it here. The human merges the docs PR separately.

If the docs are already in sync (marker already at `vX.Y.Z`), `update-docs`
reports that and makes no PR — pass that through.

#### Track C — CLI compatibility

Verify the **latest `vibexp/cli` release** still works against the new platform
image, and file follow-up issues for CLI catch-up the release implies. Two parts,
both concurrent with Tracks A and B.

**C1 — e2e compatibility run (in CI, never local).** The CLI repo already owns a
self-contained e2e job: `vibexp/cli` `.github/workflows/ci.yml` has an `e2e` job
plus a `workflow_dispatch` input `platform_image_tag` that boots
`ghcr.io/vibexp/vibexp:<tag>` and drives the built CLI against it. **We verify the
released CLI, not `main`.**

- **Default path — the release already dispatched it (#448).** Publishing the
  release runs `release.yml`'s `dispatch-cli-e2e` job (after the image push, via
  `needs: release`), which resolves the latest CLI release tag, dispatches
  `ci.yml` against it with `platform_image_tag=X.Y.Z`, and writes a **`CLI e2e:`
  line with the run URL to that job's summary**. So: open the release run's
  summary, take the link, and watch it —
  `gh run watch <id> --repo vibexp/cli --exit-status`. Read the URL from the
  summary rather than guessing from `gh run list`; the job already did the
  before/after run-id disambiguation for you.
  - Its summary line may instead say **skipped** (the CLI release predates the
    `platform_image_tag` input) — then there is no run to watch. Report that and
    move to C2.
  - If `dispatch-cli-e2e` itself is **red**, its error names the remediation (the
    VibeXP Bot app needs `Actions: read & write` on `vibexp/cli`). The image is
    already published — a red dispatch job never affects the release. Use the
    fallback below for this release, and get the app installed.
- **Fallback (only when the job skipped, went red, or the release was cut by a
  manual `workflow_dispatch`, which deliberately does not dispatch downstream):**
  self-dispatch against the latest CLI release tag:
  ```bash
  CLI_TAG=$(gh release view --repo vibexp/cli --json tagName -q .tagName)
  gh workflow run ci.yml --repo vibexp/cli --ref "$CLI_TAG" -f platform_image_tag=X.Y.Z
  # then poll for the new run and: gh run watch <id> --repo vibexp/cli --exit-status
  ```
- **Guard (required on the fallback path):** `workflow_dispatch --ref <tag>` runs
  the workflow **from that tag**, so if the latest CLI release predates the e2e
  harness/job, the dispatch does nothing useful. Before dispatching, confirm the
  tag carries the job:
  `git -C ../cli show "$CLI_TAG:.github/workflows/ci.yml" | grep -q 'platform_image_tag'`
  (or the API equivalent). **If it is absent, SKIP the e2e run and report it
  clearly** ("latest CLI release `<tag>` predates the e2e harness; compatibility
  e2e skipped") — do NOT silently fall back to `main` (that tests unreleased CLI
  code, not the release). Still do C2. (On the default path the job applies this
  same guard itself.)
- A failing CLI e2e is a **compatibility regression**: report it loudly (link the
  failing run) so a CLI fix can follow. It does NOT roll back the platform release
  (already published) — it is post-release signal.

**C2 — gap analysis + issues (always run).** Independently of C1, assess whether
the new release adds surface the CLI should catch up to, and file issues in
`vibexp/cli` for real gaps so they are tracked for later:

- Compare the release's new/changed API surface and features (from the Phase 2
  notes and the spec diff) against the CLI's curated command coverage
  (`../cli/internal/cli/*cmd/` nouns + `vibexp api` passthrough) and its auth/output
  behavior (`../cli/CLAUDE.md` is the design map).
- File one focused `gh issue create --repo vibexp/cli` per genuine gap (new
  resource nouns/commands, changed auth/response behavior the CLI narrates, new
  read fields worth surfacing). WHAT/WHY/HOW bodies, cite the platform PRs and CLI
  source. Do not over-file: skip anything the raw-JSON passthrough already covers
  with no UX loss.
- These are **follow-up tickets, not blockers** — the release is already out.

Track C ends with: the e2e verdict (passed / failed+link / skipped-with-reason)
and the list of filed CLI issue URLs (or "no gaps").

## Guardrails

- STOP (do not publish) if: working tree dirty / not synced, fast CI not green,
  the **E2E run fails**, the tag/release already exists, or the user has not
  approved the notes.
- **Hotfix mode adds three more STOPs** (Phase 0.4): the diff touches the spec
  or `backend/migrations/`; `git cherry origin/main HEAD` shows a `+` commit;
  or the line carries something other than the previous tag plus cherry-picks.
  Never work around these — each one means the patch is really a minor, or that
  a fix is about to be lost from `main`.
- **Never create the release branch or the cherry-picks yourself.** Those are
  human steps that go through normal PRs and CI; this skill releases what is
  already there.
- Never `git commit/push --no-verify`. This skill makes **no commit to
  `vibexp/vibexp`** — it operates on an already-merged `$REF`. Its only writes are
  an **unmerged PR in `vibexp/docs`** (Track B, via `update-docs`) and **follow-up
  issues in `vibexp/cli`** (Track C). It never edits or merges CLI code.
- **Never merge the docs PR.** Track B ends at a review-approved, unmerged PR;
  the human merges it. Do not merge even with admin rights.
- **Track C tests the CLI *release*, never `main`.** If the latest CLI release
  predates the e2e job, skip the run and say so — do not fall back to `main`.
  A CLI e2e failure is post-release signal (report it, file a fix issue); it does
  not roll back the already-published platform release.
- The isolated smoke project must use a version-specific name + its own volume;
  never reuse the self-host `docker-compose.yml` project or its data.
- Keep temp files (notes, smoke compose) in the scratchpad, not the repo.
- The release is complete only after **all three** tracks are reported: the
  smoke-test URL (A), the docs PR URL / "already in sync" (B), and the CLI e2e
  verdict + filed CLI issues / "no gaps" (C).
