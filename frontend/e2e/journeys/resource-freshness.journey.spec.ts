import { expect, test, type Page } from '@playwright/test'

import { devLogin } from '../fixtures/auth'
import { backdateResource, e2eStackAvailable } from '../helpers/e2eDatabase'

/**
 * The Resource Freshness loop, end to end (issue #739, the ship gate for epic
 * #726). One journey across every seam the epic added:
 *
 *   rule created in the UI (#731/#736)
 *     -> scheduler picks the team up and the evaluator marks an aged artifact
 *        stale, with a `rule_run` audit row (#728/#732)
 *     -> the mark surfaces on the artifact list as a badge and under the
 *        "stale only" filter (#735/#738), and in the analytics + audit tabs
 *        (#734/#737)
 *     -> opening the artifact clears the flag and logs the access (#733).
 *
 * ## How this stays deterministic
 *
 * Nothing here waits on wall-clock time; both halves of the timing are inputs
 * the harness controls.
 *
 * **The resource is old before the rule exists.** `threshold_days` has a
 * one-day floor, and staleness is
 * `GREATEST(updated_at, <last_accessed_*>) < now() - threshold`, so a resource
 * created by this spec can never be a candidate. It is seeded through the API
 * and then aged by {@link backdateResource} — see `helpers/e2eDatabase.ts` for
 * why that is SQL and not an API call.
 *
 * **The evaluation run is triggered, not awaited.** Creating a rule calls
 * `FreshnessService.syncSchedule`, which upserts the team's `schedules` row with
 * a zero `next_run_at` — the database stamps "now", so the team is due on the
 * very next scheduler tick. `docker-compose.e2e.yml` sets
 * `SCHEDULER_TICK_INTERVAL: 5s` so that tick is seconds away rather than the
 * production-shaped minute. Ordering is therefore load-bearing: the artifact
 * must be aged BEFORE the rule is created, because the run the rule triggers is
 * the only one this journey gets (the schedule then advances by the team's
 * interval, a day).
 *
 * Waits are `expect.poll` against the API — the authority on the state — rather
 * than sleeps, and the UI assertions follow a settled backend.
 *
 * The whole journey needs the docker e2e stack (`make e2e` / `ci-e2e.yml`),
 * because aging a row is not something the API can express; against a bare
 * `npm run dev` it skips with a reason instead of quietly asserting nothing.
 */

const HAS_E2E_STACK = e2eStackAvailable()

const stamp = Date.now()
const worker = process.env.TEST_WORKER_INDEX ?? '0'
const ownerEmail = `e2e_freshness_owner_${worker}_${stamp}@example.com`
const OWNER_NAME = 'Freshness Owner'

const ARTIFACT_TITLE = `Freshness Subject Artifact ${stamp}`

/** Older than the rule's threshold by a wide margin, so the seed is unambiguous. */
const BACKDATE_DAYS = 30
const THRESHOLD_DAYS = '1'

/**
 * How long an evaluation run (or a reversal) may take to land. Generous
 * relative to the 5s tick: the stack runs the whole suite in parallel, and the
 * cost of being slow here is seconds while the cost of being tight is a flaky
 * ship gate.
 */
const ENGINE_TIMEOUT = 60_000
/** Combined-stack UI budget, matching the other journeys. */
const UI_TIMEOUT = 20_000

let page: Page
let teamId: string
let projectId: string
let artifactId: string
let artifactSlug: string
let artifactUrl: string

async function postJson(url: string, data: unknown) {
  const res = await page.request.post(url, { data })
  expect(
    res.ok(),
    `POST ${url} failed: ${res.status()} ${await res.text()}`
  ).toBeTruthy()
  return res.json() as Promise<Record<string, unknown>>
}

/**
 * How many of the team's artifacts the API currently reports as stale.
 *
 * The LIST endpoint is used on purpose: only detail reads record an access, so
 * polling this cannot itself clear the flag the poll is waiting for.
 */
async function staleArtifactCount(): Promise<number> {
  const res = await page.request.get(
    `/api/v1/${teamId}/artifacts?freshness=stale`
  )
  expect(
    res.ok(),
    `stale artifact listing failed: ${res.status()} ${await res.text()}`
  ).toBeTruthy()
  const body = (await res.json()) as { total_count: number }
  return body.total_count
}

/**
 * Pin the active team from boot rather than writing localStorage after a load,
 * which races team hydration (same discipline as the relations and comments
 * specs).
 */
async function pinTeam(id: string) {
  await page.addInitScript(tid => {
    localStorage.setItem('vx_current_team_id', tid)
  }, id)
}

/** Opens the Resource Freshness page on the named tab and waits for it to settle. */
async function openFreshnessTab(name: 'Settings' | 'Analytics' | 'Audit') {
  await page.goto(`/teams/${teamId}/settings/freshness`)
  await expect(
    page.getByRole('heading', { level: 1, name: 'Resource Freshness' })
  ).toBeVisible({ timeout: UI_TIMEOUT })
  await page.getByRole('tab', { name }).click()
}

/** The artifact's row on a resource list page. */
const artifactRow = () =>
  page.getByRole('row').filter({ hasText: ARTIFACT_TITLE })

test.describe.serial('Resource freshness journey', () => {
  test.describe.configure({ timeout: 180_000 })

  test.skip(
    !HAS_E2E_STACK,
    'needs the docker e2e stack (make e2e): the seeded artifact is aged with SQL'
  )

  test.beforeAll(async ({ browser }) => {
    page = await browser.newPage()
    await devLogin(page, ownerEmail, OWNER_NAME)

    const teamBody = await postJson('/api/v1/teams', {
      name: `Freshness E2E Team ${stamp}`,
      description: 'Created by the resource-freshness e2e',
    })
    teamId = ((teamBody.team as { id?: string })?.id ??
      (teamBody.id as string)) as string
    expect(teamId, 'team id missing from create response').toBeTruthy()

    const projectBody = await postJson(`/api/v1/${teamId}/projects`, {
      name: `Freshness E2E Project ${stamp}`,
      slug: `e2e-freshness-project-${stamp}`,
    })
    projectId = projectBody.id as string
    expect(projectId).toBeTruthy()

    const artifact = await postJson(`/api/v1/${teamId}/artifacts`, {
      project_id: projectId,
      slug: `e2e-freshness-subject-${stamp}`,
      title: ARTIFACT_TITLE,
      content: 'Nobody has opened this in a long time.',
    })
    artifactId = artifact.id as string
    artifactSlug = artifact.slug as string
    artifactUrl = `/artifacts/${projectId}/${artifactSlug}`

    // Age it BEFORE any rule exists — see the header note on ordering.
    backdateResource('artifacts', artifactId, BACKDATE_DAYS)

    // Nothing is stale until a rule says so; asserting that here is what makes
    // the later "1 stale" a change this journey caused rather than a fixture.
    expect(await staleArtifactCount()).toBe(0)

    await pinTeam(teamId)
  })

  test.afterAll(async () => {
    await page.close()
  })

  test('an admin creates a freshness rule and it persists', async () => {
    await openFreshnessTab('Settings')

    // Reversibility is the precondition for the last step of the journey, and
    // it is on by default — assert rather than set it, so a change of default
    // fails here instead of silently disabling the reversal assertion.
    await expect(page.locator('#reversibility')).toHaveAttribute(
      'aria-checked',
      'true'
    )

    await page.getByTestId('create-rule-button').click()

    await page.locator('#resource-type-artifact').click()
    await page.locator('#rule-threshold').fill(THRESHOLD_DAYS)
    await page.getByTestId('submit-rule-button').click()

    // Any project, any medium, one day — the sentence the rules card renders.
    const rule = page.getByTestId('freshness-rule-row')
    await expect(rule).toHaveCount(1, { timeout: UI_TIMEOUT })
    await expect(rule).toContainText(/artifacts in any project/i)
    await expect(rule).toContainText(/for 1 day/i)

    // Survives a reload: it is stored, not just in the page's state.
    await page.reload()
    await expect(page.getByTestId('freshness-rule-row')).toHaveCount(1, {
      timeout: UI_TIMEOUT,
    })
  })

  test('the scheduled evaluation marks the aged artifact stale', async () => {
    await expect
      .poll(staleArtifactCount, {
        timeout: ENGINE_TIMEOUT,
        message:
          'the freshness evaluation run never marked the aged artifact stale',
      })
      .toBe(1)
  })

  test('the stale artifact carries its badge on the artifact list', async () => {
    await page.goto('/artifacts')

    const row = artifactRow()
    await expect(row).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(row.getByTestId('freshness-badge')).toBeVisible()
  })

  test('the "stale only" filter returns it', async () => {
    await page.goto('/artifacts')
    await expect(artifactRow()).toBeVisible({ timeout: UI_TIMEOUT })

    // Each list page names its own control (`FreshnessFilterSelect` takes the
    // test id), so this is the artifacts one, not the shared default.
    await page.getByTestId('artifact-freshness-filter').click()
    await page.getByRole('option', { name: 'Stale only' }).click()

    // The filter is URL-synced, so the address bar is part of the contract.
    await expect(page).toHaveURL(/freshness=stale/)
    await expect(artifactRow()).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(page.getByTestId('freshness-badge')).toHaveCount(1)
  })

  test('the analytics tab counts it under its resource type', async () => {
    await openFreshnessTab('Analytics')

    const byType = page
      .getByTestId('category-breakdown-chart')
      .filter({ hasText: 'Stale by resource type' })
    await expect(byType).toBeVisible({ timeout: UI_TIMEOUT })
    // Label and value are separate elements, so the flattened text has no space
    // between them; the lookahead is what stops "1" from also matching "10".
    await expect(byType).toContainText(/Stale resources:\s*1(?!\d)/)

    // The breakdown lists every type, so the assertion has to be the artifact
    // row's own count — a bare "contains 1" would pass on the zeroes' siblings.
    const artifactsRow = byType
      .getByTestId('category-row')
      .filter({ hasText: 'Artifacts' })
    await expect(artifactsRow).toHaveCount(1)
    await expect(artifactsRow).toHaveText(/^Artifacts\s*1$/)
  })

  test('the audit tab records the mark and attributes it to a rule', async () => {
    await openFreshnessTab('Audit')

    const marked = page
      .getByTestId('audit-row')
      .filter({ hasText: /marked stale/i })
    await expect(marked).toHaveCount(1, { timeout: UI_TIMEOUT })
    // `rule_run` is the only reason that can mark, and this is how the tab
    // spells it.
    await expect(marked).toContainText(/a rule matched it/i)
  })

  test('opening the artifact clears the flag and logs the access', async () => {
    await page.goto(artifactUrl)
    // The detail read IS the access — gate on the artifact's own h1, since the
    // loading and not-found states also render one.
    await expect(
      page.getByRole('heading', { level: 1, name: ARTIFACT_TITLE })
    ).toBeVisible({ timeout: UI_TIMEOUT })

    await expect
      .poll(staleArtifactCount, {
        timeout: ENGINE_TIMEOUT,
        message: 'opening the artifact never cleared its stale flag',
      })
      .toBe(0)

    // The artifact is still listed and still findable — it just is not stale.
    // (The positive half: a bare "no badge" assertion would also pass on a page
    // that rendered nothing at all.)
    await page.goto('/artifacts')
    const row = artifactRow()
    await expect(row).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(row.getByTestId('freshness-badge')).toHaveCount(0)

    await page.goto('/artifacts?freshness=stale')
    await expect(artifactRow()).toHaveCount(0, { timeout: UI_TIMEOUT })

    await openFreshnessTab('Audit')
    const cleared = page
      .getByTestId('audit-row')
      .filter({ hasText: /someone opened it/i })
    await expect(cleared).toHaveCount(1, { timeout: UI_TIMEOUT })
    // The mark is still on the record — a clear appends, it does not rewrite.
    await expect(
      page.getByTestId('audit-row').filter({ hasText: /marked stale/i })
    ).toHaveCount(1)
  })
})
