import { expect, test, type BrowserContext, type Page } from '@playwright/test'

import { devLogin } from '../fixtures/auth'

/**
 * Custom statuses, end to end (issue #1271, the ship gate for epic #1263).
 *
 *   an owner creates a custom artifact status on Settings › Statuses (#1269)
 *     -> sets it on an artifact through the edit form's catalog-driven select,
 *        and the detail and list badges read its DISPLAY NAME while the
 *        resource stores its slug (#1266/#1270)
 *     -> the artifact is still found by global search and by the list's status
 *        filter, which survives a reload (#1267/#1270)
 *     -> a plain member reads the same Statuses page with no write control,
 *        and a direct POST from that session is refused (#1265)
 *     -> the owner deletes the status: the dialog names the one artifact and
 *        its destination, the artifact lands on the system default, and the
 *        status disappears from the table, the form select and the filter.
 *
 * ## Why these assertions need an e2e
 *
 * Every sub-issue of the epic is tested with a mock at its boundary, and two of
 * the epic's promises cross those boundaries. "An item in a custom status stays
 * visible" spans the open status column, the live-status denylist that search
 * and lists read, and the catalog the filter is built from: a denylist that
 * regressed into an allowlist of the system values would pass every unit suite
 * and hide the artifact here. "Deleting a status reassigns its items" spans the
 * delete transaction, the settings page and the shared resource UI: only a
 * browser against the real stack shows the badge the user ends up looking at.
 *
 * ## How this stays deterministic
 *
 * Nothing waits on wall-clock time. The team is new, so it holds exactly the
 * two artifacts seeded here: the one that moves through the custom status and
 * a control that stays `active`, which is what makes the filter assertion able
 * to fail (a filter that narrowed nothing would still show the control). The
 * stack has no embedding provider, so search takes the keyword path — it reads
 * the same live-status rule as semantic search and has no indexing latency.
 * Scaffolding (the team, the project, the artifacts, the invitation) goes
 * through the API because it is not what is under test; creating, applying,
 * filtering on and deleting the status are driven through the pages.
 */

const stamp = Date.now()
const worker = process.env.TEST_WORKER_INDEX ?? '0'

const OWNER_NAME = 'Status Owner'
const MEMBER_NAME = 'Status Member'

/** The system status a deleted artifact status falls back to, as the UI names it. */
const FALLBACK_LABEL = 'Active'
const FALLBACK_SLUG = 'active'

/** Combined-stack UI budget, matching the other journeys. */
const UI_TIMEOUT = 20_000

/**
 * Everything a locator matches by NAME is scoped per ATTEMPT, not per file:
 * `describe.serial` re-runs `beforeAll` on a retry, and a file-scoped name
 * would match the previous attempt's rows too. Assigned in `beforeAll`.
 */
let STATUS_NAME: string
let STATUS_SLUG: string
let ARTIFACT_TITLE: string
let ARTIFACT_SLUG: string
let CONTROL_TITLE: string
/** One unique word both artifacts contain — the global search term. */
let QUERY: string

let ownerCtx: BrowserContext
let memberCtx: BrowserContext
let ownerPage: Page
let memberPage: Page

let teamId: string
let projectId: string

/** An API call that must succeed; returns the parsed JSON body. */
async function api(
  page: Page,
  method: 'GET' | 'POST',
  url: string,
  data?: unknown
): Promise<Record<string, unknown>> {
  const res = await page.request.fetch(url, { method, data })
  const body = await res.text()
  expect(
    res.ok(),
    `${method} ${url} failed: ${String(res.status())} ${body}`
  ).toBe(true)
  return JSON.parse(body) as Record<string, unknown>
}

/**
 * A search term full-text search tokenizes as ONE unique word: letters only,
 * so the run's digits can neither split it nor stem it into something shared.
 */
function lettersOnly(seed: string): string {
  return seed.replace(/\d/g, digit => 'abcdefghij'.charAt(Number(digit)))
}

interface StatusRow {
  slug: string
  name: string
  is_system: boolean
  usage_count: number
}

/** The team's artifact statuses, as the API reports them. */
async function artifactStatuses(page: Page): Promise<StatusRow[]> {
  const body = await api(
    page,
    'GET',
    `/api/v1/${teamId}/statuses?resource_type=artifact`
  )
  return body.statuses as StatusRow[]
}

/** The status the journey's artifact currently stores (a slug, never a name). */
async function storedStatus(): Promise<string> {
  const artifact = await api(
    ownerPage,
    'GET',
    `/api/v1/${teamId}/artifacts/${projectId}/${ARTIFACT_SLUG}`
  )
  return artifact.status as string
}

const artifactPath = () => `/artifacts/${projectId}/${ARTIFACT_SLUG}`

/**
 * Open Settings › Statuses on the Artifacts tab and wait for the LIST, not the
 * heading: the heading renders while the statuses are still loading, and the
 * `Active` system row is in every team's answer.
 */
async function openArtifactStatuses(page: Page): Promise<void> {
  await page.goto(`/teams/${teamId}/settings/statuses`)
  await expect(
    page.getByRole('heading', { level: 1, name: 'Statuses' })
  ).toBeVisible({ timeout: UI_TIMEOUT })
  await page.getByRole('tab', { name: 'Artifacts', exact: true }).click()
  await expect(statusRow(page, FALLBACK_SLUG)).toContainText('System', {
    timeout: UI_TIMEOUT,
  })
}

/** The settings-table row of one status, matched on its slug cell. */
function statusRow(page: Page, slug: string) {
  return page.getByTestId('status-item').filter({
    has: page.locator('code', { hasText: new RegExp(`^${slug}$`) }),
  })
}

/** A row of the artifacts list, matched on the artifact's title. */
function artifactRow(page: Page, title: string) {
  return page.getByRole('row').filter({ hasText: title })
}

/** Open the artifacts list and wait until its rows have loaded. */
async function openArtifactList(page: Page, query = ''): Promise<void> {
  await page.goto(`/artifacts${query}`)
  await expect(artifactRow(page, ARTIFACT_TITLE)).toBeVisible({
    timeout: UI_TIMEOUT,
  })
}

/** Open the artifact's edit form and wait for the status select. */
async function openArtifactEdit(page: Page): Promise<void> {
  await page.goto(artifactPath())
  await page.getByTestId('edit-artifact-button').click()
  await expect(page).toHaveURL(/\/edit$/, { timeout: UI_TIMEOUT })
  await expect(page.getByTestId('artifact-status-select')).toBeVisible({
    timeout: UI_TIMEOUT,
  })
}

test.describe.serial('Custom statuses journey', () => {
  test.describe.configure({ timeout: 180_000 })

  test.beforeAll(async ({ browser }, testInfo) => {
    const run = `${worker}-${String(stamp)}-${String(testInfo.retry)}`
    // Short on purpose: a status name and its slug are capped at 50 characters.
    const tag = `${worker}${stamp.toString(36)}${String(testInfo.retry)}`
    STATUS_NAME = `In review ${tag}`
    STATUS_SLUG = `in-review-${tag}`
    QUERY = `quartz${lettersOnly(run.replace(/-/g, ''))}`
    ARTIFACT_TITLE = `Status journey artifact ${QUERY}`
    ARTIFACT_SLUG = `status-journey-${tag}`
    CONTROL_TITLE = `Status journey control ${QUERY}`

    ownerCtx = await browser.newContext()
    ownerPage = await ownerCtx.newPage()
    await devLogin(ownerPage, `e2e_status_owner_${run}@example.com`, OWNER_NAME)

    const team = await api(ownerPage, 'POST', '/api/v1/teams', {
      name: `Status Journey Team ${run}`,
      description: 'Created by the custom statuses e2e',
    })
    teamId = ((team.team as { id?: string } | undefined)?.id ??
      team.id) as string
    expect(teamId, 'team id missing from create response').toBeTruthy()

    const project = await api(ownerPage, 'POST', `/api/v1/${teamId}/projects`, {
      name: `Status Journey Project ${tag}`,
      slug: `status-journey-${tag}`,
    })
    projectId = project.id as string
    expect(projectId, 'project id missing from create response').toBeTruthy()

    // Two artifacts, both created in the default status. Only the first ever
    // leaves it; the second is what the status filter has to exclude.
    for (const artifact of [
      { slug: ARTIFACT_SLUG, title: ARTIFACT_TITLE },
      { slug: `status-control-${tag}`, title: CONTROL_TITLE },
    ]) {
      await api(ownerPage, 'POST', `/api/v1/${teamId}/artifacts`, {
        project_id: projectId,
        ...artifact,
        content: `Written for the custom statuses journey: ${QUERY}.`,
      })
    }
    expect(await storedStatus()).toBe(FALLBACK_SLUG)

    // --- A plain member of the team, for the permission check. Invited and
    // accepted through the API: joining a team is not what is under test.
    const memberEmail = `e2e_status_member_${run}@example.com`
    await api(ownerPage, 'POST', `/api/v1/teams/${teamId}/invitations`, {
      emails: [memberEmail],
      role: 'member',
    })
    memberCtx = await browser.newContext()
    memberPage = await memberCtx.newPage()
    await devLogin(memberPage, memberEmail, MEMBER_NAME)
    const pending = await api(memberPage, 'GET', '/api/v1/invitations/pending')
    const token = (pending.invitations as { token: string }[] | undefined)?.[0]
      ?.token
    expect(token, 'invitee has no pending invitation token').toBeTruthy()
    await api(
      memberPage,
      'POST',
      `/api/v1/invitations/${encodeURIComponent(token ?? '')}/accept`
    )
    const memberTeams = await api(memberPage, 'GET', '/api/v1/teams')
    const joined = (
      memberTeams.teams as { id: string; role?: string }[] | undefined
    )?.find(candidate => candidate.id === teamId)
    expect(joined?.role, 'the invitee did not join as a plain member').toBe(
      'member'
    )

    // Both actors work inside the new team. `addInitScript` rather than a
    // post-load localStorage write, which races team hydration.
    for (const page of [ownerPage, memberPage]) {
      await page.addInitScript(id => {
        localStorage.setItem('vx_current_team_id', id)
      }, teamId)
    }
  })

  // Optional chaining is deliberate: a `beforeAll` that fails partway leaves
  // the later context unassigned, and an unguarded `close()` would replace the
  // real setup failure with a TypeError.
  test.afterAll(async () => {
    await memberCtx?.close()
    await ownerCtx?.close()
  })

  test('the owner creates a custom artifact status', async () => {
    await openArtifactStatuses(ownerPage)
    await expect(statusRow(ownerPage, STATUS_SLUG)).toHaveCount(0)

    await ownerPage.getByTestId('create-status-button').click()
    await ownerPage.getByTestId('status-name-input').fill(STATUS_NAME)
    // The slug follows the name until it is edited by hand; it is what the
    // resources will store, so the journey pins it rather than typing it.
    await expect(ownerPage.getByTestId('status-slug-input')).toHaveValue(
      STATUS_SLUG
    )
    await ownerPage.getByTestId('submit-status-button').click()

    const row = statusRow(ownerPage, STATUS_SLUG)
    await expect(row).toHaveCount(1, { timeout: UI_TIMEOUT })
    await expect(row).toContainText(STATUS_NAME)
    await expect(row).toContainText('Custom')
    await expect(row.getByTestId('status-usage')).toHaveText('0')
    // The write controls the member must NOT be offered, present for the owner
    // on this same row — the positive half of the permission check below.
    await expect(row.getByTestId('rename-status-button')).toBeVisible()
    await expect(row.getByTestId('delete-status-button')).toBeVisible()
  })

  test('the owner applies it to an artifact and the badges read its display name', async () => {
    await openArtifactEdit(ownerPage)

    await ownerPage.getByTestId('artifact-status-select').click()
    await ownerPage
      .getByRole('option', { name: STATUS_NAME, exact: true })
      .click()
    // By exact name: the edit header's "Copy slug: <slug>" chip would match a
    // loose /update|save/ filter whenever the slug contains the word (#1179).
    await ownerPage.getByRole('button', { name: 'Save changes' }).click()

    // The detail page: the header badge reads the display name...
    await expect(ownerPage).not.toHaveURL(/\/edit$/, { timeout: UI_TIMEOUT })
    await expect(ownerPage.getByTestId('resource-header-meta')).toContainText(
      STATUS_NAME,
      { timeout: UI_TIMEOUT }
    )
    // ...while the resource itself stores the slug.
    expect(await storedStatus()).toBe(STATUS_SLUG)

    // The list: the same name on the artifact's row, and the control artifact
    // still on the system default beside it.
    await openArtifactList(ownerPage)
    await expect(artifactRow(ownerPage, ARTIFACT_TITLE)).toContainText(
      STATUS_NAME
    )
    await expect(artifactRow(ownerPage, CONTROL_TITLE)).toContainText(
      FALLBACK_LABEL
    )
    await expect(artifactRow(ownerPage, CONTROL_TITLE)).not.toContainText(
      STATUS_NAME
    )
  })

  test('the artifact is found by global search and by the status filter', async () => {
    // Search first through the API, so a failure says whether the backend or
    // the page dropped the artifact. It is polled because the response, not a
    // sleep, is what says the document is searchable.
    await expect
      .poll(
        async () => {
          const found = await api(
            ownerPage,
            'POST',
            `/api/v1/${teamId}/search`,
            { query: QUERY }
          )
          return (found.results as { title?: string }[]).map(
            result => result.title
          )
        },
        {
          timeout: UI_TIMEOUT,
          message: 'search never returned the artifact in the custom status',
        }
      )
      .toContain(ARTIFACT_TITLE)

    await ownerPage.goto(`/search?q=${encodeURIComponent(QUERY)}`)
    await expect(
      ownerPage.getByRole('heading', { name: ARTIFACT_TITLE })
    ).toBeVisible({ timeout: UI_TIMEOUT })

    // The list filter: choosing the custom status keeps the artifact, drops
    // the control and writes the SLUG to the URL.
    await openArtifactList(ownerPage)
    await expect(artifactRow(ownerPage, CONTROL_TITLE)).toBeVisible()
    const filter = ownerPage.getByTestId('artifact-status-filter')
    await filter.click()
    await ownerPage
      .getByRole('option', { name: STATUS_NAME, exact: true })
      .click()

    const filteredUrl = new RegExp(`[?&]status=${STATUS_SLUG}(&|$)`)
    await expect(ownerPage).toHaveURL(filteredUrl)
    await expect(artifactRow(ownerPage, CONTROL_TITLE)).toHaveCount(0, {
      timeout: UI_TIMEOUT,
    })
    await expect(artifactRow(ownerPage, ARTIFACT_TITLE)).toBeVisible()

    // A reload has to wait for the catalog before it can tell this slug from
    // junk; it must neither strip the filter nor show the unfiltered list.
    await ownerPage.reload()
    await expect(artifactRow(ownerPage, ARTIFACT_TITLE)).toBeVisible({
      timeout: UI_TIMEOUT,
    })
    await expect(ownerPage).toHaveURL(filteredUrl)
    await expect(filter).toContainText(STATUS_NAME)
    await expect(artifactRow(ownerPage, CONTROL_TITLE)).toHaveCount(0)
  })

  test('a member reads the statuses and is offered no write control', async () => {
    await openArtifactStatuses(memberPage)

    // The rows are there, system and custom alike, with the live usage...
    const row = statusRow(memberPage, STATUS_SLUG)
    await expect(row).toContainText(STATUS_NAME)
    await expect(row).toContainText('Custom')
    await expect(row.getByTestId('status-usage')).toHaveText('1')

    // ...and none of the controls the owner had on this same page.
    await expect(memberPage.getByTestId('create-status-button')).toHaveCount(0)
    await expect(memberPage.getByTestId('rename-status-button')).toHaveCount(0)
    await expect(memberPage.getByTestId('delete-status-button')).toHaveCount(0)

    // Hiding a button is convenience; the server is what refuses. The body is
    // one the owner's session would have been allowed to create, so a 403 can
    // only be about who is asking.
    const refusedSlug = `member-made-${STATUS_SLUG}`.slice(0, 50)
    const refused = await memberPage.request.post(
      `/api/v1/${teamId}/statuses`,
      {
        data: {
          resource_type: 'artifact',
          slug: refusedSlug,
          name: 'Member made',
        },
      }
    )
    expect(refused.status()).toBe(403)
    const slugs = (await artifactStatuses(memberPage)).map(
      status => status.slug
    )
    expect(slugs).toContain(STATUS_SLUG)
    expect(slugs).not.toContain(refusedSlug)
  })

  test('deleting the status moves the artifact to the system default', async () => {
    await openArtifactStatuses(ownerPage)
    const row = statusRow(ownerPage, STATUS_SLUG)
    await expect(row.getByTestId('status-usage')).toHaveText('1')

    await row.getByTestId('delete-status-button').click()
    // The confirmation says what will happen before it happens: which status,
    // how many artifacts, and where they go.
    const dialog = ownerPage.getByRole('alertdialog')
    await expect(dialog).toContainText(STATUS_NAME, { timeout: UI_TIMEOUT })
    await expect(dialog).toContainText(
      `1 artifact will move to ${FALLBACK_LABEL}`
    )

    // The response is the contract. Armed before the click so it cannot be
    // missed.
    const deleteResponse = ownerPage.waitForResponse(
      response =>
        response.request().method() === 'DELETE' &&
        /\/api\/v1\/[^/]+\/statuses\/[^/]+$/.test(
          new URL(response.url()).pathname
        )
    )
    await dialog.getByRole('button', { name: 'Delete', exact: true }).click()
    const response = await deleteResponse
    expect(response.status()).toBe(200)
    expect(await response.json()).toMatchObject({
      reassigned_count: 1,
      reassigned_to: FALLBACK_SLUG,
    })

    // Gone from the settings table, with the system row gaining both artifacts.
    await expect(row).toHaveCount(0, { timeout: UI_TIMEOUT })
    await expect(
      statusRow(ownerPage, FALLBACK_SLUG).getByTestId('status-usage')
    ).toHaveText('2')
    expect(await storedStatus()).toBe(FALLBACK_SLUG)

    // The artifact's badge reads the system default...
    await ownerPage.goto(artifactPath())
    const meta = ownerPage.getByTestId('resource-header-meta')
    await expect(meta).toContainText(FALLBACK_LABEL, { timeout: UI_TIMEOUT })
    await expect(meta).not.toContainText(STATUS_NAME)

    // ...the form select no longer offers the deleted status. `Active` being
    // selected says the select has its options, so the absence is a real one.
    await openArtifactEdit(ownerPage)
    const select = ownerPage.getByTestId('artifact-status-select')
    await expect(select).toContainText(FALLBACK_LABEL)
    await select.click()
    await expect(
      ownerPage.getByRole('option', { name: 'Archived', exact: true })
    ).toBeVisible()
    await expect(
      ownerPage.getByRole('option', { name: STATUS_NAME, exact: true })
    ).toHaveCount(0)
    await ownerPage.keyboard.press('Escape')

    // ...and neither does the list filter, whose stale `?status=` URL now
    // falls back to the unfiltered list instead of an error.
    await openArtifactList(ownerPage, `?status=${STATUS_SLUG}`)
    await expect(artifactRow(ownerPage, CONTROL_TITLE)).toBeVisible()
    await expect(artifactRow(ownerPage, ARTIFACT_TITLE)).toContainText(
      FALLBACK_LABEL
    )
    await ownerPage.getByTestId('artifact-status-filter').click()
    await expect(
      ownerPage.getByRole('option', { name: 'All statuses', exact: true })
    ).toBeVisible()
    await expect(
      ownerPage.getByRole('option', { name: STATUS_NAME, exact: true })
    ).toHaveCount(0)
  })
})
