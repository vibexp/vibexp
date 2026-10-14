import { expect, type Page, test } from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'
import { e2eAppAvailable } from '../helpers/e2eApp'
import {
  createTeam,
  emailedAccountLink,
  freshPage,
  passwordFor,
  removeEmail,
  RUN_ID,
  runEmail,
  setPolicy,
  useMailpit,
  whoAmI,
  withInstanceAuthRestored,
} from '../helpers/passwordAuth'

/**
 * Journey 2 of #1444: an administrator creates a user (#1436, #1443).
 *
 * With an email provider the setup link is emailed: the journey reads it from
 * the stack's Mailpit, opens it signed out, creates a password and lands
 * signed in, a member of the team chosen at creation. Without one the link is
 * shown once in the dialog, copied, and works the same way.
 */
const UI_TIMEOUT = 20_000

/** New user → email, name, one team → Create user; returns the outcome panel. */
async function createUser(
  admin: Page,
  email: string,
  team: string
): Promise<void> {
  await admin.goto('/admin/users')
  await admin.getByRole('button', { name: /New user/ }).click()
  const dialog = admin.getByRole('dialog')
  await dialog.getByLabel('Email').fill(email)
  await dialog.getByLabel('Name (optional)').fill(`E2E ${RUN_ID}`)
  await dialog.getByRole('button', { name: 'Add a team' }).click()
  await admin.getByPlaceholder('Search teams…').fill(team)
  await admin.getByRole('option', { name: team }).click()
  await dialog.getByRole('button', { name: 'Create user' }).click()
  await expect(admin.getByTestId('user-created')).toBeVisible({
    timeout: UI_TIMEOUT,
  })
}

/** Opens a setup link signed out, sets the password, checks who and where. */
async function completeSetup(
  page: Page,
  link: string,
  email: string,
  team: string
): Promise<void> {
  await page.goto(link)
  const form = page.getByRole('form', { name: 'Set up your account' })
  await expect(form).toBeVisible({ timeout: UI_TIMEOUT })
  await expect(form.getByLabel('Email')).toHaveValue(email)
  const password = passwordFor(email)
  await form.getByLabel('Password', { exact: true }).fill(password)
  await form.getByLabel('Repeat the password').fill(password)
  await form
    .getByRole('button', { name: 'Create password and sign in' })
    .click()
  await expect(page).toHaveURL(/\/$/, { timeout: UI_TIMEOUT })
  expect(await whoAmI(page)).toBe(email)
  const teams = await (await page.request.get('/api/v1/teams')).text()
  expect(teams).toContain(team)
}

test.describe
  .serial('Password journeys: an administrator creates a user', () => {
  test('emailed setup link, then a link shown once with no email provider', async ({
    page,
    browser,
  }) => {
    test.skip(!e2eAppAvailable(), 'needs the docker e2e stack (make e2e)')
    test.setTimeout(180_000)
    await devLogin(page, ADMIN_EMAIL, ADMIN_NAME)

    await withInstanceAuthRestored(page, async () => {
      await setPolicy(page, {
        password_login_enabled: true,
        registration_mode: 'open',
      })
      const team = `E2E Password Team ${RUN_ID}`
      await createTeam(page, team)

      // --- Emailed ---------------------------------------------------------
      await useMailpit(page)
      const mailed = runEmail('create', 'mailed')
      await createUser(page, mailed, team)
      await expect(page.getByTestId('user-created')).toContainText(
        'Setup link emailed'
      )
      await page.getByRole('button', { name: 'Done' }).click()
      const link = await emailedAccountLink(mailed, 'setup')
      const owner = await freshPage(browser)
      try {
        await completeSetup(owner.page, link, mailed, team)
      } finally {
        await owner.context.close()
      }

      // --- No email provider: shown once ------------------------------------
      await removeEmail(page)
      const handed = runEmail('create', 'handed')
      await createUser(page, handed, team)
      const shown = (
        await page.getByTestId('link-reveal-value').innerText()
      ).trim()
      expect(shown).toContain('/account/setup#token=')
      await page.getByRole('button', { name: 'Done' }).click()
      // Reopening the dialog shows nothing of it.
      await page.getByRole('button', { name: /New user/ }).click()
      await expect(page.getByRole('dialog')).not.toContainText(
        shown.split('#token=')[1]
      )
      await page.keyboard.press('Escape')

      const second = await freshPage(browser)
      try {
        await completeSetup(
          second.page,
          new URL(shown).pathname + new URL(shown).hash,
          handed,
          team
        )
      } finally {
        await second.context.close()
      }
    })
  })
})
