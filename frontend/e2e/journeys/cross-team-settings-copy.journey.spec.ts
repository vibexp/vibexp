import { expect, test, type BrowserContext, type Page } from '@playwright/test'

import { devLogin } from '../fixtures/auth'

/**
 * Cross-team settings copy, end to end (issue #838, the ship gate for epic
 * #827). One owner, two teams, every copy surface the epic added:
 *
 *   a model provider, an embedding provider and a custom artifact type
 *   configured in team A (#830/#831/#829)
 *     -> each copied into team B through the settings-page dialogs
 *        (#833/#834/#835), with the credential moving server-side
 *     -> the destination reports `has_api_key: true` while the key value itself
 *        never appears in any response the run observed
 *     -> a type slug team B already owns comes back SKIPPED, not failed
 *     -> Settings › Audit lists all three, attributed to the actor and the
 *        source team (#832/#836)
 *     -> and a plain member of team B is offered no copy action at all.
 *
 * ## Why these assertions need an e2e
 *
 * Two of the epic's properties cannot be shown from unit tests. The first is
 * that a credential crosses a team boundary **without ever being serialized**:
 * that is a claim about every response body in a whole session, not about one
 * handler, so it is asserted by recording every `/api/v1/` body the run sees —
 * both the browser's and this spec's own API calls — and searching all of them
 * for the key. The second is that the audit trail actually records the copy,
 * which spans three services, a migration and a page.
 *
 * ## How this stays deterministic
 *
 * Nothing waits on wall-clock time and nothing is conditional. The destination
 * team starts empty, so the embedding copy's activation verdict
 * (`becomes_active`, computed server-side from "the default one, else the most
 * recently updated one") is necessarily true and its dialog necessarily
 * appears — it is asserted, not probed. The type collision is seeded before the
 * copy rather than discovered. Scaffolding (teams, providers, types, the
 * invitation) goes through the API because it is not what is under test; every
 * copy is driven through the UI because the dialogs are.
 */

const stamp = Date.now()
const worker = process.env.TEST_WORKER_INDEX ?? '0'

const OWNER_NAME = 'Copy Owner'
const MEMBER_NAME = 'Copy Member'

const MODEL_PROVIDER_MODEL = 'gpt-4o-mini'
const EMBEDDING_PROVIDER_MODEL = 'text-embedding-3-small'

/**
 * The one string that must never be serialized. Distinctive on purpose: the
 * final assertion is a substring search across every recorded body, so a value
 * that could occur incidentally would make a pass meaningless. Deliberately NOT
 * per-attempt — every retry seeds the same key, so the search still covers the
 * traffic of the attempts before it.
 */
const SOURCE_API_KEY = `sk-e2e-must-not-leak-${worker}-${stamp}`

/**
 * Everything a locator matches by NAME is scoped per ATTEMPT, not per file.
 *
 * `describe.serial` re-runs `beforeAll` on a retry, so a second attempt seeds a
 * second pair of teams. With a file-scoped name the source-team picker would
 * then offer two identically named options and the retry would fail on an
 * ambiguity that has nothing to do with the feature — a red ship gate caused by
 * the harness. These are assigned in `beforeAll` from the attempt index.
 */
let SOURCE_TEAM_NAME: string
let DEST_TEAM_NAME: string
let MODEL_PROVIDER_NAME: string
let EMBEDDING_PROVIDER_NAME: string
/** Present in BOTH teams before the copy runs — this is the skip under test. */
let SHARED_TYPE_SLUG: string
let SHARED_TYPE_NAME: string
/** Present only in the source team — this is the one the copy must add. */
let SOURCE_ONLY_TYPE_SLUG: string
let SOURCE_ONLY_TYPE_NAME: string

/** Combined-stack UI budget, matching the other journeys. */
const UI_TIMEOUT = 20_000

let ownerCtx: BrowserContext
let memberCtx: BrowserContext
let ownerPage: Page
let memberPage: Page

let sourceTeamId: string
let destTeamId: string
let sourceModelProviderId: string
let sourceEmbeddingProviderId: string

/**
 * Every `/api/v1/` response body the run has seen, from both halves of the
 * traffic — see {@link recordApiResponses} and {@link postJson}.
 */
const observedBodies: string[] = []

/**
 * Reading a response body is asynchronous, so a body can still be in flight when
 * the search runs — and a body that has not landed yet is a body the search
 * cannot fail on. Every read is tracked here and awaited before the assertion,
 * so "we found no key" can never mean "we had not finished looking".
 */
const bodyReads: Promise<void>[] = []

/**
 * Record the browser's own API traffic.
 *
 * Bound to the CONTEXT rather than a page so it covers anything the app opens,
 * and registered before the first navigation so nothing is missed. A body that
 * cannot be read (a redirect, an aborted request) is skipped rather than
 * failing the run: the assertion this feeds is "none of what we saw carried the
 * key", and it is paired with a check that we saw a substantial amount.
 */
function recordApiResponses(context: BrowserContext): void {
  context.on('response', response => {
    if (!response.url().includes('/api/v1/')) return
    bodyReads.push(
      response
        .text()
        .then(body => {
          observedBodies.push(body)
        })
        .catch(() => {
          /* body unavailable — nothing to inspect */
        })
    )
  })
}

/**
 * Requests made through `page.request` do NOT surface as context `response`
 * events (they bypass the page's network stack entirely), so this spec's own
 * API calls are recorded here instead. Without that, the seeding and
 * verification traffic — which is exactly where a leaked key would show up —
 * would sit outside the search.
 */
async function postJson(page: Page, url: string, data: unknown) {
  const res = await page.request.post(url, { data })
  const text = await res.text()
  observedBodies.push(text)
  expect(res.ok(), `POST ${url} failed: ${res.status()} ${text}`).toBeTruthy()
  return JSON.parse(text) as Record<string, unknown>
}

async function getJson(page: Page, url: string) {
  const res = await page.request.get(url)
  const text = await res.text()
  observedBodies.push(text)
  expect(res.ok(), `GET ${url} failed: ${res.status()} ${text}`).toBeTruthy()
  return JSON.parse(text) as unknown
}

/**
 * Pin a page's active team from boot. `addInitScript` rather than a post-load
 * localStorage write, which races team hydration — the settings pages read
 * `useTeam().teams` for the source-team picker, and under load a late write
 * leaves the picker empty.
 */
async function pinTeam(page: Page, id: string) {
  await page.addInitScript(tid => {
    localStorage.setItem('vx_current_team_id', tid)
  }, id)
}

interface ProviderRow {
  id: string
  name: string
  has_api_key: boolean
  is_default: boolean
}

/** The destination team's providers, as the API reports them. */
async function destProviders(kind: 'model' | 'embedding') {
  return (await getJson(
    ownerPage,
    `/api/v1/${destTeamId}/${kind}-providers`
  )) as ProviderRow[]
}

/** Open a settings page in the destination team and wait for its own h1. */
async function openDestSettings(
  page: Page,
  path: string,
  heading: string
): Promise<void> {
  await page.goto(`/teams/${destTeamId}/settings/${path}`)
  await expect(
    page.getByRole('heading', { level: 1, name: heading })
  ).toBeVisible({ timeout: UI_TIMEOUT })
}

/** Pick the source team in the shared CopyFromTeamDialog. */
async function chooseSourceTeam(page: Page): Promise<void> {
  await page.getByTestId('source-team-picker').click()
  await page
    .getByTestId('source-team-option')
    .filter({ hasText: SOURCE_TEAM_NAME })
    .click()
}

test.describe.serial('Cross-team settings copy journey', () => {
  test.describe.configure({ timeout: 180_000 })

  test.beforeAll(async ({ browser }, testInfo) => {
    // Fresh identities per attempt too, so the owner's team list is exactly
    // "personal + source + destination" and the picker can never be ambiguous.
    const run = `${worker}-${stamp}-${String(testInfo.retry)}`
    const uniqueEmail = (prefix: string) => `${prefix}_${run}@example.com`
    SOURCE_TEAM_NAME = `Copy Source Team ${run}`
    DEST_TEAM_NAME = `Copy Destination Team ${run}`
    MODEL_PROVIDER_NAME = `Copy Model Provider ${run}`
    EMBEDDING_PROVIDER_NAME = `Copy Embedding Provider ${run}`
    SHARED_TYPE_SLUG = `shared-type-${run}`
    SHARED_TYPE_NAME = `Shared Type ${run}`
    SOURCE_ONLY_TYPE_SLUG = `source-only-type-${run}`
    SOURCE_ONLY_TYPE_NAME = `Source Only Type ${run}`

    ownerCtx = await browser.newContext()
    recordApiResponses(ownerCtx)
    ownerPage = await ownerCtx.newPage()
    await devLogin(ownerPage, uniqueEmail('e2e_copy_owner'), OWNER_NAME)

    // Two teams, one owner. Both are created by this user, so the owner holds
    // `team.update` on each — which is what the provider copies require on BOTH
    // sides (#827 decision 3).
    const source = await postJson(ownerPage, '/api/v1/teams', {
      name: SOURCE_TEAM_NAME,
      description: 'Source of the cross-team copy e2e',
    })
    sourceTeamId = ((source.team as { id?: string })?.id ??
      (source.id as string)) as string
    expect(
      sourceTeamId,
      'source team id missing from create response'
    ).toBeTruthy()

    const dest = await postJson(ownerPage, '/api/v1/teams', {
      name: DEST_TEAM_NAME,
      description: 'Destination of the cross-team copy e2e',
    })
    destTeamId = ((dest.team as { id?: string })?.id ??
      (dest.id as string)) as string
    expect(
      destTeamId,
      'destination team id missing from create response'
    ).toBeTruthy()

    // --- Source team configuration (scaffolding, not the behaviour under test).
    const modelProvider = await postJson(
      ownerPage,
      `/api/v1/${sourceTeamId}/model-providers`,
      {
        name: MODEL_PROVIDER_NAME,
        provider_type: 'openai_compatible',
        model: MODEL_PROVIDER_MODEL,
        base_url: 'https://api.openai.com/v1',
        api_key: SOURCE_API_KEY,
      }
    )
    sourceModelProviderId = modelProvider.id as string
    expect(sourceModelProviderId).toBeTruthy()

    const embeddingProvider = await postJson(
      ownerPage,
      `/api/v1/${sourceTeamId}/embedding-providers`,
      {
        name: EMBEDDING_PROVIDER_NAME,
        provider_type: 'openai',
        model: EMBEDDING_PROVIDER_MODEL,
        base_url: 'https://api.openai.com/v1',
        api_key: SOURCE_API_KEY,
      }
    )
    sourceEmbeddingProviderId = embeddingProvider.id as string
    expect(sourceEmbeddingProviderId).toBeTruthy()

    for (const type of [
      { slug: SHARED_TYPE_SLUG, name: SHARED_TYPE_NAME },
      { slug: SOURCE_ONLY_TYPE_SLUG, name: SOURCE_ONLY_TYPE_NAME },
    ]) {
      await postJson(ownerPage, `/api/v1/${sourceTeamId}/types`, {
        resource_type: 'artifacts',
        ...type,
      })
    }

    // The collision, seeded deliberately: the destination already owns one of
    // the source's slugs, so the copy has something to skip.
    await postJson(ownerPage, `/api/v1/${destTeamId}/types`, {
      resource_type: 'artifacts',
      slug: SHARED_TYPE_SLUG,
      name: SHARED_TYPE_NAME,
    })

    // The destination starts with no providers at all — asserted rather than
    // assumed, because "the copy landed" and "it was already there" are
    // indistinguishable afterwards.
    expect(await destProviders('model')).toHaveLength(0)
    expect(await destProviders('embedding')).toHaveLength(0)

    // --- A plain member of the destination team, for the permission check.
    const memberEmail = uniqueEmail('e2e_copy_member')
    // Invitations are nested under /teams/ (unlike the team-scoped resources
    // above, which are /api/v1/{teamId}/...).
    await postJson(ownerPage, `/api/v1/teams/${destTeamId}/invitations`, {
      emails: [memberEmail],
      role: 'member',
    })

    memberCtx = await browser.newContext()
    recordApiResponses(memberCtx)
    memberPage = await memberCtx.newPage()
    await devLogin(memberPage, memberEmail, MEMBER_NAME)
    const pending = (await getJson(
      memberPage,
      '/api/v1/invitations/pending'
    )) as { invitations?: { token: string }[] }
    const token = pending.invitations?.[0]?.token
    expect(token, 'invitee has no pending invitation token').toBeTruthy()
    await memberPage.goto(`/invitations/accept/${encodeURIComponent(token!)}`)
    await memberPage
      .getByRole('button', { name: /^accept(\s+invitation)?$/i })
      .first()
      .click()
    // Confirm real membership before relying on it.
    await expect
      .poll(
        async () => {
          const res = await memberPage.request.get('/api/v1/teams')
          const body = (await res.json()) as { teams?: { id?: string }[] }
          return (body.teams ?? []).some(t => t.id === destTeamId)
        },
        {
          timeout: 15_000,
          message: 'the invited member never joined the destination team',
        }
      )
      .toBe(true)

    // Both actors work inside the destination team from here on.
    await pinTeam(ownerPage, destTeamId)
    await pinTeam(memberPage, destTeamId)
  })

  // Optional chaining is not defensive noise: a `beforeAll` that fails partway
  // leaves the later contexts unassigned, and an unguarded `close()` would then
  // throw a TypeError that REPLACES the real setup failure in the report.
  test.afterAll(async () => {
    await memberCtx?.close()
    await ownerCtx?.close()
  })

  test('the owner copies a model provider into the second team', async () => {
    await openDestSettings(ownerPage, 'model-providers', 'Model Providers')

    await ownerPage.getByTestId('copy-model-provider-button').click()
    await chooseSourceTeam(ownerPage)

    // The copy moves exactly one provider, so the preview is also the picker.
    const sourceProvider = ownerPage.getByTestId('copy-source-provider')
    await expect(sourceProvider).toHaveCount(1, { timeout: UI_TIMEOUT })
    await expect(ownerPage.getByTestId('copy-preview')).toContainText(
      MODEL_PROVIDER_NAME
    )
    await sourceProvider.check()
    await ownerPage.getByTestId('confirm-copy-from-team-button').click()

    // Copy mode of the provider dialog: the trust decision is stated, and the
    // key is a read-only statement of intent rather than an input — there is
    // nothing for the SPA to fill in, because it never holds the key.
    await expect(ownerPage.getByTestId('copy-credential-warning')).toBeVisible({
      timeout: UI_TIMEOUT,
    })
    await expect(ownerPage.getByTestId('copy-api-key-field')).toHaveValue(
      `Will be copied from ${SOURCE_TEAM_NAME}`
    )
    await ownerPage.getByRole('button', { name: 'Copy provider' }).click()

    await expect(
      ownerPage.getByText(MODEL_PROVIDER_NAME, { exact: true })
    ).toBeVisible({ timeout: UI_TIMEOUT })

    // The credential travelled: the destination reports it holds one, and the
    // copy landed non-default (#827 decision 6) rather than displacing anything.
    const copied = await destProviders('model')
    expect(copied).toHaveLength(1)
    expect(copied[0].id).not.toBe(sourceModelProviderId)
    expect(copied[0].has_api_key).toBe(true)
    expect(copied[0].is_default).toBe(false)
  })

  test('the owner copies an embedding provider and is warned it became active', async () => {
    await openDestSettings(
      ownerPage,
      'embedding-providers',
      'Embedding Providers'
    )

    await ownerPage.getByTestId('copy-embedding-provider-button').click()
    await chooseSourceTeam(ownerPage)

    const sourceProvider = ownerPage.getByTestId('copy-source-provider')
    await expect(sourceProvider).toHaveCount(1, { timeout: UI_TIMEOUT })
    await sourceProvider.check()
    await ownerPage.getByTestId('confirm-copy-from-team-button').click()

    await expect(ownerPage.getByTestId('copy-api-key-field')).toHaveValue(
      `Will be copied from ${SOURCE_TEAM_NAME}`,
      { timeout: UI_TIMEOUT }
    )
    await ownerPage.getByRole('button', { name: 'Copy provider' }).click()

    // The destination had no embedding provider, so the copy is necessarily the
    // team's active one — `becomes_active` is the server's verdict and the
    // dialog is unconditional on it. Declining leaves the copy in place; the
    // re-embed is the remedy on offer, not part of the copy.
    const activation = ownerPage.getByRole('alertdialog')
    await expect(activation).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(activation).toContainText(
      `Re-embed ${DEST_TEAM_NAME}'s resources?`
    )
    await activation.getByRole('button', { name: 'Not now' }).click()

    await expect(
      ownerPage.getByText(EMBEDDING_PROVIDER_NAME, { exact: true })
    ).toBeVisible({ timeout: UI_TIMEOUT })

    const copied = await destProviders('embedding')
    expect(copied).toHaveLength(1)
    expect(copied[0].id).not.toBe(sourceEmbeddingProviderId)
    expect(copied[0].has_api_key).toBe(true)
    expect(copied[0].is_default).toBe(false)
  })

  test('copying the artifact types skips the slug the destination already owns', async () => {
    await openDestSettings(ownerPage, 'customization', 'Customization')

    await ownerPage.getByTestId('copy-types-button').click()
    await chooseSourceTeam(ownerPage)

    // The preview states the skip BEFORE the copy runs — a skip is a normal
    // outcome the user is told about, not an error they discover afterwards.
    const preview = ownerPage.getByTestId('copy-preview')
    await expect(preview).toContainText(/1 type will be added/i, {
      timeout: UI_TIMEOUT,
    })
    await expect(preview).toContainText(
      /1 already exists here and will be skipped/i
    )
    await expect(ownerPage.getByTestId('copy-preview-add')).toHaveText([
      SOURCE_ONLY_TYPE_NAME,
    ])

    // The response is the contract: a collision is reported in `skipped`, with
    // a 2xx. Armed before the click so the body cannot be missed.
    const copyResponse = ownerPage.waitForResponse(
      response =>
        response.url().includes('/settings/types/copy') &&
        response.request().method() === 'POST'
    )
    await ownerPage.getByTestId('confirm-copy-from-team-button').click()

    const response = await copyResponse
    expect(response.status()).toBe(200)
    const result = (await response.json()) as {
      added_count: number
      skipped_count: number
      added: { slug: string }[]
      skipped: { resource_type: string; slug: string }[]
    }
    expect(result.added_count).toBe(1)
    expect(result.added.map(t => t.slug)).toEqual([SOURCE_ONLY_TYPE_SLUG])
    expect(result.skipped_count).toBe(1)
    expect(result.skipped).toEqual([
      { resource_type: 'artifacts', slug: SHARED_TYPE_SLUG },
    ])

    // And the destination ends up with both, the pre-existing one untouched.
    const types = (await getJson(
      ownerPage,
      `/api/v1/${destTeamId}/types?resource_type=artifacts`
    )) as { types: { slug: string; is_system?: boolean }[] }
    const custom = types.types
      .filter(t => t.is_system !== true)
      .map(t => t.slug)
      .sort((a, b) => a.localeCompare(b))
    expect(custom).toEqual(
      [SHARED_TYPE_SLUG, SOURCE_ONLY_TYPE_SLUG].sort((a, b) =>
        a.localeCompare(b)
      )
    )
  })

  test('no response body in the run ever carried the API key', async () => {
    // Settle every body still being read, so the search below runs against all
    // of the traffic rather than whatever happened to have arrived.
    await Promise.allSettled(bodyReads)

    // The positive companion: without it this passes just as happily on a run
    // that recorded nothing at all. Both halves of the traffic must be present
    // — the browser's (which is where `has_api_key` is rendered from) and this
    // spec's own seeding calls (which is where the key was sent).
    expect(
      observedBodies.length,
      'the response recorder captured nothing — the assertion below would be vacuous'
    ).toBeGreaterThan(20)
    expect(
      observedBodies.filter(body => body.includes('"has_api_key"')).length,
      'no recorded body carried has_api_key, so no provider payload was observed'
    ).toBeGreaterThan(0)

    const leaked = observedBodies.filter(body => body.includes(SOURCE_API_KEY))
    expect(
      leaked,
      `the API key appeared in ${String(leaked.length)} response body/bodies`
    ).toEqual([])
  })

  test('the audit tab lists every copy with its actor and source team', async () => {
    await openDestSettings(ownerPage, 'audit', 'Audit')

    const rows = ownerPage.getByTestId('settings-audit-row')
    await expect(rows).toHaveCount(3, { timeout: UI_TIMEOUT })

    // Every entry names who did it and where it came from.
    for (const surface of [
      'Model provider',
      'Embedding provider',
      'Artifact types',
    ]) {
      const row = rows.filter({ hasText: surface })
      await expect(row).toHaveCount(1)
      await expect(row).toContainText(OWNER_NAME)
      await expect(row).toContainText(`from ${SOURCE_TEAM_NAME}`)
    }

    // The two provider copies carried a credential and say so; the types copy
    // did not, so the badge is what distinguishes them.
    await expect(ownerPage.getByTestId('carried-credential')).toHaveCount(2)
    await expect(rows.filter({ hasText: 'Model provider' })).toContainText(
      MODEL_PROVIDER_NAME
    )
    await expect(rows.filter({ hasText: 'Artifact types' })).toContainText(
      `1 type: ${SOURCE_ONLY_TYPE_SLUG}`
    )
  })

  test('a member of the destination team is offered no copy action', async () => {
    // The copy requires `team.update` on both teams, which a member holds on
    // neither. The owner's runs above are the positive companion: the same
    // buttons were present and clickable for them on these same pages.
    await openDestSettings(memberPage, 'model-providers', 'Model Providers')
    await expect(
      memberPage.getByRole('button', { name: /add provider/i }).first()
    ).toBeVisible()
    await expect(
      memberPage.getByTestId('copy-model-provider-button')
    ).toHaveCount(0)
    await expect(
      memberPage.getByTestId('copy-model-provider-button-empty')
    ).toHaveCount(0)

    await openDestSettings(
      memberPage,
      'embedding-providers',
      'Embedding Providers'
    )
    await expect(
      memberPage.getByRole('button', { name: /add provider/i }).first()
    ).toBeVisible()
    await expect(
      memberPage.getByTestId('copy-embedding-provider-button')
    ).toHaveCount(0)
    await expect(
      memberPage.getByTestId('copy-embedding-provider-button-empty')
    ).toHaveCount(0)
  })
})
