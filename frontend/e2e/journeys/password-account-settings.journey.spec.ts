import { expect, test } from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'
import { e2eAppAvailable } from '../helpers/e2eApp'
import {
  freshPage,
  passwordFor,
  passwordWorks,
  RUN_ID,
  runEmail,
  setPolicy,
  withInstanceAuthRestored,
} from '../helpers/passwordAuth'

/**
 * Journey 6 of #1444: Settings → Account (#1441).
 *
 * A person who signed in with dev login (standing in for a provider: no
 * password, and an identity the page does not list) sets a first password,
 * signs in with it, changes it, and cannot remove the password, their last way
 * in.
 */
const UI_TIMEOUT = 20_000

test.describe.serial('Password journeys: account settings', () => {
  test('set, use and change a password; the last method stays', async ({
    page,
    browser,
  }) => {
    test.skip(!e2eAppAvailable(), 'needs the docker e2e stack (make e2e)')
    test.setTimeout(120_000)
    await devLogin(page, ADMIN_EMAIL, ADMIN_NAME)

    await withInstanceAuthRestored(page, async () => {
      await setPolicy(page, {
        password_login_enabled: true,
        registration_mode: 'open',
      })
      const email = runEmail('account', 'person')
      const first = passwordFor('account-first')
      const second = passwordFor('account-second')

      const { context, page: person } = await freshPage(browser)
      try {
        await devLogin(person, email, `Account ${RUN_ID}`)
        await person.goto('/settings/account')

        // --- A first password (the sign-in is recent) -------------------------
        const set = person.getByRole('form', { name: 'Set a password' })
        await expect(set).toBeVisible({ timeout: UI_TIMEOUT })
        await set.getByLabel('New password', { exact: true }).fill(first)
        await set.getByLabel('Repeat the new password').fill(first)
        await set.getByRole('button', { name: 'Set password' }).click()
        await expect(
          person.getByText('Password set. Your other devices were signed out.')
        ).toBeVisible({ timeout: UI_TIMEOUT })
        expect(await passwordWorks(browser, email, first)).toBe(true)

        // --- Changing it needs the current one --------------------------------
        const change = person.getByRole('form', { name: 'Change password' })
        await expect(change).toBeVisible()
        await change.getByLabel('Current password').fill(first)
        await change.getByLabel('New password', { exact: true }).fill(second)
        await change.getByLabel('Repeat the new password').fill(second)
        await change.getByRole('button', { name: 'Change password' }).click()
        await expect(
          person.getByText(
            'Password changed. Your other devices were signed out.'
          )
        ).toBeVisible({ timeout: UI_TIMEOUT })
        expect(await passwordWorks(browser, email, first)).toBe(false)
        expect(await passwordWorks(browser, email, second)).toBe(true)

        // --- The only way in cannot be removed --------------------------------
        await expect(
          person.getByRole('button', { name: 'Remove password' })
        ).toBeDisabled()
        await expect(person.getByTestId('account-password-row')).toContainText(
          'the password cannot be removed'
        )
      } finally {
        await context.close()
      }
    })
  })
})
