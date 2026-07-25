---
name: run-e2e-tests
description: >-
  Run the VibeXP Playwright end-to-end suite — locally against a from-source
  combined-image stack (`make e2e`) or in CI (`gh workflow run ci-e2e.yml`), or
  both — then triage every failure to a root cause and file one GitHub issue per
  cause. Knows the on-demand-only workflow, the dispatch-then-poll recipe, how to
  rerun a single spec against a live stack, and the stale-spec-vs-real-regression
  call. Use when asked to "run e2e", "run the e2e tests", "trigger e2e on main",
  or to check whether the suite still passes after a batch of merges.
argument-hint: "[ref to test — branch/tag/SHA, default main] [local | ci | both]"
---

# Run the VibeXP E2E suite

Drive the Playwright suite to a verdict and, when it is red, to a set of
**root causes with issues filed** — not a pile of logs. The suite is the only
gate that exercises the artifact we actually ship (Go backend serving the
embedded SPA), so a red run is a real signal even when the fix is test-side.

## Repo-specific facts (do not re-derive)

- **Nothing runs e2e automatically.** `.github/workflows/ci-e2e.yml` is
  `workflow_dispatch`-only (input `branch`), deliberately: it builds the combined
  image from source and boots a full stack, too heavy to gate every PR. The
  consequence is drift — specs break on `main` and stay green-by-absence until
  someone runs this. Expect stale specs after any batch of route/UI refactors.
- **Local and CI run the identical command.** `ci-e2e.yml` just calls `make e2e`,
  so a green local run means a green CI run and vice versa. If they disagree,
  that itself is the finding.
- **What `make e2e` does** (Makefile, `E2E_COMPOSE` / `e2e-*` targets):
  `e2e-browsers` (installs chromium, needs sudo for `--with-deps`) → `e2e-up`
  (`docker compose -f docker-compose.e2e.yml up -d --build --wait`, builds the
  combined image from source) → Playwright → teardown.
- **Stack shape** (`docker-compose.e2e.yml`, project name `vibexp-e2e`): Postgres
  (pgvector), fake-gcs-server + a one-shot bucket init, an `a2a-test-agent`, and
  `app` — the combined image, the **only** host-exposed port, `:8080`. Dev login
  is on (`FRONTEND_BASE_URL` is localhost → `IsLocalDevelopment()`), which also
  bypasses the per-IP rate limiter (#299). `INSTANCE_ADMIN_EMAILS` must stay in
  sync with `ADMIN_EMAIL` in `e2e/features/admin/*.spec.ts`.
- **Suite size:** ~209 tests, 2 workers, **~5–7 min** of test time; add the image
  build (first run is several minutes; later runs hit the Docker cache). `make
  e2e` sets `CI=true`, so every test retries twice (`smoke` specs excepted —
  they are meant to be stable and get none): a failure repeated across all 3
  attempts is deterministic, not flake.
- **Specs live in `frontend/e2e/`**: `features/`, `journeys/`, `smoke/`,
  plus `fixtures/` (`auth` → `devLogin`) and `helpers/`.
- **Known trap — teardown is broken (#598).** The `e2e` recipe's `cd frontend`
  leaks into the teardown line, so `docker compose … down` runs from `frontend/`
  and fails on the relative compose path. **The stack keeps running, volume and
  all**, and the next run reuses that Postgres data. Always tear down by hand
  from the repo root (below) until #598 lands.

## Inputs

- **ref** — branch, tag or SHA. Default `main`.
- **where** — `local`, `ci`, or `both`. Default: **`both`** when testing `main`
  (they run in parallel and cross-check each other); `ci` when testing a pushed
  branch you have not checked out; `local` when iterating on a fix.

## Procedure

### Phase 0 — Preflight

1. Resolve the ref. For a local run, the checkout must be **at** that ref and
   clean (`git fetch origin && git rev-parse HEAD origin/<ref>`); note that
   `make e2e` builds the image from the working tree, so uncommitted changes are
   silently included.
2. For a local run, check the prerequisites:
   - `docker compose version` works and the daemon is up.
   - **Port 8080 is free** — `ss -ltn | grep :8080`. A dev server on 8080 will
     make `e2e-up` fail. (Only 8080 is published; the stack's Postgres is
     internal, so a local Postgres on 5432 is fine.)
   - No stale stack from a previous run (see the teardown trap):
     `docker compose -f docker-compose.e2e.yml ps` — if anything is running,
     `make e2e-down` first, so the run does not start against old data.
3. For a CI run, confirm the `gh` account that may dispatch workflows on
   `vibexp/vibexp` (`gh api user -q .login`).
4. Note the **last green run** for context — it tells you how much has landed
   unverified:
   ```bash
   gh run list --workflow ci-e2e.yml -L 5 \
     --json databaseId,status,conclusion,createdAt
   git log --since="<that run's createdAt>" --oneline
   ```

### Phase 1 — Run it

**CI:**

```bash
gh workflow run ci-e2e.yml -f branch=<ref>
```

The dispatch returns no run id, so poll for the newest run just after it:

```bash
sleep 6
RUN_ID=$(gh run list --workflow ci-e2e.yml -L 1 --json databaseId --jq '.[0].databaseId')
gh run watch "$RUN_ID" --exit-status --interval 20
```

**Local** — long-running, so start it in the background with the output tee'd to
a log you can tail, and keep working while it runs:

```bash
(make frontend-install && make e2e) > <scratchpad>/e2e-local.log 2>&1
```

Do not babysit it with short sleeps; wait on a condition, e.g.
`until grep -qE '^  [0-9]+ (failed|passed)' <scratchpad>/e2e-local.log; do sleep 15; done`.
If both runs are in flight, let them run concurrently — they are independent.

Watch for these during the run: `Running N tests using 2 workers` (suite
started), `✘` lines (a failing test, with its retries), and the trailing
`N failed / M passed` summary.

### Phase 2 — Triage each failure to a root cause

Work per **distinct failing test**, not per failing assertion.

1. Pull the full error block from the log (`awk '/^  1\) /,/^  2\) /'` …) or, in
   CI, `gh run view <id> --log-failed`. The Playwright report and traces are
   uploaded as the `playwright-report` artifact; locally they are in
   `frontend/playwright-report` and `frontend/test-results` (screenshot, video,
   `error-context.md`, and `trace.zip` on retries —
   `npx playwright show-trace <path>`).
2. **Read the whole retry sequence, not just the first failure.** Different
   errors across retries are diagnostic: a timeout on attempt 1 and a
   *strict mode violation* on attempt 2 means the locator matched a second,
   newly-rendered element — a selector collision, not a race.
3. Decide **stale spec vs real regression** — this is the call that matters, and
   in this repo the answer is usually "stale spec", because refactors merge
   without an e2e run. Evidence for stale spec:
   - The error is "element not found" / "URL never matched" for chrome the app
     intentionally changed.
   - `git log --oneline -- <the component or route file>` since the last green
     run names a commit that renamed the heading, moved the route, or changed the
     control. Confirm by reading the **current** source, and diff it against the
     pre-change version (`git show <sha>^:<path>`) to prove what the spec was
     written against.
   Evidence for a real regression: the API 4xx/5xxs, the page errors, or the
   behaviour (not the label) is wrong. Check `docker compose -f
   docker-compose.e2e.yml logs app` while the stack is still up.
4. **Look for the assertion behind the assertion.** A spec that fails on line N
   usually has more stale lines further down that the first failure masks. Read
   the rest of the test and say so in the report, or the next run fails again on
   line N+20.
5. Recurring root-cause families worth checking first:
   - **Moved routes** — a `waitForURL`/`toHaveURL` regex pinned to a retired path
     (`/settings/teams/:id` → `/teams/:id`).
   - **Renamed/restructured chrome** — `getByRole('heading', …)` for a title that
     became a `<span>`, or case-sensitive `getByText`.
   - **Loose text selectors colliding with new UI** — `button:has-text("X")` also
     matching an unrelated control whose label now contains `X` (e.g. a fixture
     team name rendered in the header switcher). Prefer the `data-testid` hook.
   - **Vacuous `count() > 0` guards** — widespread in these specs (#559); a test
     that silently self-disables looks identical to one that passes.

### Phase 3 — Rerun cheaply while investigating

Do **not** re-run `make e2e` to check one spec. Keep the stack up and iterate:

```bash
make e2e-up                                   # build + boot, --wait for healthy
cd frontend && CI=true PLAYWRIGHT_BASE_URL=http://localhost:8080 \
  E2E_A2A_AGENT_URL=http://a2a-test-agent:9001 \
  npx playwright test e2e/features/teams/team-management.spec.ts --headed
```

`make e2e-test` does the same for the whole suite against a running stack. The
app is a real browsable instance at <http://localhost:8080> with dev login on —
open it and click through the failing flow; that usually settles stale-spec vs
regression in seconds.

**Always finish with `make e2e-down` from the repo root** (`-v` wipes the
volumes) — see the #598 trap. Verify with
`docker compose -f docker-compose.e2e.yml ps`.

### Phase 4 — Report and file

1. Report the headline first: `N failed / M passed`, whether local and CI agree,
   and whether the failures were deterministic across retries.
2. **File one GitHub issue per distinct root cause** (`gh issue create --label bug`),
   not one per failing test and not one big issue. Each should carry: the failing
   spec and line, the verbatim error, the **commit that caused it** (`<sha>` +
   PR/issue number), what the code does now vs what the spec expects, the
   masked-assertion follow-ups, and a suggested fix ending in "verify with
   `gh workflow run ci-e2e.yml -f branch=<branch>`". Cross-link the siblings.
3. Check for duplicates first (`gh issue list --search "e2e in:title"`), and link
   related known issues rather than restating them (#559 vacuous guards, #598
   teardown).
4. Do **not** fix anything unless asked — the ask is usually "run it and tell me
   what broke". Fixing a spec you have not re-run is how a dead test gets
   replaced with a differently-dead test.

## Related

- `release` skill — Phase 1 uses this same suite as the required pre-release gate.
- `verify` skill — for proving one feature works against the **dev** stack
  (`make backend-run-dev` + Vite), not the shipped image.
