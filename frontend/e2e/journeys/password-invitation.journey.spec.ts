import { expect, test } from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'
import { e2eAppAvailable } from '../helpers/e2eApp'
import {
  createTeam,
  emailedInvitationLink,
  freshPage,
  passwordFor,
  RUN_ID,
  runEmail,
  setPolicy,
  useMailpit,
  whoAmI,
  type InstanceAuthSnapshot,
  restoreInstanceAuth,
  snapshotInstanceAuth,
} from '../helpers/passwordAuth'

/**
 * Journey 3 of #1444: a team invitation taken up with a password (#1435,
 * #1440).
 *
 * A team admin invites a new address. The invitee opens the emailed link in a
 * browser with no session, sees the invitation, creates an account with a
 * password, and lands in the team with the welcome toast.
 */
const UI_TIMEOUT = 20_000

test.describe.serial('Password journeys: team invitation', () => {
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

  test('a new person joins a team through the invitation link', async ({
    page,
    browser,
  }) => {
    test.setTimeout(120_000)

    await setPolicy(page, {
      password_login_enabled: true,
      registration_mode: 'open',
    })
    await useMailpit(page)
    const team = `E2E Invite Team ${RUN_ID}`
    const teamId = await createTeam(page, team)
    const invitee = runEmail('invite', 'new')
    const sent = await page.request.post(
      `/api/v1/teams/${teamId}/invitations`,
      {
        data: { emails: [invitee], role: 'member' },
      }
    )
    expect(sent.ok(), await sent.text()).toBeTruthy()

    const link = await emailedInvitationLink(invitee)
    const { context, page: person } = await freshPage(browser)
    try {
      await person.goto(link)
      const preview = person.getByTestId('invitation-preview')
      await expect(preview).toContainText(team, { timeout: UI_TIMEOUT })
      await expect(preview).toContainText(invitee)

      const form = person.getByRole('form', { name: 'Create your account' })
      const password = passwordFor('invitee')
      await form.getByLabel('Name (optional)').fill(`Invitee ${RUN_ID}`)
      await form.getByLabel('Password', { exact: true }).fill(password)
      await form.getByLabel('Repeat the password').fill(password)
      await form
        .getByRole('button', { name: 'Create account and join' })
        .click()

      await expect(person).toHaveURL(new RegExp(`/teams/${teamId}`), {
        timeout: UI_TIMEOUT,
      })
      await expect(person.getByText(`Welcome to ${team}`)).toBeVisible({
        timeout: UI_TIMEOUT,
      })
      expect(await whoAmI(person)).toBe(invitee)
    } finally {
      await context.close()
    }
  })
})
