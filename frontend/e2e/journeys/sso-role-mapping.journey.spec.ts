import {
  type Browser,
  type BrowserContext,
  expect,
  type Page,
  test,
} from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { completeOnboarding, devLogin } from '../fixtures/auth'
import {
  expectIssuerReady,
  ISSUER_CLIENT,
  ISSUER_URL,
  refreshCalls,
  refuseAuthorization,
  refuseRefresh,
  resetIssuer,
  scriptClaim,
  scriptUser,
  signInWithProvider,
} from '../helpers/fakeIssuer'

/**
 * SSO role mapping and access re-check, signed in through a real (fake)
 * identity provider (issue #1650, epic #1640).
 *
 * Every other journey signs in with a dev login, which has no provider and no
 * refresh token, so nothing else drives the provider sign-in, the role mapping
 * (#1644), its re-application on refresh (#1648) or the access re-check
 * (#1647) in a browser. Here the e2e stack's scriptable issuer
 * (`backend/cmd/oidc-test-issuer`, see `helpers/fakeIssuer.ts`) signs in users
 * whose `groups` claim the spec chooses, and changes it, or refuses the
 * refresh or the sign-in, while they are signed in.
 *
 * 1. A role mapping set in the provider dialog makes a user whose claim holds
 *    the admin value an instance admin, shown as provider-sourced.
 * 2. A user without the value signs in but is kept out of the admin area.
 * 3. The issuer drops the value: the next re-check takes the admin area away,
 *    with no new sign-in.
 * 4. The issuer refuses the refresh (`invalid_grant`): the next re-check signs
 *    the user out.
 * 5. The issuer denies the sign-in (`access_denied`): the "No access" page,
 *    and no account or session.
 * 6. A root admin (`INSTANCE_ADMIN_EMAILS`) signing in through the provider
 *    with no roles claim at all stays an admin, across a re-check too.
 *
 * The stack sets `AUTH_SESSION_RECHECK_INTERVAL` to 10s, so a re-check is
 * waited for by polling (`RECHECK_WAIT`), never by sleeping.
 *
 * ## Sharing the stack
 *
 * Providers are instance-wide, so this is an instance journey (one at a time,
 * after the parallel projects). Each test creates its provider under a slug
 * prefix only this spec uses and deletes it, and puts the issuer back, before
 * and after it runs, pass or fail; `admin-auth-settings` filters by its own
 * prefix and never sees it.
 */

const UI_TIMEOUT = 20_000
/** Twice the stack's 10s re-check interval, plus a request's worth. */
const RECHECK_WAIT = 30_000
const PROVIDERS_PATH = '/api/v1/admin/settings/auth/providers'
const ADMINS_PATH = '/api/v1/admin/settings/auth/admins'
const SLUG_PREFIX = 'e2e-sso-'
const ADMIN_VALUE = 'vibexp-admins'
const HOME = /^https?:\/\/[^/]+\/$/

interface ProviderList {
  version: number
  providers: { id: string; slug: string }[]
}

interface Me {
  id: string
  email: string
  is_instance_admin: boolean
  is_root_instance_admin: boolean
}

interface Provider {
  slug: string
  name: string
}

async function listProviders(page: Page): Promise<ProviderList> {
  const response = await page.request.get(PROVIDERS_PATH)
  expect(response.status()).toBe(200)
  return (await response.json()) as ProviderList
}

/** Deletes every provider this spec created, on this or an earlier attempt. */
async function purgeProviders(page: Page): Promise<void> {
  for (;;) {
    const list = await listProviders(page)
    const mine = list.providers.find(p => p.slug.startsWith(SLUG_PREFIX))
    if (!mine) return
    const response = await page.request.delete(
      `${PROVIDERS_PATH}/${mine.id}?expected_version=${String(list.version)}&confirm_lockout_risk=true`
    )
    expect(response.status(), `purge of ${mine.slug}`).toBe(204)
  }
}

/** A fresh provider name for this attempt. */
function newProvider(): Provider {
  const tag = Date.now().toString(36)
  return { slug: `${SLUG_PREFIX}${tag}`, name: `E2E SSO ${tag}` }
}

/** A per-attempt user the issuer signs in as. */
function newUser(who: string): { sub: string; email: string } {
  const tag = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
  return {
    sub: `e2e-sso-${who}-${tag}`,
    email: `e2e-sso-${who}-${tag}@vibexp.test`,
  }
}

/** Creates the issuer's provider with the role mapping, through the API. */
async function createProvider(page: Page, provider: Provider): Promise<void> {
  const response = await page.request.post(PROVIDERS_PATH, {
    data: {
      type: 'oidc',
      slug: provider.slug,
      display_name: provider.name,
      issuer_url: ISSUER_URL,
      client_id: ISSUER_CLIENT.id,
      client_secret: ISSUER_CLIENT.secret,
      roles_claim: 'groups',
      admin_values: [ADMIN_VALUE],
    },
  })
  expect(response.status(), 'provider create').toBe(201)
  await expectOffered(page, provider)
}

/** Waits until the provider is healthy and offered on the sign-in page. */
async function expectOffered(page: Page, provider: Provider): Promise<void> {
  await expect
    .poll(
      async () => {
        const response = await page.request.get('/api/v1/auth/providers')
        const body = (await response.json()) as {
          providers: { name: string }[]
        }
        return body.providers.map(p => p.name)
      },
      { timeout: UI_TIMEOUT }
    )
    .toContain(provider.slug)
}

async function me(page: Page): Promise<{ status: number; body?: Me }> {
  const response = await page.request.get('/api/v1/auth/me')
  if (response.status() !== 200) return { status: response.status() }
  return { status: 200, body: (await response.json()) as Me }
}

async function freshContext(browser: Browser): Promise<BrowserContext> {
  return browser.newContext({ viewport: { width: 1920, height: 1080 } })
}

/** Signs the scripted user in, in a context of their own, and lands them. */
async function signInAs(
  browser: Browser,
  provider: Provider,
  email: string
): Promise<Page> {
  const context = await freshContext(browser)
  const page = await context.newPage()
  await signInWithProvider(page, provider.name)
  const signedIn = await me(page)
  expect(signedIn.status, 'signed in through the provider').toBe(200)
  expect(signedIn.body?.email).toBe(email)
  await completeOnboarding(page)
  return page
}

/** /admin opens the admin shell. */
async function expectAdminArea(page: Page): Promise<void> {
  await page.goto('/admin')
  await expect(page).toHaveURL(/\/admin$/)
  await expect(page.getByRole('link', { name: 'Back to app' })).toBeVisible({
    timeout: UI_TIMEOUT,
  })
  expect((await page.request.get(PROVIDERS_PATH)).status()).toBe(200)
}

/** /admin sends the user home, the menu has no entry, and the API 404s. */
async function expectNoAdminArea(page: Page): Promise<void> {
  await page.goto('/admin')
  await expect(page).toHaveURL(HOME, { timeout: UI_TIMEOUT })
  await expect(page.getByRole('link', { name: 'Back to app' })).toHaveCount(0)
  await page.getByTestId('user-menu').click()
  await expect(page.getByRole('menuitem', { name: 'Settings' })).toBeVisible()
  await expect(
    page.getByRole('menuitem', { name: 'Admin Portal' })
  ).toHaveCount(0)
  await page.keyboard.press('Escape')
  expect((await page.request.get(PROVIDERS_PATH)).status()).toBe(404)
}

/** The provider-sourced admin rows the admin API lists for email. */
async function providerAdminRows(
  page: Page,
  email: string
): Promise<{ email: string; matched_value: string }[]> {
  const response = await page.request.get(ADMINS_PATH)
  expect(response.status()).toBe(200)
  const body = (await response.json()) as {
    provider_admins: { email: string; matched_value: string }[]
  }
  return body.provider_admins.filter(a => a.email === email)
}

test.describe.serial('SSO role mapping and access re-check', () => {
  // The dev-login root admin's page, which sets up and checks from the side.
  let admin: Page
  let provider: Provider
  const opened: BrowserContext[] = []

  test.beforeEach(async ({ browser }) => {
    const context = await freshContext(browser)
    opened.push(context)
    admin = await context.newPage()
    await devLogin(admin, ADMIN_EMAIL, ADMIN_NAME)
    await expectIssuerReady(admin.request)
    await resetIssuer(admin.request)
    await purgeProviders(admin)
    provider = newProvider()
  })

  test.afterEach(async () => {
    await purgeProviders(admin)
    await resetIssuer(admin.request)
    expect(
      (await listProviders(admin)).providers.filter(p =>
        p.slug.startsWith(SLUG_PREFIX)
      ),
      'no provider of this spec is left'
    ).toEqual([])
    for (const context of opened.splice(0)) await context.close()
  })

  test('a mapping set in the dialog makes a provider admin', async ({
    browser,
  }) => {
    test.setTimeout(120_000)

    // --- The role mapping, from the provider dialog (#1645) ------------------
    await admin.goto('/admin/settings/auth')
    await expect(admin.getByTestId('auth-providers-section')).toBeVisible({
      timeout: UI_TIMEOUT,
    })
    await admin.getByRole('button', { name: 'Add provider' }).click()
    const dialog = admin.getByRole('dialog')
    await dialog.getByRole('radio', { name: 'OpenID Connect' }).check()
    await dialog.getByLabel('Slug').fill(provider.slug)
    await dialog.getByLabel('Display name').fill(provider.name)
    await dialog.getByLabel('Issuer URL').fill(ISSUER_URL)
    await dialog.getByLabel('Client ID').fill(ISSUER_CLIENT.id)
    await dialog.getByLabel('Client secret').fill(ISSUER_CLIENT.secret)
    await dialog.getByLabel('Roles claim').fill('groups')
    await dialog.getByLabel('Admin values').fill(`${ADMIN_VALUE}\nauditors`)
    await dialog.getByRole('button', { name: 'Add provider' }).click()
    await expect(dialog).toBeHidden({ timeout: UI_TIMEOUT })
    const row = admin.locator(
      `[data-testid="auth-provider-row"][data-provider-slug="${provider.slug}"]`
    )
    await expect(row.getByTestId('auth-provider-health')).toHaveText(
      'Healthy',
      { timeout: UI_TIMEOUT }
    )
    await expectOffered(admin, provider)

    // --- A user whose groups hold the admin value signs in -------------------
    const user = newUser('admin')
    await scriptUser(admin.request, {
      ...user,
      claims: { groups: ['engineering', ADMIN_VALUE] },
    })
    const page = await signInAs(browser, provider, user.email)
    opened.push(page.context())
    expect((await me(page)).body?.is_instance_admin).toBe(true)
    expect((await me(page)).body?.is_root_instance_admin).toBe(false)
    await expectAdminArea(page)
    await page.getByRole('link', { name: 'Back to app' }).click()
    await page.getByTestId('user-menu').click()
    await expect(
      page.getByRole('menuitem', { name: 'Admin Portal' })
    ).toBeVisible()
    await page.keyboard.press('Escape')

    // --- The admins list shows them as provider-sourced ----------------------
    await admin.reload()
    const providerAdmin = admin
      .getByTestId('auth-provider-admin')
      .filter({ hasText: user.email })
    await expect(providerAdmin).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(providerAdmin).toContainText('Identity provider')
    await expect(providerAdmin).toContainText(`via ${provider.name}`)
    await expect(providerAdmin).toContainText(`matched ${ADMIN_VALUE}`)
    await expect(
      admin.getByTestId('auth-db-admin').filter({ hasText: user.email })
    ).toHaveCount(0)
  })

  test('a user without the admin value is kept out of the admin area', async ({
    browser,
  }) => {
    test.setTimeout(90_000)
    await createProvider(admin, provider)

    const user = newUser('member')
    await scriptUser(admin.request, {
      ...user,
      claims: { groups: ['engineering'] },
    })
    const page = await signInAs(browser, provider, user.email)
    opened.push(page.context())

    expect((await me(page)).body?.is_instance_admin).toBe(false)
    await expectNoAdminArea(page)
    expect(await providerAdminRows(admin, user.email)).toEqual([])
  })

  test('dropping the admin value at the issuer takes the admin area away at the next re-check', async ({
    browser,
  }) => {
    test.setTimeout(150_000)
    await createProvider(admin, provider)

    const user = newUser('demoted')
    await scriptUser(admin.request, {
      ...user,
      claims: { groups: [ADMIN_VALUE] },
    })
    const page = await signInAs(browser, provider, user.email)
    opened.push(page.context())
    const signedIn = await me(page)
    expect(signedIn.body?.is_instance_admin).toBe(true)
    await expectAdminArea(page)
    expect(await providerAdminRows(admin, user.email)).toHaveLength(1)

    // The issuer no longer lists the user in the admin group.
    const refreshesBefore = await refreshCalls(admin.request)
    await scriptClaim(admin.request, 'groups', ['engineering'])

    await expect(async () => {
      const now = await me(page)
      expect(now.status, 'still signed in').toBe(200)
      expect(now.body?.is_instance_admin).toBe(false)
    }).toPass({ timeout: RECHECK_WAIT, intervals: [2_000] })

    // The same session, refreshed rather than replaced: no new sign-in.
    expect((await me(page)).body?.id).toBe(signedIn.body?.id)
    expect(await refreshCalls(admin.request)).toBeGreaterThan(refreshesBefore)
    await expectNoAdminArea(page)
    expect(await providerAdminRows(admin, user.email)).toEqual([])
  })

  test('a refresh the issuer refuses signs the user out at the next re-check', async ({
    browser,
  }) => {
    test.setTimeout(150_000)
    await createProvider(admin, provider)

    const user = newUser('revoked')
    await scriptUser(admin.request, { ...user, claims: { groups: [] } })
    const page = await signInAs(browser, provider, user.email)
    opened.push(page.context())
    const signInHeading = page.getByRole('heading', {
      name: 'Sign in to VibeXP',
      level: 1,
    })
    await expect(page).toHaveURL(HOME)
    await expect(page.getByTestId('user-menu')).toBeVisible()
    await expect(signInHeading).toHaveCount(0)

    await refuseRefresh(admin.request)

    await expect(async () => {
      expect((await me(page)).status).toBe(401)
    }).toPass({ timeout: RECHECK_WAIT, intervals: [2_000] })

    // The browser is signed out too: the app shows it the sign-in page, which
    // a signed-in user never sees at /.
    await page.goto('/')
    await expect(signInHeading).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(page.getByTestId('user-menu')).toHaveCount(0)
    await expect(
      page.getByRole('button', { name: `Continue with ${provider.name}` })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    expect((await me(page)).status).toBe(401)
  })

  test('a sign-in the issuer denies shows the no-access page and signs nobody in', async ({
    browser,
  }) => {
    test.setTimeout(90_000)
    await createProvider(admin, provider)

    const user = newUser('denied')
    await scriptUser(admin.request, { ...user })
    await refuseAuthorization(admin.request, 'access_denied')

    const context = await freshContext(browser)
    opened.push(context)
    const page = await context.newPage()
    await signInWithProvider(page, provider.name)

    await expect(page).toHaveURL(
      /\/auth\/callback\?error=provider_access_denied/
    )
    await expect(page.getByText('No access', { exact: true })).toBeVisible({
      timeout: UI_TIMEOUT,
    })
    await expect(
      page.getByText(
        'Your account does not have access. Contact your administrator.'
      )
    ).toBeVisible()
    expect((await me(page)).status).toBe(401)

    // No account was made for the denied user.
    const users = await admin.request.get(
      `/api/v1/admin/users?search=${encodeURIComponent(user.email)}`
    )
    expect(users.status()).toBe(200)
    const found = (await users.json()) as { users: { email: string }[] }
    expect(found.users.map(u => u.email)).not.toContain(user.email)
  })

  test('a root admin keeps the admin area with no roles claim at all', async ({
    browser,
  }) => {
    test.setTimeout(150_000)
    await createProvider(admin, provider)

    // The root admin's address, from INSTANCE_ADMIN_EMAILS; no groups claim.
    await scriptUser(admin.request, {
      sub: newUser('root').sub,
      email: ADMIN_EMAIL,
    })
    const page = await signInAs(browser, provider, ADMIN_EMAIL)
    opened.push(page.context())

    const signedIn = await me(page)
    expect(signedIn.body?.is_instance_admin).toBe(true)
    expect(signedIn.body?.is_root_instance_admin).toBe(true)
    await expectAdminArea(page)
    // Not through the mapping: there is no provider-sourced row for them.
    expect(await providerAdminRows(admin, ADMIN_EMAIL)).toEqual([])

    // A re-check with still no claim leaves them an admin.
    const refreshesBefore = await refreshCalls(admin.request)
    await expect
      .poll(
        async () => {
          expect((await me(page)).status).toBe(200)
          return refreshCalls(admin.request)
        },
        { timeout: RECHECK_WAIT, intervals: [2_000] }
      )
      .toBeGreaterThan(refreshesBefore)
    expect((await me(page)).body?.is_instance_admin).toBe(true)
    await expectAdminArea(page)
  })
})
