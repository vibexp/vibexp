import { expect, test } from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'
import { e2eAppAvailable } from '../helpers/e2eApp'
import {
  createTeam,
  emailedInvitationLink,
  freshPage,
  passwordFor,
  passwordSignIn,
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
 * Journey 7 of #1444: registration by invitation only (#1434, #1442).
 *
 * With the registration mode `invite_only`, an INVITED address still creates
 * its account with a password through the invitation, and with no identity
 * provider on the stack the invitation page shows the password form and no
 * provider choices.
 *
 * The sign-in page checks (no way to create an account there, an unknown
 * address refused like a wrong password) are baseline checks: the sign-in page
 * reads no registration mode, so they hold in every mode and are not a gate
 * on `invite_only` itself.
 *
 * NOT covered here: a provider sign-up being refused (`registration_closed`).
 * The e2e stack has no identity provider, and that gate guards only a new
 * account created by a provider sign-in; it is covered by the backend tests
 * of #1434 and #1430.
 */
const UI_TIMEOUT = 20_000

test.describe.serial('Password journeys: invite only', () => {
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

  test('only an invitation creates an account', async ({ page, browser }) => {
    test.setTimeout(120_000)

    await setPolicy(page, {
      password_login_enabled: true,
      registration_mode: 'invite_only',
    })
    await useMailpit(page)

    // --- The sign-in page: no way to create an account ----------------------
    const stranger = await freshPage(browser)
    try {
      await stranger.page.goto('/login')
      const form = stranger.page.getByRole('form', {
        name: 'Password sign-in',
      })
      await expect(form).toBeVisible({ timeout: UI_TIMEOUT })
      await expect(
        stranger.page.getByRole('link', {
          name: /sign up|create an account|register/i,
        })
      ).toHaveCount(0)
      await expect(
        stranger.page.getByRole('button', {
          name: /sign up|create an account|register/i,
        })
      ).toHaveCount(0)

      // An unknown address gets the generic refusal and no account.
      await form.getByLabel('Email').fill(runEmail('inviteonly', 'stranger'))
      await form
        .getByLabel('Password', { exact: true })
        .fill(passwordFor('stranger'))
      await form.getByRole('button', { name: 'Sign in' }).click()
      await expect(
        stranger.page.getByTestId('password-sign-in-error')
      ).toBeVisible({ timeout: UI_TIMEOUT })
      await expect(stranger.page).toHaveURL(/\/login/)
      expect(await whoAmI(stranger.page)).toBeNull()
    } finally {
      await stranger.context.close()
    }

    // --- An invitation still creates the account ---------------------------
    const team = `E2E Invite Only ${RUN_ID}`
    const teamId = await createTeam(page, team)
    const invitee = runEmail('inviteonly', 'invited')
    const sent = await page.request.post(
      `/api/v1/teams/${teamId}/invitations`,
      {
        data: { emails: [invitee], role: 'member' },
      }
    )
    expect(sent.ok(), await sent.text()).toBeTruthy()

    const person = await freshPage(browser)
    try {
      await person.page.goto(await emailedInvitationLink(invitee))
      const create = person.page.getByTestId('invitation-create')
      await expect(create).toBeVisible({ timeout: UI_TIMEOUT })
      // No identity provider on the stack: the password form only.
      await expect(person.page.getByTestId('provider-choices')).toHaveCount(0)
      const password = passwordFor('invited')
      const form = person.page.getByRole('form', {
        name: 'Create your account',
      })
      await form.getByLabel('Password', { exact: true }).fill(password)
      await form.getByLabel('Repeat the password').fill(password)
      await form
        .getByRole('button', { name: 'Create account and join' })
        .click()
      await expect(person.page).toHaveURL(new RegExp(`/teams/${teamId}`), {
        timeout: UI_TIMEOUT,
      })
      expect(await whoAmI(person.page)).toBe(invitee)
    } finally {
      await person.context.close()
    }

    // And that person can sign in with the password from now on.
    const again = await freshPage(browser)
    try {
      await passwordSignIn(again.page, invitee, passwordFor('invited'))
      expect(await whoAmI(again.page)).toBe(invitee)
    } finally {
      await again.context.close()
    }
  })
})
