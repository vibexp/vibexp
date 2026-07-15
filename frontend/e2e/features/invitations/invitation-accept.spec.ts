import { expect, test } from '@playwright/test'
import type { Browser, Page } from '@playwright/test'

import { devLogin, getCurrentTeam } from '../../fixtures/auth'

/**
 * Feature Test: Team Invitation Accept
 *
 * Covers the public `/invitations/accept/:token` route and the in-app accept
 * (issues #66, #251, #252).
 *
 * A full two-user accept IS a clean e2e: `GET /api/v1/invitations/pending`
 * returns the invitation's `token` (`buildInvitationResponses` populates it), and
 * dev login provisions a brand-new user per call, so one browser context per user
 * is all the setup needed.
 *
 * (A previous version of this file claimed the token "is intentionally never
 * exposed by the API — see convertInvitationsToResponses, which omits it". That
 * conflated two functions: `convertInvitationsToResponses` serves the *team's*
 * invitation list and does omit the token; `buildInvitationResponses` serves the
 * *pending* list and includes it. Acting on that false premise, this file tested
 * only an unresolvable token — a negative assertion the 100%-broken real flow
 * satisfied just as well, so the suite stayed green right through #251.)
 *
 * What this proves end-to-end: an invited user actually joins the team, via both
 * the in-app list and the emailed link, using a REAL token — which is what
 * exercises the percent-encoding that #251 broke.
 */

/** Dev login mints a fresh user per call; unique per worker+run to stay parallel-safe. */
const uniqueEmail = (prefix: string) =>
  `${prefix}_${process.env.TEST_WORKER_INDEX ?? '0'}_${Date.now()}@example.com`

interface Session {
  page: Page
  close: () => Promise<void>
}

/** A signed-in inviter who owns a freshly created team. */
async function createInviterWithTeam(
  browser: Browser,
  teamName: string
): Promise<Session & { teamId: string }> {
  const context = await browser.newContext()
  const page = await context.newPage()
  await devLogin(page, uniqueEmail('e2e_inviter'), 'E2E Inviter')

  // Team creation is setup, not the behaviour under test — drive it over the API
  // so a change to the create-team dialog cannot fail the accept test.
  const response = await page.request.post('/api/v1/teams', {
    data: {
      name: teamName,
      description: 'Created by the invitation accept e2e',
    },
  })
  expect(response.ok(), `create team failed: ${response.status()}`).toBeTruthy()

  const payload = (await response.json()) as {
    id?: string
    team?: { id?: string }
  }
  const teamId = payload.team?.id ?? payload.id
  expect(teamId, 'create team response carried no team id').toBeTruthy()

  return { page, teamId: teamId as string, close: () => context.close() }
}

/** Invites `email` to `teamId` as the already-signed-in inviter. */
async function inviteToTeam(inviterPage: Page, teamId: string, email: string) {
  const response = await inviterPage.request.post(
    `/api/v1/teams/${teamId}/invitations`,
    { data: { emails: [email], role: 'member' } }
  )
  expect(
    response.ok(),
    `send invitation failed: ${response.status()}`
  ).toBeTruthy()
}

/** A signed-in invitee, in their own context so both sessions coexist. */
async function signInInvitee(
  browser: Browser,
  email: string
): Promise<Session> {
  const context = await browser.newContext()
  const page = await context.newPage()
  await devLogin(page, email, 'E2E Invitee')
  return { page, close: () => context.close() }
}

/** Pending-invitation count for the signed-in user. */
async function pendingCount(page: Page): Promise<number> {
  const res = await page.request.get('/api/v1/invitations/pending')
  const body = (await res.json()) as { invitations?: unknown[] }
  return body.invitations?.length ?? 0
}

test.describe('Team Invitation Accept', () => {
  test('an invited user accepts from the pending list and joins the team', async ({
    browser,
  }) => {
    const teamName = `E2E Invite Team ${Date.now()}`
    const inviteeEmail = uniqueEmail('e2e_invitee')

    const inviter = await createInviterWithTeam(browser, teamName)
    let invitee: Session | undefined

    try {
      await inviteToTeam(inviter.page, inviter.teamId, inviteeEmail)
      invitee = await signInInvitee(browser, inviteeEmail)

      await invitee.page.goto('/settings/teams')

      // The team NAME must render. It was silently "" before #251 (the lookup ran
      // through TeamService.GetTeam, which requires a membership a pending invitee
      // does not have), so the banner read "X invited you to .".
      await expect(
        invitee.page.getByText(teamName, { exact: false }).first()
      ).toBeVisible({ timeout: 15000 })

      const acceptButton = invitee.page
        .getByRole('button', { name: /^accept$/i })
        .first()
      await expect(acceptButton).toBeVisible({ timeout: 15000 })
      await acceptButton.click()

      // Assert real membership rather than a transient toast: the invitation is
      // consumed and the team is now part of the invitee's context.
      await expect
        .poll(async () => await pendingCount(invitee!.page), { timeout: 15000 })
        .toBe(0)

      await expect
        .poll(async () => await getCurrentTeam(invitee!.page), {
          timeout: 15000,
        })
        .toBeTruthy()

      await expect(
        invitee.page.getByText(teamName, { exact: false }).first()
      ).toBeVisible({ timeout: 15000 })
    } finally {
      await invitee?.close()
      await inviter.close()
    }
  })

  test('an invited user accepts via the emailed invitation link', async ({
    browser,
  }) => {
    const teamName = `E2E Link Team ${Date.now()}`
    const inviteeEmail = uniqueEmail('e2e_link_invitee')

    const inviter = await createInviterWithTeam(browser, teamName)
    let invitee: Session | undefined

    try {
      await inviteToTeam(inviter.page, inviter.teamId, inviteeEmail)
      invitee = await signInInvitee(browser, inviteeEmail)

      // Read the REAL token the emailed link would carry. This is the case that
      // would have caught #251: the token is padded base64, the client
      // percent-encodes its '=' to %3D, and the backend must decode it before the
      // exact-match lookup. A synthetic '='-free token never exercises that.
      const pending = await invitee.page.request.get(
        '/api/v1/invitations/pending'
      )
      expect(pending.ok()).toBeTruthy()

      const body = (await pending.json()) as {
        invitations?: { token: string; team_name: string }[]
      }
      const invitation = body.invitations?.[0]
      expect(invitation, 'no pending invitation returned').toBeTruthy()
      expect(
        invitation!.token,
        'pending invitation carried no token'
      ).toBeTruthy()
      expect(invitation!.team_name).toBe(teamName)

      await invitee.page.goto(
        `/invitations/accept/${encodeURIComponent(invitation!.token)}`
      )

      // The landing page resolves the token (GET /api/v1/invitations/{token}) and
      // names the team before offering to accept.
      await expect(
        invitee.page.getByText(teamName, { exact: false }).first()
      ).toBeVisible({ timeout: 15000 })

      await invitee.page
        .getByRole('button', { name: /^accept(\s+invitation)?$/i })
        .first()
        .click()

      await expect
        .poll(async () => await pendingCount(invitee!.page), { timeout: 15000 })
        .toBe(0)
    } finally {
      await invitee?.close()
      await inviter.close()
    }
  })

  test('an unresolvable invitation token renders a graceful error', async ({
    browser,
  }) => {
    // The negative counterpart to the two positives above. On its own it proved
    // nothing: the 100%-broken flow produced this very same error card (#252).
    const invitee = await signInInvitee(browser, uniqueEmail('e2e_bad_token'))

    try {
      await invitee.page.goto(
        `/invitations/accept/nonexistent-e2e-token-${Date.now()}`
      )

      await expect(
        invitee.page.getByRole('button', { name: /go to dashboard/i })
      ).toBeVisible({ timeout: 15000 })
      await expect(invitee.page.getByRole('alert')).toBeVisible()
    } finally {
      await invitee.close()
    }
  })
})
