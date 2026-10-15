import { expect, type Page, test } from '@playwright/test'

import { devLogin, ONBOARDING_PATH } from '../fixtures/auth'

/**
 * First-run onboarding, end to end.
 *
 * A brand-new user signs in and is taken from the dashboard to the onboarding
 * at `/getting-started`: welcome, building blocks, Claude Code plugin, team
 * rules, VibeXP CLI, done. Steps can be skipped one at a time or all at once,
 * and the flow can be reopened at any time from the app.
 *
 * ## Why this is an e2e
 *
 * The step components and the redirect decision are unit-tested. Only a
 * browser against the real stack shows the pieces working together:
 *
 * - **The redirect fires for a real new user and only once.** It hangs on the
 *   `onboarding_completed` flag the backend sends with the user. Finishing
 *   or skipping the flow stores that flag server-side, so after a full reload
 *   (a fresh `/auth/me`) the dashboard has to stay put.
 * - **Deep links are not hijacked.** A new user who opens a shared link lands
 *   on it; the onboarding waits for the dashboard.
 * - **The copy buttons write the real clipboard**, with the commands a
 *   person pastes into Claude Code and their terminal.
 */

/** Combined-stack UI budget, matching the other journeys. */
const UI_TIMEOUT = 20_000

function uniqueEmail(who: string): string {
  return `e2e-onboarding-${who}-${Date.now()}@vibexp.test`
}

async function signInAsNewUser(page: Page, who: string): Promise<void> {
  await devLogin(page, uniqueEmail(who), `Onboarding ${who}`, {
    keepOnboarding: true,
  })
}

async function expectOnStep(page: Page, step: number): Promise<void> {
  // The flow keeps the step on screen in `?step=` (0-based); the header's
  // "Step N of 6" is the same fact for a screen reader.
  await expect(page).toHaveURL(new RegExp(`[?&]step=${String(step - 1)}$`), {
    timeout: UI_TIMEOUT,
  })
  await expect(page.getByRole('banner')).toContainText(`Step ${String(step)}`)
}

async function readOnboardingCompleted(page: Page): Promise<boolean> {
  const response = await page.request.get('/api/v1/auth/me')
  expect(response.status(), 'read the signed-in user').toBe(200)
  const user = (await response.json()) as { onboarding_completed?: boolean }
  return user.onboarding_completed === true
}

test.describe('First-run onboarding', () => {
  test('a new user walks the flow, skips a step, finishes it later and replays it', async ({
    page,
    context,
  }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await signInAsNewUser(page, 'walk')

    // Signing in lands on the dashboard, which hands a new user over.
    await page.waitForURL(url => url.pathname === ONBOARDING_PATH, {
      timeout: UI_TIMEOUT,
    })
    await expectOnStep(page, 1)
    await expect(
      page.getByRole('heading', {
        name: "Your team's shared brain for AI agents",
      })
    ).toBeVisible()
    await expect(
      page.locator('iframe[src*="youtube-nocookie.com/embed/-9NFGBjoc5g"]')
    ).toBeVisible()

    // Step 2: the building blocks are tabs.
    await page.getByRole('button', { name: 'Get started' }).click()
    await expectOnStep(page, 2)
    const tabs = page.getByRole('tablist', { name: 'Building blocks' })
    await expect(tabs.getByRole('tab')).toHaveText([
      'Memory',
      'Blueprints',
      'Artifacts',
      'AI Feeds',
    ])
    await tabs.getByRole('tab', { name: 'Blueprints' }).click()
    await expect(tabs.getByRole('tab', { name: 'Blueprints' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    await expect(
      page.getByRole('heading', { name: 'Blueprints', level: 3 })
    ).toBeVisible()

    // Step 3: the plugin commands land on the clipboard as shown.
    await page.getByRole('button', { name: 'Continue' }).click()
    await expectOnStep(page, 3)
    await page.getByRole('button', { name: 'Copy marketplace command' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      '/plugin marketplace add vibexp/plugin-marketplace'
    )
    await page.getByRole('button', { name: 'From your terminal' }).click()
    await expect(
      page.getByRole('button', { name: 'From your terminal' })
    ).toHaveAttribute('aria-pressed', 'true')
    await page.getByRole('button', { name: 'Copy install command' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      'claude plugin install vibexp@vibexp'
    )

    // Step 4: skip the team rules for now.
    await page.getByRole('button', { name: "I've installed it" }).click()
    await expectOnStep(page, 4)
    await page.getByRole('button', { name: 'Skip this step' }).click()

    // Step 5: the CLI command carries this instance's address.
    await expectOnStep(page, 5)
    const origin = new URL(page.url()).origin
    await page.getByRole('button', { name: 'Copy install command' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      `claude "/vibexp:install ${origin}"`
    )

    // Step 6: the summary offers the skipped step back.
    await page.getByRole('button', { name: "I've run it" }).click()
    await expectOnStep(page, 6)
    await expect(
      page.getByRole('heading', {
        name: 'Your agents now learn with your team',
      })
    ).toBeVisible()
    await page.getByRole('button', { name: 'Finish this step' }).click()
    await expectOnStep(page, 4)
    await expect(
      page.getByRole('heading', { name: 'Make it a habit in every session' })
    ).toBeVisible()
    await page.getByRole('button', { name: 'Copy rules' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toContain(
      '/vibexp:wrap'
    )

    // Leaving through the last step stores completion on the server.
    await page.goto(`${ONBOARDING_PATH}?step=5`)
    await expectOnStep(page, 6)
    await page.getByRole('button', { name: 'Go to VibeXP' }).click()
    await page.waitForURL('/', { timeout: UI_TIMEOUT })
    expect(await readOnboardingCompleted(page)).toBe(true)
    await page.reload()
    await expect(page).toHaveURL('/')

    // It can be reopened at any time from the app header, where it resumes
    // at the first step not done: the team rules, still only skipped.
    const entry = page.getByRole('link', { name: /^Getting started,/ })
    await expect(entry).toHaveAccessibleName(
      'Getting started, 4 of 5 steps done'
    )
    await entry.click()
    await expectOnStep(page, 4)
  })

  test('"Skip setup" ends the onboarding for good', async ({ page }) => {
    await signInAsNewUser(page, 'skip')
    await page.waitForURL(url => url.pathname === ONBOARDING_PATH, {
      timeout: UI_TIMEOUT,
    })

    await page.getByRole('button', { name: 'Skip setup' }).click()
    await page.waitForURL('/', { timeout: UI_TIMEOUT })
    expect(await readOnboardingCompleted(page)).toBe(true)

    // A fresh session in the same browser stays on the dashboard.
    await page.evaluate(() => sessionStorage.clear())
    await page.reload()
    await expect(page).toHaveURL('/')
  })

  test('a deep link is not hijacked by the onboarding', async ({ page }) => {
    await signInAsNewUser(page, 'deeplink')

    await page.goto('/memories')
    await expect(page).toHaveURL(/\/memories$/)
    await expect(
      page.getByRole('heading', { name: 'Memories', level: 1 })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    expect(await readOnboardingCompleted(page)).toBe(false)
  })
})
