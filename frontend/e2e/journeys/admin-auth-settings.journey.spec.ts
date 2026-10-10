import {
  type Browser,
  type BrowserContext,
  expect,
  type Page,
  test,
} from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'
import { e2eAppAvailable, rearmSetup } from '../helpers/e2eApp'

/**
 * Admin → Settings → Authentication and the first-run setup page, end to end
 * (issue #1239, epic #1230).
 *
 * 1. As the root admin (`ADMIN_EMAIL`, via `INSTANCE_ADMIN_EMAILS`): add a
 *    provider from the UI, see it on the PUBLIC provider list and on the
 *    sign-in page with no restart, test it, edit it, add a second one whose
 *    issuer cannot be reached (unhealthy, so not offered), reorder, delete,
 *    then disable the last enabled one through the lockout confirmation.
 * 2. Tighten the access allowlist and watch a second signed-in user get signed
 *    out, after the confirmation that says who is affected.
 * 3. Re-arm setup the way an operator would (`vibexp admin auth setup rearm`
 *    in the app container), open `/setup?token=…` with no session at all,
 *    configure a provider there, and reach the "sign in as a root admin"
 *    prompt.
 *
 * ## What this cannot do
 *
 * The stack has no identity provider, so nobody can actually sign in THROUGH
 * a provider. The journey therefore stops at the prompt; the provider login
 * that ends setup is covered by the backend's own tests (#1236). For the same
 * reason every session here is a dev login, which carries no provider — the
 * `own_provider` lockout reason cannot be produced and is covered in Vitest.
 *
 * ## Sharing the stack
 *
 * The authentication settings are instance-wide and other specs run in
 * parallel against the same instance, so:
 *
 * - providers are created under a slug prefix only this spec uses, and every
 *   one of them is deleted before and after each test. An extra button on the
 *   sign-in page is all another spec could notice;
 * - the allowlist admits every email domain the suite signs in with
 *   (`SUITE_DOMAINS`) and shuts out only a domain made up for this attempt, so
 *   no other spec's user is signed out. What was stored before is restored.
 */

const UI_TIMEOUT = 20_000
const PROVIDERS_PATH = '/api/v1/admin/settings/auth/providers'
const ALLOWLIST_PATH = '/api/v1/admin/settings/auth/allowlist'
const SLUG_PREFIX = 'e2e-auth-'
/** Every email domain the e2e suite dev-logs in with. */
const SUITE_DOMAINS = ['example.com', 'vibexp.test']

interface ProviderList {
  version: number
  providers: { id: string; slug: string; enabled: boolean }[]
}

interface Allowlist {
  domains: string[]
  emails: string[]
  version: number | null
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

async function readAllowlist(page: Page): Promise<Allowlist> {
  const response = await page.request.get(ALLOWLIST_PATH)
  expect(response.status()).toBe(200)
  return (await response.json()) as Allowlist
}

/** Put the allowlist back to what `original` read. */
async function restoreAllowlist(page: Page, original: Allowlist) {
  const response =
    original.version === null
      ? await page.request.delete(ALLOWLIST_PATH)
      : await page.request.put(ALLOWLIST_PATH, {
          data: { domains: original.domains, emails: original.emails },
        })
  expect(response.ok(), `allowlist restore: ${String(response.status())}`).toBe(
    true
  )
}

/** The slugs the PUBLIC provider list offers for sign-in. */
async function publicProviderSlugs(page: Page): Promise<string[]> {
  const response = await page.request.get('/api/v1/auth/providers')
  expect(response.status()).toBe(200)
  const body = (await response.json()) as { providers: { name: string }[] }
  return body.providers.map(p => p.name)
}

const row = (page: Page, slug: string) =>
  page.locator(
    `[data-testid="auth-provider-row"][data-provider-slug="${slug}"]`
  )

/** Fills the add-provider dialog and saves it. */
async function addProvider(
  page: Page,
  provider: {
    type: 'GitHub' | 'OpenID Connect'
    slug: string
    name: string
    issuer?: string
  }
): Promise<void> {
  await page.getByRole('button', { name: 'Add provider' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('radio', { name: provider.type }).check()
  await dialog.getByLabel('Slug').fill(provider.slug)
  await dialog.getByLabel('Display name').fill(provider.name)
  if (provider.issuer) {
    await dialog.getByLabel('Issuer URL').fill(provider.issuer)
  }
  await dialog.getByLabel('Client ID').fill('e2e-client-id')
  await dialog.getByLabel('Client secret').fill('e2e-client-credential')
  await dialog.getByRole('button', { name: 'Add provider' }).click()
  await expect(dialog).toBeHidden({ timeout: UI_TIMEOUT })
  await expect(row(page, provider.slug)).toBeVisible({ timeout: UI_TIMEOUT })
}

async function openAuthSettings(page: Page): Promise<void> {
  await page.goto('/admin')
  const nav = page.getByRole('navigation', { name: 'Admin sections' })
  await nav.getByRole('link', { name: 'Authentication' }).click()
  await expect(page).toHaveURL(/\/admin\/settings\/auth$/)
  await expect(
    page.getByRole('heading', { name: 'Authentication', level: 1 })
  ).toBeVisible()
  await expect(page.getByTestId('auth-providers-section')).toBeVisible({
    timeout: UI_TIMEOUT,
  })
}

async function freshContext(browser: Browser): Promise<BrowserContext> {
  return browser.newContext({ viewport: { width: 1920, height: 1080 } })
}

test.describe.serial('Admin → Settings → Authentication', () => {
  let originalAllowlist: Allowlist

  test.beforeEach(async ({ page }) => {
    await devLogin(page, ADMIN_EMAIL, ADMIN_NAME)
    await purgeProviders(page)
    originalAllowlist = await readAllowlist(page)
  })

  test.afterEach(async ({ page }) => {
    await purgeProviders(page)
    await restoreAllowlist(page, originalAllowlist)
  })

  test('manage providers from the UI, with no restart', async ({
    page,
    browser,
  }) => {
    test.setTimeout(180_000)

    const tag = Date.now().toString(36)
    const github = {
      slug: `${SLUG_PREFIX}gh-${tag}`,
      name: `E2E GitHub ${tag}`,
    }
    const oidc = { slug: `${SLUG_PREFIX}oidc-${tag}`, name: `E2E OIDC ${tag}` }
    const renamed = `E2E Renamed ${tag}`

    await openAuthSettings(page)

    // --- Add: stored, healthy and offered for sign-in at once ---------------
    await addProvider(page, { type: 'GitHub', ...github })
    const githubRow = row(page, github.slug)
    await expect(githubRow).toContainText(github.name)
    await expect(githubRow.getByTestId('auth-provider-health')).toHaveText(
      'Healthy',
      { timeout: UI_TIMEOUT }
    )
    await expect(
      githubRow.getByRole('switch', { name: `${github.name} enabled` })
    ).toBeChecked()
    await expect
      .poll(() => publicProviderSlugs(page), { timeout: UI_TIMEOUT })
      .toContain(github.slug)

    // The sign-in page of someone with no session offers it.
    const visitor = await freshContext(browser)
    const signIn = await visitor.newPage()
    await signIn.goto('/login')
    await expect(
      signIn.getByRole('button', { name: `Continue with ${github.name}` })
    ).toBeVisible({ timeout: UI_TIMEOUT })

    // --- Edit: the redirect URI is read-only, the secret is kept -------------
    await githubRow.getByRole('button', { name: `Edit ${github.name}` }).click()
    const dialog = page.getByRole('dialog')
    const redirectUri = dialog.getByRole('textbox', { name: 'Redirect URI' })
    await expect(redirectUri).toHaveValue(/\/api\/v1\/auth\/callback$/)
    await expect(redirectUri).toHaveAttribute('readonly', '')
    await expect(
      dialog.getByRole('button', { name: 'Copy redirect URI' })
    ).toBeVisible()
    await expect(dialog.getByLabel('Client secret')).toHaveValue('')
    await expect(dialog.getByText('Leave this blank to keep it.')).toBeVisible()
    await dialog.getByLabel('Display name').fill(renamed)
    await dialog.getByRole('button', { name: 'Save changes' }).click()
    await expect(dialog).toBeHidden({ timeout: UI_TIMEOUT })
    await expect(githubRow).toContainText(renamed)
    // The stored secret survived the edit: the provider is still healthy.
    await expect(githubRow.getByTestId('auth-provider-health')).toHaveText(
      'Healthy'
    )
    await signIn.reload()
    await expect(
      signIn.getByRole('button', { name: `Continue with ${renamed}` })
    ).toBeVisible({ timeout: UI_TIMEOUT })

    // --- A provider whose issuer cannot be reached is stored but not offered -
    await addProvider(page, {
      type: 'OpenID Connect',
      ...oidc,
      // Nothing listens there, so discovery fails fast.
      issuer: 'http://127.0.0.1:1',
    })
    const oidcRow = row(page, oidc.slug)
    await expect(oidcRow.getByTestId('auth-provider-health')).toHaveText(
      'Unhealthy',
      { timeout: UI_TIMEOUT }
    )
    await expect(oidcRow.getByTestId('auth-provider-error')).toContainText(
      'Not offered for sign-in'
    )
    expect(await publicProviderSlugs(page)).not.toContain(oidc.slug)

    // --- Test a stored provider: the server says why it cannot be built -----
    // (The OIDC one, whose check stays on the stack: testing the GitHub one
    // would send its made-up credentials to github.com.)
    await oidcRow.getByRole('button', { name: `Test ${oidc.name}` }).click()
    await expect(oidcRow.getByTestId('auth-provider-test')).toContainText(
      'oidc: discover issuer "http://127.0.0.1:1"',
      { timeout: UI_TIMEOUT }
    )

    // --- Reorder: it survives a reload ----------------------------------------
    const mine = page.locator(
      `[data-testid="auth-provider-row"][data-provider-slug^="${SLUG_PREFIX}"]`
    )
    await expect(mine).toHaveCount(2)
    await expect(mine.first()).toHaveAttribute(
      'data-provider-slug',
      github.slug
    )
    await oidcRow.getByRole('button', { name: `Move ${oidc.name} up` }).click()
    await expect(mine.first()).toHaveAttribute(
      'data-provider-slug',
      oidc.slug,
      { timeout: UI_TIMEOUT }
    )
    await page.reload()
    await expect(mine.first()).toHaveAttribute(
      'data-provider-slug',
      oidc.slug,
      { timeout: UI_TIMEOUT }
    )

    // --- Delete, after a confirmation -----------------------------------------
    await oidcRow.getByRole('button', { name: `Delete ${oidc.name}` }).click()
    const confirmDelete = page.getByRole('alertdialog')
    await expect(confirmDelete).toContainText(`Delete ${oidc.name}?`)
    await confirmDelete.getByRole('button', { name: 'Delete provider' }).click()
    await expect(oidcRow).toHaveCount(0, { timeout: UI_TIMEOUT })

    // --- Disable the last enabled provider: the lockout confirmation ---------
    const enabledSwitch = githubRow.getByRole('switch', {
      name: `${renamed} enabled`,
    })
    const others = (await listProviders(page)).providers.filter(
      p => p.enabled && p.slug !== github.slug
    )
    expect(others, 'no other spec enables a provider').toEqual([])
    await enabledSwitch.click()
    const lockout = page.getByRole('alertdialog')
    await expect(lockout).toContainText('This leaves no way to sign in', {
      timeout: UI_TIMEOUT,
    })
    // Refused so far: it is still enabled and still offered.
    expect(await publicProviderSlugs(page)).toContain(github.slug)
    await lockout.getByRole('button', { name: 'Apply anyway' }).click()
    await expect(lockout).toBeHidden({ timeout: UI_TIMEOUT })
    await expect(enabledSwitch).not.toBeChecked({ timeout: UI_TIMEOUT })
    await expect(githubRow.getByTestId('auth-provider-health')).toHaveText(
      'Disabled'
    )
    await expect
      .poll(() => publicProviderSlugs(page), { timeout: UI_TIMEOUT })
      .not.toContain(github.slug)
    await signIn.reload()
    await expect(
      signIn.getByRole('button', { name: `Continue with ${renamed}` })
    ).toHaveCount(0)
    await visitor.close()

    // --- The history recorded it, by this admin -------------------------------
    const history = page
      .getByTestId('auth-audit-entry-auth_providers')
      .filter({ hasText: github.slug })
    await expect(history.first()).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(history.first()).toContainText(ADMIN_NAME)
    await expect(
      page
        .getByTestId('auth-audit-entry-auth_providers')
        .filter({ hasText: 'Deleted' })
        .filter({ hasText: oidc.slug })
    ).toHaveCount(1)
  })

  test('tightening the allowlist signs a second user out', async ({
    page,
    browser,
  }) => {
    test.setTimeout(180_000)

    const tag = Date.now().toString(36)
    // A domain no other spec uses, so only this user is shut out.
    const victimEmail = `victim-${tag}@locked-${tag}.test`

    const victimContext = await freshContext(browser)
    const victim = await victimContext.newPage()
    await devLogin(victim, victimEmail, 'Locked Out')
    expect((await victim.request.get('/api/v1/auth/me')).status()).toBe(200)

    await openAuthSettings(page)
    const section = page.getByTestId('auth-allowlist-section')
    await expect(section).toBeVisible({ timeout: UI_TIMEOUT })

    // --- Tighten: every suite domain stays allowed, the victim's does not ----
    const domains = section.getByLabel('Allowed domains')
    for (const domain of SUITE_DOMAINS) {
      await domains.fill(domain)
      await domains.press('Enter')
    }
    await section.getByRole('button', { name: 'Save allowlist' }).click()

    const confirm = page.getByRole('alertdialog')
    await expect(confirm).toContainText(
      /\d+ signed-in users? will be signed out/,
      { timeout: UI_TIMEOUT }
    )
    await expect(confirm.getByTestId('allowlist-impact')).toContainText(
      victimEmail
    )
    // Nothing is saved until the admin confirms.
    expect((await victim.request.get('/api/v1/auth/me')).status()).toBe(200)
    await confirm
      .getByRole('button', { name: 'Save and sign them out' })
      .click()
    await expect(confirm).toBeHidden({ timeout: UI_TIMEOUT })
    await expect(section.getByTestId('allowlist-status')).toHaveText(
      'Restricted',
      { timeout: UI_TIMEOUT }
    )

    // --- The victim is signed out; the admin (a root admin) is not ----------
    await expect
      .poll(
        async () => (await victim.request.get('/api/v1/auth/me')).status(),
        {
          timeout: UI_TIMEOUT,
        }
      )
      .not.toBe(200)
    await victim.goto('/')
    await expect(
      victim.getByRole('heading', { name: /^Sign in to / })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    expect((await page.request.get('/api/v1/auth/me')).status()).toBe(200)
    await victimContext.close()

    // --- Reset to open access --------------------------------------------------
    await section.getByRole('button', { name: 'Reset to open access' }).click()
    const reset = page.getByRole('alertdialog')
    await reset.getByRole('button', { name: 'Reset to open access' }).click()
    await expect(section.getByTestId('allowlist-status')).toHaveText(
      'Open access',
      { timeout: UI_TIMEOUT }
    )
    expect((await readAllowlist(page)).version).toBeNull()
  })

  test('first-run setup: token → provider → root-admin prompt', async ({
    page,
    browser,
  }) => {
    test.skip(
      !e2eAppAvailable(),
      'needs the docker e2e stack: the setup URL only comes from `vibexp admin auth setup rearm` in the app container'
    )
    test.setTimeout(180_000)

    const tag = Date.now().toString(36)
    const github = {
      slug: `${SLUG_PREFIX}setup-${tag}`,
      name: `E2E Setup ${tag}`,
    }

    // No provider is enabled (beforeEach purged this spec's, and no other spec
    // adds one), which is what makes this a first-run instance.
    expect(
      (await listProviders(page)).providers.filter(p => p.enabled)
    ).toEqual([])
    const token = rearmSetup()

    // --- The sign-in page says where the setup URL is -------------------------
    const operatorContext = await freshContext(browser)
    const operator = await operatorContext.newPage()
    await operator.goto('/login')
    await expect(operator.getByTestId('setup-required-hint')).toContainText(
      'SETUP URL',
      { timeout: UI_TIMEOUT }
    )

    // --- A wrong token is a terminal state, not a broken page ----------------
    await operator.goto('/setup?token=not-the-token')
    await expect(operator.getByTestId('setup-invalid')).toContainText(
      'This setup link is no longer valid',
      { timeout: UI_TIMEOUT }
    )

    // --- The real token opens the setup shell, with no user at all ----------
    await operator.goto(`/setup?token=${token}`)
    await expect(
      operator.getByRole('heading', {
        name: 'Set up sign-in for this instance',
        level: 1,
      })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    // The token is gone from the address bar.
    await expect(operator).toHaveURL(/\/setup$/)
    await expect(operator.getByTestId('auth-providers-section')).toBeVisible()
    await expect(operator.getByTestId('auth-allowlist-section')).toBeVisible()
    // Only providers and the allowlist: no admins, no history...
    await expect(operator.getByTestId('auth-admins-section')).toHaveCount(0)
    await expect(operator.getByTestId('auth-audit-section')).toHaveCount(0)
    // ...and the setup session could not read them anyway, nor is it a user.
    expect(
      (
        await operator.request.get('/api/v1/admin/settings/auth/admins')
      ).status()
    ).toBe(404)
    expect((await operator.request.get('/api/v1/auth/me')).status()).toBe(401)
    await expect(operator.getByTestId('setup-next-step')).toBeVisible()

    // --- Configure a provider on the setup session ----------------------------
    await addProvider(operator, { type: 'GitHub', ...github })
    await expect(
      row(operator, github.slug).getByTestId('auth-provider-health')
    ).toHaveText('Healthy', { timeout: UI_TIMEOUT })
    expect(await publicProviderSlugs(operator)).toContain(github.slug)

    // --- The last step: sign in as a root admin -------------------------------
    await expect(
      operator.getByRole('button', {
        name: `Sign in with ${github.name} as a root admin to finish setup`,
      })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(operator.getByTestId('setup-next-step')).toHaveCount(0)

    // A reload has no token left, and resumes on the setup cookie.
    await operator.reload()
    await expect(row(operator, github.slug)).toBeVisible({
      timeout: UI_TIMEOUT,
    })
    await operatorContext.close()
  })
})
