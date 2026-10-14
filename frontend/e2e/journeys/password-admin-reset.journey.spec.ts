import { expect, test } from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'
import { e2eAppAvailable } from '../helpers/e2eApp'
import {
  createPasswordAccount,
  freshPage,
  passwordFor,
  passwordWorks,
  removeEmail,
  runEmail,
  setPolicy,
  whoAmI,
  type InstanceAuthSnapshot,
  restoreInstanceAuth,
  snapshotInstanceAuth,
} from '../helpers/passwordAuth'

/**
 * Journey 5 of #1444: an administrator resets a password on an instance that
 * sends no email (#1436, #1439, #1443).
 *
 * With no email provider, "Forgot password?" tells the person to ask their
 * administrator. The administrator opens the user, chooses Reset password with
 * "Sign out and disable the current password now", confirms, and copies the
 * link shown once. The old password stops at once; the person opens the link,
 * sets a new password and is signed in.
 */
const UI_TIMEOUT = 20_000

test.describe
  .serial('Password journeys: administrator reset without email', () => {
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

  test('the "ask your administrator" state, then an admin reset link', async ({
    page,
    browser,
  }) => {
    test.setTimeout(150_000)

    await setPolicy(page, {
      password_login_enabled: true,
      registration_mode: 'open',
    })
    await removeEmail(page)
    const email = runEmail('reset', 'person')
    const oldPassword = passwordFor('reset-old')
    const newPassword = passwordFor('reset-new')
    await createPasswordAccount(page, browser, email, oldPassword)

    // --- The person asks for a reset: there is no email -------------------
    const person = await freshPage(browser)
    await person.page.goto('/forgot-password')
    await expect(
      person.page.getByTestId('forgot-password-admin')
    ).toContainText('Ask your administrator', { timeout: UI_TIMEOUT })

    // --- The administrator resets, disabling the old password now --------
    const found = await page.request.get(
      `/api/v1/admin/users?search=${encodeURIComponent(email)}`
    )
    const userId = ((await found.json()) as { users: { id: string }[] })
      .users[0].id
    await page.goto(`/admin/users/${userId}`)
    await page.getByRole('button', { name: 'Reset password' }).click()
    const dialog = page.getByTestId('reset-password-dialog')
    await dialog
      .getByRole('checkbox', {
        name: 'Sign out and disable the current password now',
      })
      .click()
    await dialog.getByRole('button', { name: 'Show the link here' }).click()
    await expect(page.getByTestId('reset-confirm')).toBeVisible()
    await dialog
      .getByRole('button', { name: 'Disable and show the link' })
      .click()
    const shown = (
      await page.getByTestId('link-reveal-value').innerText({
        timeout: UI_TIMEOUT,
      })
    ).trim()
    await expect(page.getByTestId('reset-invalidated')).toBeVisible()
    expect(await passwordWorks(browser, email, oldPassword)).toBe(false)
    await dialog.getByRole('button', { name: 'Done' }).click()
    await expect(page.getByTestId('user-sign-in-methods')).toContainText(
      'Password (disabled)'
    )

    // --- The person uses the handed-over link ------------------------------
    const url = new URL(shown)
    await person.page.goto(url.pathname + url.hash)
    const reset = person.page.getByRole('form', {
      name: 'Set a new password',
    })
    await expect(reset).toBeVisible({ timeout: UI_TIMEOUT })
    await reset.getByLabel('Password', { exact: true }).fill(newPassword)
    await reset.getByLabel('Repeat the password').fill(newPassword)
    await reset
      .getByRole('button', { name: 'Set password and sign in' })
      .click()
    await expect(person.page).toHaveURL(/\/$/, { timeout: UI_TIMEOUT })
    expect(await whoAmI(person.page)).toBe(email)
    await person.context.close()
    expect(await passwordWorks(browser, email, newPassword)).toBe(true)
  })
})
