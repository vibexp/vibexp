import {
  expect,
  test,
  type Browser,
  type BrowserContext,
  type Page,
} from '@playwright/test'

import { devLogin } from '../fixtures/auth'
import { e2eStackAvailable } from '../helpers/e2eDatabase'

/**
 * AI Summary, end to end (issue #1080, the ship gate for epic #1068).
 *
 *   a team with a model provider searches
 *     -> the AI Summary section renders collapsed and costs nothing
 *     -> expanding it generates one summary (skeleton, then the answer with a
 *        resolving citation and a Sources footer)
 *     -> collapsing, re-expanding and paging to results page 2 reuse it —
 *        still exactly ONE summary request for the search
 *     -> the header search dialog's compact summary hands over to /search
 *        without regenerating
 *     -> a settings change (style + "Results to read") reaches the next summary
 *     -> a rejected key renders the classified message, and Retry recovers
 *     -> and a team with no provider shows a member nothing and an owner the
 *        configure hint.
 *
 * ## The model provider is a stub, never a real LLM
 *
 * `docker-compose.e2e.yml` runs `backend/cmd/llm-test-provider`, a
 * deterministic OpenAI-compatible server, so the request really travels
 * browser → backend → provider and back: the LLM service, its error
 * classification and the UI are all exercised, and CI contacts no model host.
 * The stub cites the first document it receives and echoes the style and
 * document count it was sent (`Style: detailed · Documents: 3`), which is how
 * the settings assertion reads its effect from the page. It answers the key
 * `e2e-bad-key` with 401, so the unauthorized path is driven by editing the
 * provider rather than by toggling shared stub state a retry could race.
 *
 * ## Why the request count is asserted in the browser
 *
 * "Generated once per search" is a client contract: the summary is cached per
 * search identity (`summaryKey` deliberately excludes the page) in a store that
 * outlives the collapsible's unmounting children. Breaking it silently
 * multiplies every team's LLM spend and no jsdom test can see it, so every
 * `POST …/search/summary` each browser context issues is counted.
 *
 * The cache lives for one page load, so a test that needs a fresh summary for
 * the same query simply reloads. The expanded state is remembered in
 * localStorage, so after the first expand a reload generates on its own.
 *
 * Needs the docker e2e stack (the stub only exists there): against a bare
 * `npm run dev` it skips with a reason, unless `E2E_LLM_PROVIDER_URL` points at
 * a stub the backend can reach.
 */

const HAS_E2E_STACK = e2eStackAvailable()

/** Where the BACKEND (not the browser) reaches the stub. */
const PROVIDER_BASE_URL =
  process.env.E2E_LLM_PROVIDER_URL ?? 'http://llm-test-provider:9002/v1'
const GOOD_KEY = 'e2e-good-key'
const BAD_KEY = 'e2e-bad-key'
const STUB_MODEL = 'e2e-stub-model'

/** More than one results page (the search page shows 20 per page). */
const DOCUMENT_COUNT = 22

const stamp = Date.now()
const worker = process.env.TEST_WORKER_INDEX ?? '0'

/** Combined-stack UI budget, matching the other journeys. */
const UI_TIMEOUT = 20_000

const SUMMARY_TEXT = 'E2E stub summary of the matching documents'
const UNAUTHORIZED_MESSAGE = /The model provider rejected its credentials/
const CONFIGURE_HINT = 'Configure a model provider to enable AI Summary'

/**
 * Names and the search term are scoped per ATTEMPT: `describe.serial` re-runs
 * `beforeAll` on a retry, and a file-scoped term would match the previous
 * attempt's documents too.
 */
let QUERY: string

let ownerCtx: BrowserContext
let ownerPage: Page
let memberCtx: BrowserContext
let memberPage: Page
let bareOwnerCtx: BrowserContext
let bareOwnerPage: Page

let teamId: string
let bareTeamId: string
let providerId: string

/** Every summary request each browser context has issued, in order. */
const summaryRequests = new Map<BrowserContext, string[]>()

function countSummaryRequests(context: BrowserContext): void {
  summaryRequests.set(context, [])
  context.on('request', request => {
    if (
      request.method() === 'POST' &&
      /\/api\/v1\/[^/]+\/search\/summary$/.test(new URL(request.url()).pathname)
    ) {
      summaryRequests.get(context)?.push(request.url())
    }
  })
}

const ownerSummaryCount = () => summaryRequests.get(ownerCtx)?.length ?? 0

/**
 * A search term the full-text search tokenizes as one unique word: letters
 * only, so the run's digits cannot split it or stem it into something shared.
 */
function uniqueWord(seed: string): string {
  return `zephyr${seed.replace(/\d/g, d => 'abcdefghij'.charAt(Number(d)))}`
}

async function send(
  page: Page,
  method: 'POST' | 'PUT',
  url: string,
  data: unknown
) {
  const res = await page.request.fetch(url, { method, data })
  expect(
    res.ok(),
    `${method} ${url} failed: ${res.status()} ${await res.text()}`
  ).toBeTruthy()
  return res.json() as Promise<Record<string, unknown>>
}

async function createTeam(page: Page, name: string): Promise<string> {
  const body = await send(page, 'POST', '/api/v1/teams', {
    name,
    description: 'Created by the AI Summary e2e',
  })
  const id = ((body.team as { id?: string })?.id ?? body.id) as string
  expect(id, 'team id missing from create response').toBeTruthy()
  return id
}

/** Seed `count` artifacts in a fresh project, all matching `word`. */
async function seedArtifacts(
  page: Page,
  team: string,
  word: string,
  count: number
): Promise<void> {
  const project = await send(page, 'POST', `/api/v1/${team}/projects`, {
    name: `AI Summary E2E Project ${word}`,
    slug: `ai-summary-e2e-${word}`,
  })
  const projectId = project.id as string
  expect(projectId).toBeTruthy()
  for (let i = 1; i <= count; i++) {
    await send(page, 'POST', `/api/v1/${team}/artifacts`, {
      project_id: projectId,
      slug: `ai-summary-doc-${String(i)}-${word}`,
      title: `AI Summary document ${String(i)} ${word}`,
      content: `Document ${String(i)} about ${word}, written for the AI Summary journey.`,
    })
  }
}

/**
 * A logged-in context pinned to `team` from boot — `addInitScript` rather than
 * a post-load localStorage write, which races team hydration.
 */
async function openContext(
  browser: Browser,
  email: string,
  name: string,
  team?: string
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext()
  countSummaryRequests(context)
  const page = await context.newPage()
  await devLogin(page, email, name)
  if (team) {
    await page.addInitScript(tid => {
      localStorage.setItem('vx_current_team_id', tid)
    }, team)
  }
  return { context, page }
}

/** Open /search for `query` and wait until the results have rendered. */
async function openSearch(page: Page, query: string): Promise<void> {
  await page.goto(`/search?q=${encodeURIComponent(query)}`)
  await expect(
    page.getByText(`AI Summary document`, { exact: false }).first()
  ).toBeVisible({ timeout: UI_TIMEOUT })
}

const summaryTrigger = (page: Page) =>
  page.getByRole('button', { name: 'AI Summary', exact: true })

async function setProviderKey(key: string): Promise<void> {
  await send(
    ownerPage,
    'PUT',
    `/api/v1/${teamId}/model-providers/${providerId}`,
    {
      api_key: key,
    }
  )
}

test.describe.serial('AI Summary journey', () => {
  test.describe.configure({ timeout: 180_000 })

  // Skipping is for the developer who pointed Playwright at `npm run dev`. It
  // must NEVER apply under CI: `make e2e` always has the stack (and the stub),
  // so a silent skip there would leave the epic's ship gate green while
  // testing nothing.
  test.skip(
    !HAS_E2E_STACK && !process.env.E2E_LLM_PROVIDER_URL && !process.env.CI,
    'needs the docker e2e stack (make e2e): the model provider is its llm-test-provider stub'
  )

  test.beforeAll(async ({ browser }, testInfo) => {
    const run = `${worker}-${stamp}-${String(testInfo.retry)}`
    QUERY = uniqueWord(run.replace(/-/g, ''))
    const ownerEmail = `e2e_ai_summary_owner_${run}@example.com`
    const memberEmail = `e2e_ai_summary_member_${run}@example.com`

    // Log in once to create the teams, then pin a context per team.
    const setup = await openContext(browser, ownerEmail, 'Summary Owner')
    teamId = await createTeam(setup.page, `AI Summary Team ${run}`)
    bareTeamId = await createTeam(setup.page, `AI Summary Bare Team ${run}`)

    await seedArtifacts(setup.page, teamId, QUERY, DOCUMENT_COUNT)
    // The provider-less team needs results too: the section's state rides on
    // the search response.
    await seedArtifacts(setup.page, bareTeamId, QUERY, 1)

    const provider = await send(
      setup.page,
      'POST',
      `/api/v1/${teamId}/model-providers`,
      {
        name: `E2E Stub Provider ${run}`,
        provider_type: 'openai_compatible',
        model: STUB_MODEL,
        base_url: PROVIDER_BASE_URL,
        api_key: GOOD_KEY,
        is_default: true,
      }
    )
    providerId = provider.id as string
    expect(providerId).toBeTruthy()

    // A plain member of the provider-less team.
    await send(setup.page, 'POST', `/api/v1/teams/${bareTeamId}/invitations`, {
      emails: [memberEmail],
      role: 'member',
    })
    await setup.context.close()
    summaryRequests.delete(setup.context)

    ;({ context: ownerCtx, page: ownerPage } = await openContext(
      browser,
      ownerEmail,
      'Summary Owner',
      teamId
    ))
    ;({ context: bareOwnerCtx, page: bareOwnerPage } = await openContext(
      browser,
      ownerEmail,
      'Summary Owner',
      bareTeamId
    ))
    ;({ context: memberCtx, page: memberPage } = await openContext(
      browser,
      memberEmail,
      'Summary Member',
      bareTeamId
    ))

    const pending = await memberPage.request.get('/api/v1/invitations/pending')
    const token = (
      (await pending.json()) as { invitations?: { token: string }[] }
    ).invitations?.[0]?.token
    expect(token, 'invitee has no pending invitation token').toBeTruthy()
    await memberPage.goto(`/invitations/accept/${encodeURIComponent(token!)}`)
    await memberPage
      .getByRole('button', { name: /^accept(\s+invitation)?$/i })
      .first()
      .click()
    await expect
      .poll(
        async () => {
          const res = await memberPage.request.get('/api/v1/teams')
          const body = (await res.json()) as { teams?: { id?: string }[] }
          return (body.teams ?? []).some(t => t.id === bareTeamId)
        },
        { timeout: 15_000, message: 'the invited member never joined the team' }
      )
      .toBe(true)
  })

  test.afterAll(async () => {
    await Promise.all(
      [ownerCtx, bareOwnerCtx, memberCtx].map(ctx => ctx?.close())
    )
  })

  test('search renders the section collapsed and requests nothing', async () => {
    await openSearch(ownerPage, QUERY)

    await expect(summaryTrigger(ownerPage)).toBeVisible()
    await expect(summaryTrigger(ownerPage)).toHaveAttribute(
      'aria-expanded',
      'false'
    )
    await expect(ownerPage.getByText(SUMMARY_TEXT)).toHaveCount(0)
    expect(ownerSummaryCount()).toBe(0)
  })

  test('expanding shows the skeleton, then the cited summary and its sources', async () => {
    await summaryTrigger(ownerPage).click()

    // The stub answers after a 1s delay, so the loading state is observable.
    await expect(ownerPage.getByTestId('ai-summary-skeleton')).toBeVisible()
    await expect(ownerPage.getByText(SUMMARY_TEXT)).toBeVisible({
      timeout: UI_TIMEOUT,
    })

    // The stub cited document [1]; it must resolve into a link to a source.
    const citation = ownerPage.locator('a[data-citation="1"]')
    await expect(citation).toBeVisible()
    await expect(citation).toHaveAttribute('href', /\/artifacts\//)
    await expect(
      ownerPage.getByRole('heading', { name: 'Sources' })
    ).toBeVisible()
    await expect(
      ownerPage.getByText(`Generated by ${STUB_MODEL}`)
    ).toBeVisible()

    expect(ownerSummaryCount()).toBe(1)
  })

  test('collapse, re-expand and page 2 reuse the one summary', async () => {
    await summaryTrigger(ownerPage).click()
    await expect(summaryTrigger(ownerPage)).toHaveAttribute(
      'aria-expanded',
      'false'
    )
    await expect(ownerPage.getByText(SUMMARY_TEXT)).toHaveCount(0)

    await summaryTrigger(ownerPage).click()
    await expect(ownerPage.getByText(SUMMARY_TEXT)).toBeVisible()

    await ownerPage.getByRole('button', { name: 'Next' }).click()
    await expect(ownerPage).toHaveURL(/[?&]page=2/)
    await expect(ownerPage.getByText(SUMMARY_TEXT)).toBeVisible({
      timeout: UI_TIMEOUT,
    })

    // The highest-value assertion of the epic: one search, one summary.
    expect(ownerSummaryCount()).toBe(1)
  })

  test('the header search dialog summary carries over to /search', async () => {
    // A fresh page load empties the summary cache.
    await ownerPage.goto('/')
    const before = ownerSummaryCount()

    await ownerPage.getByRole('button', { name: 'Search', exact: true }).click()
    const dialog = ownerPage.getByRole('dialog')
    await dialog.getByRole('textbox', { name: 'Search query' }).fill(QUERY)

    const trigger = dialog.getByRole('button', { name: 'AI Summary' })
    await expect(trigger).toBeVisible({ timeout: UI_TIMEOUT })
    await trigger.click()
    await expect(dialog.getByTestId('ai-summary-compact')).toContainText(
      SUMMARY_TEXT,
      { timeout: UI_TIMEOUT }
    )
    await expect(
      dialog.getByTestId('ai-summary-compact').locator('sup[data-citation="1"]')
    ).toBeVisible()
    expect(ownerSummaryCount()).toBe(before + 1)

    // "See full results" opens /search with the summary expanded, reusing the
    // dialog's answer instead of generating another.
    await dialog.getByRole('button', { name: 'See full results' }).click()
    await expect(ownerPage).toHaveURL(/\/search\?/)
    await expect(ownerPage.getByText(SUMMARY_TEXT)).toBeVisible({
      timeout: UI_TIMEOUT,
    })
    expect(ownerSummaryCount()).toBe(before + 1)
  })

  test('a settings change reaches the next summary', async () => {
    await ownerPage.goto(`/teams/${teamId}/settings/model-providers`)
    const card = ownerPage.getByTestId('ai-summary-settings')
    await expect(card).toBeVisible({ timeout: UI_TIMEOUT })

    await card.locator('#ai-summary-style').selectOption('detailed')
    await card.locator('#ai-summary-top-n').fill('3')
    await card.getByRole('button', { name: 'Save changes' }).click()
    await expect(card.getByText('Saved', { exact: true })).toBeVisible({
      timeout: UI_TIMEOUT,
    })

    // Re-run the search in a fresh page load. The section remembers it was
    // expanded, so it generates on its own, with the stored settings.
    const before = ownerSummaryCount()
    await openSearch(ownerPage, QUERY)
    await expect(
      ownerPage.getByText('Style: detailed · Documents: 3')
    ).toBeVisible({ timeout: UI_TIMEOUT })
    expect(ownerSummaryCount()).toBe(before + 1)
  })

  test('a rejected key shows the classified message, and Retry recovers', async () => {
    await setProviderKey(BAD_KEY)
    try {
      await openSearch(ownerPage, QUERY)
      const alert = ownerPage.getByRole('alert').filter({
        hasText: UNAUTHORIZED_MESSAGE,
      })
      await expect(alert).toBeVisible({ timeout: UI_TIMEOUT })
    } finally {
      await setProviderKey(GOOD_KEY)
    }

    const before = ownerSummaryCount()
    await ownerPage.getByRole('button', { name: 'Retry' }).click()
    await expect(ownerPage.getByText(SUMMARY_TEXT)).toBeVisible({
      timeout: UI_TIMEOUT,
    })
    expect(ownerSummaryCount()).toBe(before + 1)
  })

  test('without a provider, an owner sees the configure hint', async () => {
    await openSearch(bareOwnerPage, QUERY)
    const hint = bareOwnerPage.getByRole('link', { name: CONFIGURE_HINT })
    await expect(hint).toBeVisible()
    await expect(hint).toHaveAttribute(
      'href',
      `/teams/${bareTeamId}/settings/model-providers`
    )
    await expect(summaryTrigger(bareOwnerPage)).toHaveCount(0)
    expect(summaryRequests.get(bareOwnerCtx)).toHaveLength(0)
  })

  test('without a provider, a member sees nothing at all', async () => {
    await openSearch(memberPage, QUERY)
    await expect(memberPage.getByText(CONFIGURE_HINT)).toHaveCount(0)
    await expect(summaryTrigger(memberPage)).toHaveCount(0)
    expect(summaryRequests.get(memberCtx)).toHaveLength(0)
  })
})
