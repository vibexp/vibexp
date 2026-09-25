import { readFileSync } from 'node:fs'

import {
  expect,
  test,
  type Browser,
  type BrowserContext,
  type Download,
  type Page,
} from '@playwright/test'

import { ADMIN_EMAIL } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'

/**
 * Admin panel v3, end to end (issue #1151, the ship gate for epic #1131).
 *
 * One seeded dataset — three users, two shared teams, three projects with a
 * varied resource mix, and one team carrying real configuration (a model
 * provider and an email provider holding a known secret, search / AI summary /
 * freshness settings, a custom artifact type) — driven through every v3 admin
 * surface as the instance admin:
 *
 *   users / teams / projects advanced filters (URL state survives a reload and
 *   a back navigation) -> sort by a count column -> save, apply and delete a
 *   filter preset -> CSV export of the filtered set -> user detail tabs ->
 *   team configuration tabs -> project detail charts and configuration.
 *
 * ## The session-wide assertions
 *
 * Two of the epic's promises are claims about the whole admin surface rather
 * than about one handler, so they are asserted over everything the run saw:
 *
 * - **Redaction.** Every `/api/v1/` response body — the admin browser's, the
 *   seeding users' and this spec's own `page.request` calls — is recorded, and
 *   the seeded secret must appear in none of them nor in any rendered admin
 *   page. Admin payloads must also carry no secret-bearing key at all
 *   (`api_key`, `secret`, `webhook_url`, `webhook_secret`, `last_error`).
 *   Paired with a check that the recorder saw an admin config payload carrying
 *   `has_api_key`, so a run that recorded nothing cannot pass.
 * - **Counts only, never titles.** No seeded resource title appears in any
 *   `/api/v1/admin/` response nor on any visited `/admin/*` page.
 *
 * ## Determinism in a shared database
 *
 * Other specs create users and teams in the same database, so no assertion
 * reads a global total: every list is first narrowed to this attempt's rows by
 * searching for {@link TAG}, and only then filtered further. Names are scoped
 * per ATTEMPT (`describe.serial` re-runs `beforeAll` on a retry).
 */

const stamp = Date.now()
const worker = process.env.TEST_WORKER_INDEX ?? '0'

/** Combined-stack UI budget, matching the other journeys. */
const UI_TIMEOUT = 20_000

/**
 * The one string that must never be serialized or rendered. Deliberately NOT
 * per-attempt, so the search on a retry still covers the earlier traffic.
 */
const SECRET = `e2e-admin-secret-${worker}-${stamp}`

/** Presets this spec saves; pruned in `beforeAll` so a failed run cannot pile up. */
const PRESET_PREFIX = 'E2E admin v3 preset'

/** A letters-and-digits token every seeded name, slug and email carries. */
let TAG: string
let PRESET_NAME: string

interface SeedUser {
  email: string
  name: string
}
let userA: SeedUser
let userB: SeedUser
let userC: SeedUser

let alphaTeamName: string
let betaTeamName: string
let alphaTeamId: string
let alphaOneSlug: string
let alphaTwoSlug: string
let betaOneSlug: string
let modelProviderName: string
let customTypeName: string

/** Every seeded resource title — none may reach an admin payload or page. */
const seededTitles: string[] = []

interface Observed {
  url: string
  body: string
}

/** Every `/api/v1/` response body the run has seen. */
const observed: Observed[] = []
/** Pending body reads, settled before the search so it never runs early. */
const bodyReads: Promise<void>[] = []
/** `page.content()` of every admin page the journey asserted on. */
const adminPages: { url: string; html: string }[] = []

const contexts: BrowserContext[] = []
let adminPage: Page

/**
 * Record a context's own API traffic. Bound to the context and registered
 * before the first navigation, so nothing the app requests is missed.
 */
function recordApiResponses(context: BrowserContext): void {
  context.on('response', response => {
    const url = response.url()
    if (!url.includes('/api/v1/')) return
    bodyReads.push(
      response
        .text()
        .then(body => {
          observed.push({ url, body })
        })
        .catch(() => {
          /* body unavailable (redirect, aborted) — nothing to inspect */
        })
    )
  })
}

/**
 * `page.request` bypasses the page's network stack, so its responses never
 * reach the context listener; they are recorded here instead.
 */
async function send(
  page: Page,
  method: 'GET' | 'POST' | 'PUT',
  url: string,
  data?: unknown
): Promise<Record<string, unknown>> {
  const res = await page.request.fetch(url, { method, data })
  const body = await res.text()
  observed.push({ url: res.url(), body })
  expect(res.ok(), `${method} ${url} failed: ${res.status()} ${body}`).toBe(
    true
  )
  return JSON.parse(body) as Record<string, unknown>
}

async function newUserPage(browser: Browser, user: SeedUser): Promise<Page> {
  const context = await browser.newContext({ acceptDownloads: true })
  contexts.push(context)
  recordApiResponses(context)
  const page = await context.newPage()
  await devLogin(page, user.email, user.name)
  return page
}

async function createTeam(page: Page, name: string): Promise<string> {
  const body = await send(page, 'POST', '/api/v1/teams', {
    name,
    description: 'Seeded by the admin panel v3 e2e',
  })
  const id = ((body.team as { id?: string } | undefined)?.id ??
    body.id) as string
  expect(id, 'team id missing from create response').toBeTruthy()
  return id
}

interface ResourceMix {
  prompts: number
  memories?: number
  artifacts?: number
  blueprints?: number
}

/** A project in `team` holding `mix` resources, each with a distinctive title. */
async function seedProject(
  page: Page,
  team: string,
  slug: string,
  mix: ResourceMix
): Promise<void> {
  const project = await send(page, 'POST', `/api/v1/${team}/projects`, {
    name: `Admin V3 ${slug}`,
    slug,
  })
  const projectId = project.id as string
  expect(projectId).toBeTruthy()

  const title = (kind: string, i: number) => {
    // Prompt names cap at 50 characters, so the title stays short.
    const t = `Hidden ${kind} ${String(i)} ${slug}`
    seededTitles.push(t)
    return t
  }
  for (let i = 1; i <= mix.prompts; i++) {
    await send(page, 'POST', `/api/v1/${team}/prompts`, {
      project_id: projectId,
      name: title('prompt', i),
      slug: `p${String(i)}-${slug}`,
      body: 'Seeded prompt body.',
    })
  }
  for (let i = 1; i <= (mix.memories ?? 0); i++) {
    await send(page, 'POST', `/api/v1/${team}/memories`, {
      project_id: projectId,
      title: title('memory', i),
      text: 'Seeded memory text.',
    })
  }
  for (let i = 1; i <= (mix.artifacts ?? 0); i++) {
    await send(page, 'POST', `/api/v1/${team}/artifacts`, {
      project_id: projectId,
      slug: `a${String(i)}-${slug}`,
      title: title('artifact', i),
      content: 'Seeded artifact content.',
    })
  }
  for (let i = 1; i <= (mix.blueprints ?? 0); i++) {
    await send(page, 'POST', `/api/v1/${team}/blueprints`, {
      project_id: projectId,
      slug: `b${String(i)}-${slug}`,
      title: title('blueprint', i),
      content: 'Seeded blueprint content.',
    })
  }
}

/** Drop this spec's presets from earlier runs so the 20-preset cap never bites. */
async function prunePresets(page: Page, list: string): Promise<void> {
  const current = (await send(
    page,
    'GET',
    `/api/v1/admin/saved-filters/${list}`
  )) as { presets: { name: string }[]; version: number }
  const kept = current.presets.filter(p => !p.name.startsWith(PRESET_PREFIX))
  if (kept.length === current.presets.length) return
  await send(page, 'PUT', `/api/v1/admin/saved-filters/${list}`, {
    presets: kept,
    version: current.version,
  })
}

/** Keep the rendered page for the end-of-run redaction and no-title search. */
async function snapshot(page: Page): Promise<void> {
  adminPages.push({ url: page.url(), html: await page.content() })
}

const rows = (page: Page) => page.locator('tbody tr')

/** Wait until the list shows exactly these row keys, in this order. */
async function expectRows(page: Page, keys: string[]): Promise<void> {
  await expect(rows(page)).toHaveCount(keys.length, { timeout: UI_TIMEOUT })
  for (const [index, key] of keys.entries()) {
    await expect(rows(page).nth(index)).toContainText(key)
  }
}

async function openAdvanced(page: Page): Promise<void> {
  const toggle = page.getByRole('button', { name: /Advanced filters/ })
  if ((await toggle.getAttribute('aria-expanded')) !== 'true') {
    await toggle.click()
  }
}

/** Commit a range bound the way a user does: type, then Enter. */
async function setMin(page: Page, label: string, value: string) {
  const input = page.getByLabel(`${label} minimum`, { exact: true })
  await input.fill(value)
  await input.press('Enter')
}

async function search(page: Page, label: string, value: string) {
  await page.getByRole('textbox', { name: label }).fill(value)
  await expect(page).toHaveURL(new RegExp(`search=${value}`))
}

/** Export the current list; returns the file name and its CSV lines. */
async function exportCsv(
  page: Page
): Promise<{ filename: string; lines: string[] }> {
  const [download]: [Download, void] = await Promise.all([
    page.waitForEvent('download', { timeout: UI_TIMEOUT }),
    page.getByRole('button', { name: 'Export CSV' }).click(),
  ])
  const path = await download.path()
  const text = readFileSync(path, 'utf8')
  return {
    filename: download.suggestedFilename(),
    lines: text.split(/\r?\n/).filter(line => line !== ''),
  }
}

test.describe.serial('Admin panel v3 journey', () => {
  test.describe.configure({ timeout: 240_000 })

  test.beforeAll(async ({ browser }, testInfo) => {
    const run = `${worker}x${stamp.toString(36)}x${String(testInfo.retry)}`
    TAG = `avthree${run}`
    PRESET_NAME = `${PRESET_PREFIX} ${run}`
    userA = { email: `e2e_a_${TAG}@example.com`, name: `Alpha Owner ${TAG}` }
    userB = { email: `e2e_b_${TAG}@example.com`, name: `Beta Owner ${TAG}` }
    userC = { email: `e2e_c_${TAG}@example.com`, name: `Idle User ${TAG}` }
    alphaTeamName = `Alpha ${TAG}`
    betaTeamName = `Beta ${TAG}`
    alphaOneSlug = `alpha-one-${TAG}`
    alphaTwoSlug = `alpha-two-${TAG}`
    betaOneSlug = `beta-one-${TAG}`
    modelProviderName = `Admin V3 Provider ${TAG}`
    customTypeName = `Admin V3 Type ${TAG}`

    // --- User A: team Alpha, two projects, and every piece of team config.
    const pageA = await newUserPage(browser, userA)
    alphaTeamId = await createTeam(pageA, alphaTeamName)
    await seedProject(pageA, alphaTeamId, alphaOneSlug, {
      prompts: 3,
      memories: 1,
      artifacts: 1,
      blueprints: 1,
    })
    await seedProject(pageA, alphaTeamId, alphaTwoSlug, { prompts: 1 })

    const provider = await send(
      pageA,
      'POST',
      `/api/v1/${alphaTeamId}/model-providers`,
      {
        name: modelProviderName,
        provider_type: 'openai_compatible',
        model: 'gpt-4o-mini',
        base_url: 'https://api.openai.com/v1',
        api_key: SECRET,
        is_default: true,
      }
    )
    const providerId = provider.id as string
    expect(providerId).toBeTruthy()
    await send(pageA, 'PUT', `/api/v1/${alphaTeamId}/settings/email-provider`, {
      provider_type: 'sendgrid',
      secret: SECRET,
      from_address: 'noreply@example.com',
    })
    await send(pageA, 'PUT', `/api/v1/${alphaTeamId}/settings/search`, {
      recency_ranking_enabled: true,
      rank_weight_relevance: 1,
      rank_weight_created: 0.5,
      rank_weight_updated: 0.25,
      rank_half_life_days: 30,
    })
    await send(pageA, 'PUT', `/api/v1/${alphaTeamId}/settings/ai-summary`, {
      enabled: true,
      model_provider_id: providerId,
      top_n: 5,
      style: 'concise',
      max_output_tokens: 512,
    })
    await send(pageA, 'PUT', `/api/v1/${alphaTeamId}/settings/freshness`, {
      interval_seconds: 7200,
      reversibility_enabled: true,
    })
    await send(pageA, 'POST', `/api/v1/${alphaTeamId}/types`, {
      resource_type: 'artifacts',
      slug: `admin-v3-type-${TAG}`,
      name: customTypeName,
    })

    // --- User B: team Beta with one small project. User C creates nothing.
    const pageB = await newUserPage(browser, userB)
    const betaTeamId = await createTeam(pageB, betaTeamName)
    await seedProject(pageB, betaTeamId, betaOneSlug, { prompts: 1 })
    await newUserPage(browser, userC)

    // --- The instance admin.
    adminPage = await newUserPage(browser, {
      email: ADMIN_EMAIL,
      name: 'Admin E2E',
    })
    for (const list of ['users', 'teams', 'projects']) {
      await prunePresets(adminPage, list)
    }
  })

  // Optional chaining: a `beforeAll` failing partway leaves contexts unset.
  test.afterAll(async () => {
    for (const context of contexts) await context?.close()
  })

  test('users: advanced filter round-trips through the URL, sorts by a count, exports', async () => {
    const page = adminPage
    await page.goto('/admin/users')
    await search(page, 'Search users', TAG)
    // Narrowed to this attempt; the default sort is newest first.
    await expect(rows(page)).toHaveCount(3, { timeout: UI_TIMEOUT })

    // Sort by a count column: a new column starts descending, a second click
    // flips it. A has 4 prompts, B 1, C none — an unambiguous order.
    await page.getByRole('button', { name: 'Prompts', exact: true }).click()
    await expect(page).toHaveURL(/sort_by=prompt_count/)
    // Descending is the default, so it is left out of the URL.
    await expect(page).not.toHaveURL(/sort_order=/)
    await expectRows(page, [userA.email, userB.email, userC.email])
    await page.getByRole('button', { name: 'Prompts', exact: true }).click()
    await expect(page).toHaveURL(/sort_order=asc/)
    await expectRows(page, [userC.email, userB.email, userA.email])

    // An advanced filter lands in the URL and narrows the rows.
    await openAdvanced(page)
    await setMin(page, 'Prompts', '2')
    await expect(page).toHaveURL(/prompt_count_min=2/)
    await expectRows(page, [userA.email])
    await expect(page.getByTestId('advanced-filters-count')).toContainText('1')

    // Reload: the URL alone restores the filter, the panel and the rows.
    await page.reload()
    await expect(page).toHaveURL(/prompt_count_min=2/)
    await expectRows(page, [userA.email])
    await expect(
      page.getByLabel('Prompts minimum', { exact: true })
    ).toHaveValue('2')

    // Export the filtered set: one data row, the filter applied server-side.
    const csv = await exportCsv(page)
    expect(csv.filename).toMatch(/^admin-users-\d{8}\.csv$/)
    expect(csv.lines[0]).toMatch(/^id,email,name,/)
    expect(csv.lines).toHaveLength(2)
    expect(csv.lines[1]).toContain(userA.email)
    await snapshot(page)

    // Into the detail page and back: the list comes back filtered.
    await rows(page).first().click()
    await expect(page).toHaveURL(/\/admin\/users\/[^/?]+/)
    await page.goBack()
    await expect(page).toHaveURL(/prompt_count_min=2/)
    await expectRows(page, [userA.email])

    // And forward again to the detail page, then back once more.
    await page.goForward()
    await expect(page).toHaveURL(/\/admin\/users\/[^/?]+/)
    await page.goBack()
    await expect(page).toHaveURL(/prompt_count_min=2/)
    await expectRows(page, [userA.email])
  })

  test('users: detail tabs render for the seeded user', async () => {
    const page = adminPage
    await rows(page).first().click()
    await expect(page).toHaveURL(/\/admin\/users\/[^/?]+/)
    await expect(page.getByText(userA.email).first()).toBeVisible({
      timeout: UI_TIMEOUT,
    })

    await expect(
      page.getByRole('heading', { name: 'Resource counts' })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    await snapshot(page)

    await page.getByRole('tab', { name: 'Activity' }).click()
    await expect(page).toHaveURL(/tab=activity/)
    await expect(page.getByRole('tabpanel')).toBeVisible()
    await expect(page.getByTestId('activity-loading')).toHaveCount(0, {
      timeout: UI_TIMEOUT,
    })
    await snapshot(page)

    await page.getByRole('tab', { name: 'Teams' }).click()
    await expect(page).toHaveURL(/tab=teams/)
    await expect(
      page.getByRole('heading', { name: 'Team memberships' })
    ).toBeVisible()
    await expect(page.getByRole('tabpanel')).toContainText(alphaTeamName, {
      timeout: UI_TIMEOUT,
    })
    await snapshot(page)

    await page.getByRole('tab', { name: 'Notifications' }).click()
    await expect(page).toHaveURL(/tab=notifications/)
    await expect(page.getByText('Email categories')).toBeVisible({
      timeout: UI_TIMEOUT,
    })
    await snapshot(page)
  })

  test('teams: filters, count sort, and a saved preset round trip', async () => {
    const page = adminPage
    await page.goto('/admin/teams')
    await search(page, 'Search teams', TAG)

    // Shared teams only (the search also matches the users' personal teams by
    // owner email).
    await page.getByRole('combobox', { name: 'Team type' }).click()
    await page.getByRole('option', { name: 'Shared only' }).click()
    await expect(page).toHaveURL(/kind=shared/)
    await expect(rows(page)).toHaveCount(2, { timeout: UI_TIMEOUT })

    // Sort by project count: Alpha (2) before Beta (1), then flipped.
    await page.getByRole('button', { name: 'Projects', exact: true }).click()
    await expect(page).toHaveURL(/sort_by=project_count/)
    await expectRows(page, [alphaTeamName, betaTeamName])
    await page.getByRole('button', { name: 'Projects', exact: true }).click()
    await expect(page).toHaveURL(/sort_order=asc/)
    await expectRows(page, [betaTeamName, alphaTeamName])

    // Advanced: a count range plus a setup tri-state.
    await openAdvanced(page)
    await setMin(page, 'Projects', '2')
    await expect(page).toHaveURL(/project_count_min=2/)
    await page
      .getByRole('radiogroup', { name: 'LLM configured' })
      .getByRole('radio', { name: 'Yes' })
      .click()
    await expect(page).toHaveURL(/llm_configured=true/)
    await expectRows(page, [alphaTeamName])

    await page.reload()
    await expect(page).toHaveURL(/project_count_min=2/)
    await expect(page).toHaveURL(/llm_configured=true/)
    await expectRows(page, [alphaTeamName])

    const csv = await exportCsv(page)
    expect(csv.filename).toMatch(/^admin-teams-\d{8}\.csv$/)
    expect(csv.lines[0]).toMatch(/^id,name,slug,/)
    expect(csv.lines).toHaveLength(2)
    expect(csv.lines[1]).toContain(alphaTeamName)
    await snapshot(page)

    // Save the view as a preset.
    await page.getByRole('button', { name: /Presets/ }).click()
    await page.getByRole('menuitem', { name: 'Save current filters…' }).click()
    const saveDialog = page.getByRole('dialog', {
      name: 'Save current filters',
    })
    await saveDialog.getByLabel('Preset name').fill(PRESET_NAME)
    await saveDialog.getByRole('button', { name: 'Save preset' }).click()
    await expect(saveDialog).toHaveCount(0, { timeout: UI_TIMEOUT })

    // Clear, then apply the preset: the whole view comes back from it.
    await page.getByRole('button', { name: 'Clear filters' }).first().click()
    await expect(page).not.toHaveURL(/project_count_min/)
    await expect(page).not.toHaveURL(/kind=shared/)
    await page.getByRole('button', { name: /Presets/ }).click()
    await page.getByRole('menuitem', { name: PRESET_NAME }).click()
    await expect(page).toHaveURL(/kind=shared/)
    await expect(page).toHaveURL(/project_count_min=2/)
    await expect(page).toHaveURL(/llm_configured=true/)
    await expect(page).toHaveURL(/sort_by=project_count/)
    await expectRows(page, [alphaTeamName])

    // Delete it (two-step) through the manage dialog.
    await page.getByRole('button', { name: /Presets/ }).click()
    await page.getByRole('menuitem', { name: 'Manage presets…' }).click()
    const manage = page.getByRole('dialog', { name: 'Manage presets' })
    const presetRow = manage
      .getByRole('listitem')
      .filter({ has: page.getByLabel(`Name for preset ${PRESET_NAME}`) })
    await presetRow.getByRole('button', { name: 'Delete' }).click()
    await presetRow.getByRole('button', { name: 'Confirm delete' }).click()
    await expect(presetRow).toHaveCount(0, { timeout: UI_TIMEOUT })
    await page.keyboard.press('Escape')
    await expect
      .poll(async () => {
        const res = await page.request.get('/api/v1/admin/saved-filters/teams')
        const body = (await res.json()) as { presets: { name: string }[] }
        return body.presets.some(p => p.name === PRESET_NAME)
      })
      .toBe(false)
  })

  test('teams: every configuration tab renders, secrets as state only', async () => {
    const page = adminPage
    await page.goto(`/admin/teams/${alphaTeamId}`)
    await expect(
      page.getByRole('heading', { name: alphaTeamName })
    ).toBeVisible({ timeout: UI_TIMEOUT })

    const openTab = async (label: string, param: string) => {
      await page.getByRole('tab', { name: label, exact: true }).click()
      await expect(page).toHaveURL(new RegExp(`tab=${param}`))
    }
    const panel = page.getByRole('tabpanel')

    await openTab('Search', 'search')
    await expect(
      page.getByRole('heading', { name: 'Search ranking' })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    await snapshot(page)

    await openTab('AI summary', 'ai-summary')
    await expect(page.getByRole('heading', { name: 'AI summary' })).toBeVisible(
      { timeout: UI_TIMEOUT }
    )
    await snapshot(page)

    await openTab('Freshness', 'freshness')
    await expect(
      page.getByRole('heading', { name: 'Freshness evaluation' })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    await snapshot(page)

    // The key is shown as "configured", never as a value.
    await openTab('Model providers', 'model-providers')
    const providerRow = page
      .getByTestId('provider-row')
      .filter({ hasText: modelProviderName })
    await expect(providerRow).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(providerRow).toContainText('Configured ✓')
    await snapshot(page)

    await openTab('Embedding providers', 'embedding-providers')
    await expect(
      page.getByRole('heading', { name: 'Embedding coverage' })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    await snapshot(page)

    await openTab('Email', 'email')
    await expect(panel).toContainText('sendgrid', { timeout: UI_TIMEOUT })
    await expect(panel).toContainText('Configured ✓')
    await snapshot(page)

    await openTab('GitHub', 'github')
    await expect(page.getByRole('heading', { name: 'GitHub App' })).toBeVisible(
      { timeout: UI_TIMEOUT }
    )
    await snapshot(page)

    await openTab('Artifact types', 'artifact-types')
    await expect(
      page.getByTestId('artifact-type-row').filter({ hasText: customTypeName })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    await snapshot(page)

    // Alpha was configured directly, never copied into, so the log is empty.
    await openTab('Settings audit', 'settings-audit')
    await expect(page.getByTestId('config-empty')).toContainText(
      'Nothing has been copied into this team from another one.',
      { timeout: UI_TIMEOUT }
    )
    await snapshot(page)
  })

  test('projects: filters, export, and the project detail charts and config', async () => {
    const page = adminPage
    await page.goto('/admin/projects')
    await search(page, 'Search projects', TAG)
    await expect(rows(page)).toHaveCount(3, { timeout: UI_TIMEOUT })

    // Resources: alpha-one 6, beta-one 1, alpha-two 1 — alpha-one leads.
    await page.getByRole('button', { name: 'Resources', exact: true }).click()
    await expect(page).toHaveURL(/sort_by=total_resource_count/)
    await expect(rows(page).first()).toContainText(alphaOneSlug, {
      timeout: UI_TIMEOUT,
    })

    await openAdvanced(page)
    await page.getByLabel('Creator email').fill(userA.email)
    await page.getByLabel('Creator email').press('Enter')
    await expect(page).toHaveURL(/owner_email=/)
    await expect(rows(page)).toHaveCount(2, { timeout: UI_TIMEOUT })
    await setMin(page, 'Prompts', '2')
    await expect(page).toHaveURL(/prompt_count_min=2/)
    await expectRows(page, [alphaOneSlug])

    await page.reload()
    await expect(page).toHaveURL(/owner_email=/)
    await expect(page).toHaveURL(/prompt_count_min=2/)
    await expectRows(page, [alphaOneSlug])

    const csv = await exportCsv(page)
    expect(csv.filename).toMatch(/^admin-projects-\d{8}\.csv$/)
    expect(csv.lines[0]).toMatch(/^id,name,slug,team_id,/)
    expect(csv.lines).toHaveLength(2)
    expect(csv.lines[1]).toContain(alphaOneSlug)
    expect(csv.lines[1]).not.toContain(alphaTwoSlug)
    await snapshot(page)

    // Project detail — Overview: the breakdown and both time-series charts.
    await rows(page).first().click()
    await expect(page).toHaveURL(/\/admin\/projects\/[^/?]+/)
    await expect(page.getByRole('heading', { name: 'Breakdown' })).toBeVisible({
      timeout: UI_TIMEOUT,
    })
    await expect(
      page.getByRole('heading', { name: 'Created over time' })
    ).toBeVisible()
    await expect(
      page.getByRole('heading', { name: 'Access over time' })
    ).toBeVisible()
    // Non-empty data, not pixels: the creation chart has something to plot.
    await expect(
      page.getByText('Nothing was created in this range.')
    ).toHaveCount(0, { timeout: UI_TIMEOUT })
    await expect(
      page.getByText('This project has no resources yet.')
    ).toHaveCount(0)
    await snapshot(page)

    await page.getByRole('tab', { name: 'Configuration' }).click()
    await expect(page).toHaveURL(/tab=configuration/)
    await expect(
      page.getByRole('heading', { name: 'Rules for this project' })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(
      page.getByRole('link', { name: /Team configuration/ })
    ).toHaveAttribute('href', `/admin/teams/${alphaTeamId}?tab=freshness`)
    await snapshot(page)
  })

  test('no admin payload or page carried a secret or a resource title', async () => {
    await Promise.allSettled(bodyReads)

    const admin = observed.filter(o => o.url.includes('/api/v1/admin/'))
    // Positive companions: a recorder that saw nothing must not pass.
    expect(
      admin.length,
      'the recorder captured no admin payloads — the checks below would be vacuous'
    ).toBeGreaterThan(20)
    expect(
      admin.filter(o => o.body.includes('"has_api_key"')).length,
      'no admin config payload carrying has_api_key was observed'
    ).toBeGreaterThan(0)
    expect(adminPages.length).toBeGreaterThan(10)
    expect(seededTitles.length).toBeGreaterThan(5)

    // The secret: nowhere in any recorded body, admin or not, nor any page.
    const leakedBodies = observed
      .filter(o => o.body.includes(SECRET))
      .map(o => o.url)
    expect(leakedBodies, 'the secret appeared in these responses').toEqual([])
    const leakedPages = adminPages
      .filter(p => p.html.includes(SECRET))
      .map(p => p.url)
    expect(leakedPages, 'the secret was rendered on these pages').toEqual([])

    // No secret-bearing key in any admin payload.
    for (const key of [
      '"api_key"',
      '"secret"',
      '"webhook_url"',
      '"webhook_secret"',
      '"last_error"',
    ]) {
      const hits = admin.filter(o => o.body.includes(key)).map(o => o.url)
      expect(hits, `admin payloads carried ${key}`).toEqual([])
    }

    // Counts only, never titles.
    for (const title of seededTitles) {
      const inBodies = admin.filter(o => o.body.includes(title)).map(o => o.url)
      expect(inBodies, `"${title}" appeared in admin payloads`).toEqual([])
      const onPages = adminPages
        .filter(p => p.html.includes(title))
        .map(p => p.url)
      expect(onPages, `"${title}" was rendered on admin pages`).toEqual([])
    }
  })
})
