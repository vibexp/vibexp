import { expect, test } from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'
import { e2eAppAvailable, rearmSetup } from '../helpers/e2eApp'
import {
  freshPage,
  passwordFor,
  passwordSignIn,
  whoAmI,
  type InstanceAuthSnapshot,
  restoreInstanceAuth,
  snapshotInstanceAuth,
} from '../helpers/passwordAuth'

/**
 * Journey 1 of #1444: a password-only bootstrap (#1438, #1442).
 *
 * The operator re-arms setup, opens the setup URL in a browser with no
 * session, chooses "Use email and password" for a root admin address that has
 * no account yet, and lands signed in; setup is then finished, and the
 * password signs in from another browser.
 *
 * It uses the stack's second root admin (`root-password-e2e@…`), so the
 * address every other spec dev-logs in as never gets a password.
 */
const ROOT_EMAIL = 'root-password-e2e@vibexp.test'

test.describe.serial('Password journeys: bootstrap', () => {
  let snapshot: InstanceAuthSnapshot | undefined

  test.beforeEach(async ({ page }) => {
    snapshot = undefined
    test.skip(!e2eAppAvailable(), 'needs the docker e2e stack (make e2e)')
    await devLogin(page, ADMIN_EMAIL, ADMIN_NAME)
    snapshot = await snapshotInstanceAuth(page)
  })

  // In afterEach, not a finally in the test: it also runs after a timeout.
  test.afterEach(async ({ page }) => {
    if (snapshot) await restoreInstanceAuth(page, snapshot)
  })

  test('set up the instance with a root admin password only', async ({
    browser,
  }) => {
    test.setTimeout(120_000)
    const password = passwordFor('root')

    const token = rearmSetup()
    const { context, page: operator } = await freshPage(browser)
    try {
      await operator.goto(`/setup?token=${token}`)
      const card = operator.getByTestId('setup-password-section')
      await expect(card).toBeVisible({ timeout: 20_000 })
      await card.getByLabel('Root admin email').fill(ROOT_EMAIL)
      await card.getByLabel('Password', { exact: true }).fill(password)
      await card.getByLabel('Repeat the password').fill(password)
      await card
        .getByRole('button', { name: 'Set up admin and sign in' })
        .click()

      await expect(operator).toHaveURL(/\/$/, { timeout: 20_000 })
      expect(await whoAmI(operator)).toBe(ROOT_EMAIL)
      const status = await operator.request.get('/api/v1/setup/status')
      expect(await status.json()).toMatchObject({ setup_required: false })
    } finally {
      await context.close()
    }

    // The password signs in through the sign-in page, from another browser.
    const second = await freshPage(browser)
    try {
      await passwordSignIn(second.page, ROOT_EMAIL, password)
      expect(await whoAmI(second.page)).toBe(ROOT_EMAIL)
    } finally {
      await second.context.close()
    }
  })
})
