import { expect, test, type Page } from '@playwright/test'

import { devLogin } from '../../fixtures/auth'

/**
 * E2E happy-path coverage for typed resource relations (issue #430, the ship
 * gate for epic #421). Exercises the assembled combined stack through the
 * shipped RelationsPanel on resource detail pages:
 *  - a stored confirmed edge renders with its direction-aware label and a
 *    working target link, and reads as the inverse direction on the target,
 *  - the composer adds a human edge (artifact --governed-by--> blueprint) and
 *    the target's panel shows the inverse ("governs"),
 *  - an ai-suggested edge carries the provenance badge; Accept confirms it
 *    (badge gone, row persists across reload) and Dismiss removes it,
 *  - a fresh resource renders the panel cleanly with zero rows and its GET
 *    payload carries `"related": []`.
 *
 * Modelled on the resource-comments spec (#278): a single authenticated owner,
 * team/resources/edges seeded via authenticated REST calls (that's scaffolding,
 * not the behaviour under test), and the UI driven for assertions. The owner has
 * resource.create / resource.update.any / resource.delete.any, so Add / Accept /
 * Dismiss all render without a second user. Suggested edges are seeded with
 * origin=ai + relation_type=governed-by, which the server maps to
 * status=suggested (models.InitialRelationStatus) — asserted in setup to fail
 * fast if that tiering ever changes.
 */

const stamp = Date.now()
const OWNER_NAME = 'Relations Owner'
const ownerEmail = `e2e_relations_owner_${process.env.TEST_WORKER_INDEX ?? '0'}_${stamp}@example.com`

// Generous timeout for panel assertions — the combined e2e stack runs specs in
// parallel, so a relation fetch/mutation can lag well past the 5s expect default.
const PANEL_TIMEOUT = 20000

// Distinct target blueprints per scenario so each row is uniquely filterable by
// its target title (the subject's panel shows every edge touching it at once).
const SUBJECT_TITLE = `Relations Subject Artifact ${stamp}`
const STORED_BP_TITLE = `Stored Governor ${stamp}`
const ACCEPT_BP_TITLE = `Accept Governor ${stamp}`
const DISMISS_BP_TITLE = `Dismiss Governor ${stamp}`
const MANUAL_BP_TITLE = `Manual Governor ${stamp}`
const EMPTY_TITLE = `Empty Relations Artifact ${stamp}`

let page: Page
let teamId: string
let projectId: string

let subjectArtifactId: string
let subjectUrl: string
let emptyArtifactSlug: string
let emptyUrl: string

const storedBlueprint = { id: '', slug: '', url: '' }
const acceptBlueprint = { id: '', slug: '', url: '' }
const dismissBlueprint = { id: '', slug: '', url: '' }
const manualBlueprint = { id: '', slug: '', url: '' }

async function postJson(url: string, data: unknown) {
  const res = await page.request.post(url, { data })
  expect(
    res.ok(),
    `POST ${url} failed: ${res.status()} ${await res.text()}`
  ).toBeTruthy()
  return res.json() as Promise<Record<string, unknown>>
}

/**
 * Pin the active team from boot via addInitScript (runs before the app reads the
 * team on every navigation) rather than a post-load localStorage write, which
 * races team hydration — under load the resource then fails to resolve and the
 * currentTeam-gated RelationsPanel never renders. (Same discipline as the
 * comments spec's pinTeam.)
 */
async function pinTeam(id: string) {
  await page.addInitScript(tid => {
    localStorage.setItem('vx_current_team_id', tid)
  }, id)
}

async function createBlueprint(title: string, slug: string) {
  const body = await postJson(`/api/v1/${teamId}/blueprints`, {
    project_id: projectId,
    slug,
    title,
    content: `Blueprint content for ${title}.`,
  })
  return { id: body.id as string, slug: body.slug as string }
}

/**
 * Seed one edge via REST and assert the server assigned the expected tiered
 * status (fail fast in setup, not mid-flow).
 */
async function seedRelation(opts: {
  toId: string
  origin: 'ai' | 'human'
  expectedStatus: 'suggested' | 'confirmed'
}) {
  const body = await postJson(`/api/v1/${teamId}/relations`, {
    from_type: 'artifact',
    from_id: subjectArtifactId,
    to_type: 'blueprint',
    to_id: opts.toId,
    relation_type: 'governed-by',
    origin: opts.origin,
  })
  expect(
    body.status,
    `seeded ${opts.origin} governed-by edge should be ${opts.expectedStatus}`
  ).toBe(opts.expectedStatus)
}

/** Wait for a resource detail page + panel to settle, then return the panel. */
async function relationsPanel(title: string) {
  // Wait for the resource's own h1 (PageHeader renders the title as an <h1>);
  // the view also renders an h1 in its loading/not-found states, so a generic
  // heading gate would pass before the real page (and its team-gated sidebar).
  await expect(
    page.getByRole('heading', { level: 1, name: title })
  ).toBeVisible({ timeout: PANEL_TIMEOUT })
  const panel = page.getByTestId('relations-panel')
  await expect(panel).toBeVisible({ timeout: PANEL_TIMEOUT })
  await expect(page.getByTestId('relations-loading')).toHaveCount(0, {
    timeout: PANEL_TIMEOUT,
  })
  return panel
}

const rowFor = (panel: ReturnType<Page['getByTestId']>, targetTitle: string) =>
  panel.getByTestId('relation-row').filter({ hasText: targetTitle })

test.describe.serial('Resource relations happy path', () => {
  test.describe.configure({ timeout: 150_000 })

  test.beforeAll(async ({ browser }) => {
    page = await browser.newPage()
    await devLogin(page, ownerEmail, OWNER_NAME)

    const teamBody = await postJson('/api/v1/teams', {
      name: `Relations E2E Team ${stamp}`,
      description: 'Created by the resource-relations e2e',
    })
    teamId = ((teamBody.team as { id?: string })?.id ??
      (teamBody.id as string)) as string
    expect(teamId, 'team id missing from create response').toBeTruthy()

    const projectBody = await postJson(`/api/v1/${teamId}/projects`, {
      name: `E2E Project ${stamp}`,
      slug: `e2e-relations-project-${stamp}`,
    })
    projectId = projectBody.id as string
    expect(projectId).toBeTruthy()

    const subject = await postJson(`/api/v1/${teamId}/artifacts`, {
      project_id: projectId,
      slug: `e2e-relations-subject-${stamp}`,
      title: SUBJECT_TITLE,
      content: 'Subject artifact under relations test.',
    })
    subjectArtifactId = subject.id as string
    subjectUrl = `/artifacts/${projectId}/${subject.slug as string}`

    const empty = await postJson(`/api/v1/${teamId}/artifacts`, {
      project_id: projectId,
      slug: `e2e-relations-empty-${stamp}`,
      title: EMPTY_TITLE,
      content: 'Artifact with no relations.',
    })
    emptyArtifactSlug = empty.slug as string
    emptyUrl = `/artifacts/${projectId}/${emptyArtifactSlug}`

    const stored = await createBlueprint(
      STORED_BP_TITLE,
      `e2e-rel-stored-${stamp}`
    )
    Object.assign(storedBlueprint, stored, {
      url: `/blueprints/${projectId}/${stored.slug}`,
    })
    const accept = await createBlueprint(
      ACCEPT_BP_TITLE,
      `e2e-rel-accept-${stamp}`
    )
    Object.assign(acceptBlueprint, accept, {
      url: `/blueprints/${projectId}/${accept.slug}`,
    })
    const dismiss = await createBlueprint(
      DISMISS_BP_TITLE,
      `e2e-rel-dismiss-${stamp}`
    )
    Object.assign(dismissBlueprint, dismiss, {
      url: `/blueprints/${projectId}/${dismiss.slug}`,
    })
    const manual = await createBlueprint(
      MANUAL_BP_TITLE,
      `e2e-rel-manual-${stamp}`
    )
    Object.assign(manualBlueprint, manual, {
      url: `/blueprints/${projectId}/${manual.slug}`,
    })

    // Seed the pre-existing edges: one confirmed (human) stored edge and two
    // ai-suggested edges (one to Accept, one to Dismiss). The manual blueprint
    // stays edge-free until the composer test links it.
    await seedRelation({
      toId: storedBlueprint.id,
      origin: 'human',
      expectedStatus: 'confirmed',
    })
    await seedRelation({
      toId: acceptBlueprint.id,
      origin: 'ai',
      expectedStatus: 'suggested',
    })
    await seedRelation({
      toId: dismissBlueprint.id,
      origin: 'ai',
      expectedStatus: 'suggested',
    })

    await pinTeam(teamId)
  })

  test.afterAll(async () => {
    await page?.close()
  })

  test('panel renders a stored confirmed edge with direction label + working target link, inverse on target', async () => {
    await page.goto(subjectUrl)
    const panel = await relationsPanel(SUBJECT_TITLE)

    const row = rowFor(panel, STORED_BP_TITLE)
    await expect(row).toBeVisible({ timeout: PANEL_TIMEOUT })
    // Outgoing governed-by reads "governed by" on the subject side; a confirmed
    // edge carries no ai-suggested badge.
    await expect(row).toContainText('governed by', { timeout: PANEL_TIMEOUT })
    await expect(row.getByTestId('relation-suggested-badge')).toHaveCount(0)

    // The target link navigates to the blueprint detail page…
    await row.getByTestId('relation-target-link').click()
    await expect(page).toHaveURL(new RegExp(`/blueprints/${projectId}/`), {
      timeout: PANEL_TIMEOUT,
    })

    // …where the same edge reads as the inverse direction ("governs") and links
    // back to the subject artifact.
    const targetPanel = await relationsPanel(STORED_BP_TITLE)
    const inverseRow = rowFor(targetPanel, SUBJECT_TITLE)
    await expect(inverseRow).toBeVisible({ timeout: PANEL_TIMEOUT })
    await expect(inverseRow).toContainText('governs', {
      timeout: PANEL_TIMEOUT,
    })
  })

  test('composer adds a governed-by edge; the target blueprint shows the inverse', async () => {
    await page.goto(subjectUrl)
    const panel = await relationsPanel(SUBJECT_TITLE)

    await panel.getByTestId('relation-add-button').click()
    const composer = panel.getByTestId('relation-composer')
    await expect(composer).toBeVisible({ timeout: PANEL_TIMEOUT })

    // governed-by is the default, but set it explicitly for robustness.
    await composer
      .getByTestId('relation-type-select')
      .selectOption('governed-by')

    const targetSelect = composer.getByTestId('relation-target-select')
    await expect(targetSelect).toBeEnabled({ timeout: PANEL_TIMEOUT })
    // Wait for the async-loaded blueprint options before selecting.
    await expect(
      targetSelect.locator('option', { hasText: MANUAL_BP_TITLE })
    ).toHaveCount(1, { timeout: PANEL_TIMEOUT })
    await targetSelect.selectOption(manualBlueprint.id)

    await composer.getByTestId('relation-add-submit').click()

    // The new edge appears on the subject's panel with the outgoing label.
    const newRow = rowFor(panel, MANUAL_BP_TITLE)
    await expect(newRow).toBeVisible({ timeout: PANEL_TIMEOUT })
    await expect(newRow).toContainText('governed by', {
      timeout: PANEL_TIMEOUT,
    })

    // The target blueprint's panel shows the inverse ("governs") back to us.
    await page.goto(manualBlueprint.url)
    const targetPanel = await relationsPanel(MANUAL_BP_TITLE)
    const inverseRow = rowFor(targetPanel, SUBJECT_TITLE)
    await expect(inverseRow).toBeVisible({ timeout: PANEL_TIMEOUT })
    await expect(inverseRow).toContainText('governs', {
      timeout: PANEL_TIMEOUT,
    })
  })

  test('an ai-suggested edge shows the badge; Accept confirms it and the row persists', async () => {
    await page.goto(subjectUrl)
    const panel = await relationsPanel(SUBJECT_TITLE)

    const row = rowFor(panel, ACCEPT_BP_TITLE)
    await expect(row).toBeVisible({ timeout: PANEL_TIMEOUT })
    await expect(row.getByTestId('relation-suggested-badge')).toBeVisible({
      timeout: PANEL_TIMEOUT,
    })

    await row.getByTestId('relation-accept').click()
    // Confirmed: badge and Accept/Dismiss controls disappear, row stays.
    await expect(row.getByTestId('relation-suggested-badge')).toHaveCount(0, {
      timeout: PANEL_TIMEOUT,
    })
    await expect(row.getByTestId('relation-accept')).toHaveCount(0, {
      timeout: PANEL_TIMEOUT,
    })
    await expect(row).toBeVisible()

    // Survives a reload (the confirm persisted server-side).
    await page.reload()
    const reloaded = await relationsPanel(SUBJECT_TITLE)
    const persistedRow = rowFor(reloaded, ACCEPT_BP_TITLE)
    await expect(persistedRow).toBeVisible({ timeout: PANEL_TIMEOUT })
    await expect(
      persistedRow.getByTestId('relation-suggested-badge')
    ).toHaveCount(0)
  })

  test('Dismiss removes a suggested edge and it stays gone after reload', async () => {
    await page.goto(subjectUrl)
    const panel = await relationsPanel(SUBJECT_TITLE)

    const row = rowFor(panel, DISMISS_BP_TITLE)
    await expect(row).toBeVisible({ timeout: PANEL_TIMEOUT })
    await expect(row.getByTestId('relation-suggested-badge')).toBeVisible({
      timeout: PANEL_TIMEOUT,
    })

    await row.getByTestId('relation-dismiss').click()
    await expect(rowFor(panel, DISMISS_BP_TITLE)).toHaveCount(0, {
      timeout: PANEL_TIMEOUT,
    })

    // Deletion persisted server-side — gone after reload too.
    await page.reload()
    const reloaded = await relationsPanel(SUBJECT_TITLE)
    await expect(rowFor(reloaded, DISMISS_BP_TITLE)).toHaveCount(0, {
      timeout: PANEL_TIMEOUT,
    })
  })

  test('a fresh resource renders a clean empty panel and its GET payload carries "related": []', async () => {
    // The resource GET (consumed by the detail page) emits related as [].
    const res = await page.request.get(
      `/api/v1/${teamId}/artifacts/${projectId}/${encodeURIComponent(
        emptyArtifactSlug
      )}`
    )
    expect(res.ok(), `GET empty artifact failed: ${res.status()}`).toBeTruthy()
    const artifact = (await res.json()) as { related?: unknown }
    expect(artifact.related).toEqual([])

    await page.goto(emptyUrl)
    const panel = await relationsPanel(EMPTY_TITLE)
    await expect(panel.getByTestId('relation-row')).toHaveCount(0, {
      timeout: PANEL_TIMEOUT,
    })
    await expect(panel).toContainText('No relations yet.', {
      timeout: PANEL_TIMEOUT,
    })
  })
})
