import { expect, type Page, test } from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'

/**
 * Admin → Settings → Search, end to end (issue #1202, epic #1196).
 *
 * As the instance admin: open the page from the sidebar's Settings group,
 * change the search half-life default, see it saved ("Customized", the audit
 * entry), see a team with no search settings of its own inherit it on its
 * settings page, then reset to the built-in defaults and see both pages
 * follow.
 *
 * ## Why the half-life, and only the half-life
 *
 * The instance defaults are shared by every spec running in parallel. The
 * half-life only matters while recency ranking is on, which the built-in
 * defaults leave off, so changing it cannot reorder another spec's search
 * results. The recency switch is never touched.
 *
 * The instance settings are a singleton, so the spec records what it found
 * and restores exactly that afterwards (a stored row or the built-in
 * defaults) — never assumes a starting state. The new half-life is derived
 * per attempt so the history assertion can only match this attempt's entry.
 */

const UI_TIMEOUT = 20_000
const SETTINGS_PATH = '/api/v1/admin/settings/search'

interface SearchSettings {
  source: 'instance' | 'default'
  values: Record<string, unknown> & { rank_half_life_days: number }
  built_in_defaults: { rank_half_life_days: number }
}

async function readSettings(page: Page): Promise<SearchSettings> {
  const response = await page.request.get(SETTINGS_PATH)
  expect(response.status()).toBe(200)
  return (await response.json()) as SearchSettings
}

/** Put the instance search settings back to what `original` read. */
async function restore(page: Page, original: SearchSettings): Promise<void> {
  const response =
    original.source === 'default'
      ? await page.request.delete(SETTINGS_PATH)
      : await page.request.put(SETTINGS_PATH, { data: original.values })
  expect(response.ok(), `restore failed: ${String(response.status())}`).toBe(
    true
  )
}

async function createTeam(page: Page, name: string): Promise<string> {
  const response = await page.request.post('/api/v1/teams', {
    data: { name, description: 'Seeded by the instance settings e2e' },
  })
  expect(response.ok(), `team create: ${String(response.status())}`).toBe(true)
  const body = (await response.json()) as {
    id?: string
    team?: { id?: string }
  }
  const id = body.team?.id ?? body.id
  expect(id, 'team id missing from create response').toBeTruthy()
  return id as string
}

/** The team search settings page's half-life, from its Advanced disclosure. */
async function teamHalfLife(page: Page, teamId: string) {
  await page.goto(`/teams/${teamId}/settings/search`)
  await expect(page.getByText('Using instance defaults')).toBeVisible({
    timeout: UI_TIMEOUT,
  })
  await page.getByRole('button', { name: /Advanced/ }).click()
  return page.getByLabel('Half-life (days)', { exact: true })
}

test.describe.serial('Admin → Settings → Search', () => {
  let original: SearchSettings

  test.beforeEach(async ({ page }) => {
    await devLogin(page, ADMIN_EMAIL, ADMIN_NAME)
    original = await readSettings(page)
  })

  test.afterEach(async ({ page }) => {
    await restore(page, original)
  })

  test('edit the search defaults, see a team inherit them, then reset', async ({
    page,
  }) => {
    test.setTimeout(120_000)

    const stamp = Date.now()
    // A whole number of days no earlier value can share: 1000–5999, well
    // inside the 36500 maximum, and never equal to the value found.
    let halfLife = 1000 + (stamp % 5000)
    if (halfLife === original.values.rank_half_life_days) halfLife += 1
    const builtInHalfLife = original.built_in_defaults.rank_half_life_days
    // Created by the admin, with no search settings of its own.
    const teamId = await createTeam(page, `Inst ${stamp.toString(36)}`)

    // --- Reach the page from the sidebar's Settings group -----------------
    await page.goto('/admin')
    const nav = page.getByRole('navigation', { name: 'Admin sections' })
    await nav.getByRole('link', { name: 'Search' }).click()
    await expect(page).toHaveURL(/\/admin\/settings\/search$/)
    await expect(
      page.getByRole('heading', { name: 'Search', level: 1 })
    ).toBeVisible()

    const halfLifeInput = page.getByLabel('Half-life (days)', { exact: true })
    await expect(halfLifeInput).toHaveValue(
      String(original.values.rank_half_life_days),
      { timeout: UI_TIMEOUT }
    )

    // --- Client-side validation blocks an out-of-range value --------------
    await halfLifeInput.fill('0')
    await expect(page.getByText(/Enter a number above 0/)).toBeVisible()
    await expect(
      page.getByRole('button', { name: 'Save changes' })
    ).toBeDisabled()

    // --- Save -----------------------------------------------------------------
    await halfLifeInput.fill(String(halfLife))
    await page.getByRole('button', { name: 'Save changes' }).click()
    const source = page.getByTestId('instance-settings-source')
    await expect(source).toHaveText('Customized', { timeout: UI_TIMEOUT })
    await expect(page.getByTestId('instance-settings-status')).toContainText(
      `Last changed by ${ADMIN_NAME}`
    )

    // The save is in the change history, with this attempt's value.
    const saved = page
      .getByTestId('instance-search-audit-entry')
      .filter({ hasText: 'Saved' })
      .filter({
        hasText: new RegExp(`Half-life \\(days\\).*→ ${String(halfLife)}`),
      })
    await expect(saved).toHaveCount(1, { timeout: UI_TIMEOUT })
    await expect(saved).toContainText(ADMIN_NAME)

    // A reload reads the stored value back.
    await page.reload()
    await expect(halfLifeInput).toHaveValue(String(halfLife), {
      timeout: UI_TIMEOUT,
    })

    // --- A team with no settings of its own inherits it ------------------
    await expect(await teamHalfLife(page, teamId)).toHaveValue(String(halfLife))

    // --- Reset to the built-in defaults -----------------------------------
    await page.goto('/admin/settings/search')
    await page.getByRole('button', { name: 'Reset to defaults' }).click()
    const dialog = page.getByRole('alertdialog')
    await expect(dialog).toContainText(
      `half-life ${String(builtInHalfLife)} days`
    )
    await dialog.getByRole('button', { name: 'Reset to defaults' }).click()

    await expect(source).toHaveText('Using built-in defaults', {
      timeout: UI_TIMEOUT,
    })
    await expect(halfLifeInput).toHaveValue(String(builtInHalfLife))
    await expect(
      page.getByRole('button', { name: 'Reset to defaults' })
    ).toHaveCount(0)
    await expect(
      page
        .getByTestId('instance-search-audit-entry')
        .filter({ hasText: 'Reset to defaults' })
        .filter({
          hasText: new RegExp(`Half-life \\(days\\).*${String(halfLife)}`),
        })
    ).toHaveCount(1, { timeout: UI_TIMEOUT })

    // ...and the team follows.
    await expect(await teamHalfLife(page, teamId)).toHaveValue(
      String(builtInHalfLife)
    )
  })
})
