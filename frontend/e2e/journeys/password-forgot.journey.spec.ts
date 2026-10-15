import { expect, test } from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin, SIGNED_IN_LANDING } from '../fixtures/auth'
import { e2eAppAvailable } from '../helpers/e2eApp'
import {
  createPasswordAccount,
  emailedAccountLink,
  freshPage,
  passwordFor,
  passwordSignIn,
  passwordWorks,
  runEmail,
  setPolicy,
  useMailpit,
  whoAmI,
  type InstanceAuthSnapshot,
  restoreInstanceAuth,
  snapshotInstanceAuth,
} from '../helpers/passwordAuth'

/**
 * Journey 4 of #1444: forgot password, by email (#1433, #1439).
 *
 * A person with a password and a browser already signed in elsewhere uses
 * "Forgot password?" on the sign-in page, takes the link from the stack's
 * Mailpit, sets a new password and is signed in. The other browser is signed
 * out, the old password no longer works and the new one does.
 */
const UI_TIMEOUT = 20_000

test.describe.serial('Password journeys: forgot password', () => {
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

  test('reset by email signs the person in and ends other sessions', async ({
    page,
    browser,
  }) => {
    test.setTimeout(150_000)

    await setPolicy(page, {
      password_login_enabled: true,
      registration_mode: 'open',
    })
    await useMailpit(page)
    const email = runEmail('forgot', 'person')
    const oldPassword = passwordFor('forgot-old')
    const newPassword = passwordFor('forgot-new')
    await createPasswordAccount(page, browser, email, oldPassword)

    // Another device, signed in before the reset.
    const elsewhere = await freshPage(browser)
    await passwordSignIn(elsewhere.page, email, oldPassword)
    expect(await whoAmI(elsewhere.page)).toBe(email)

    const { context, page: person } = await freshPage(browser)
    try {
      await person.goto('/login')
      await person.getByRole('link', { name: 'Forgot password?' }).click()
      await expect(person).toHaveURL(/\/forgot-password$/)
      const request = person.getByRole('form', {
        name: 'Request a password reset',
      })
      await request.getByLabel('Email').fill(email)
      await request
        .getByRole('button', { name: 'Email me a reset link' })
        .click()
      await expect(person.getByTestId('forgot-password-sent')).toBeVisible({
        timeout: UI_TIMEOUT,
      })

      await person.goto(await emailedAccountLink(email, 'reset'))
      const reset = person.getByRole('form', { name: 'Set a new password' })
      await expect(reset).toBeVisible({ timeout: UI_TIMEOUT })
      await reset.getByLabel('Password', { exact: true }).fill(newPassword)
      await reset.getByLabel('Repeat the password').fill(newPassword)
      await reset
        .getByRole('button', { name: 'Set password and sign in' })
        .click()
      await expect(person).toHaveURL(SIGNED_IN_LANDING, { timeout: UI_TIMEOUT })
      expect(await whoAmI(person)).toBe(email)
    } finally {
      await context.close()
    }

    expect(await whoAmI(elsewhere.page)).toBeNull()
    await elsewhere.context.close()
    expect(await passwordWorks(browser, email, oldPassword)).toBe(false)
    expect(await passwordWorks(browser, email, newPassword)).toBe(true)
  })
})
