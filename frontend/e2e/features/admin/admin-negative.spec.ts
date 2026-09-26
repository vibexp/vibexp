import { test, expect, devLogin } from '../../fixtures/auth'

import { NON_ADMIN_EMAIL } from './admin-emails'

/**
 * Non-admin negative path (#317, epic #309): a regular user must not see the
 * Admin Portal entry, must be redirected away from /admin, and the admin API
 * itself must 404 (surface deliberately not advertised — not 403).
 */
test.describe('Admin portal — non-admin negative path', () => {
  test('a non-admin cannot see or reach the admin surface', async ({
    page,
  }) => {
    await devLogin(page, NON_ADMIN_EMAIL, 'Non Admin E2E')

    // No Admin Portal item in the user menu (Settings is present as a control).
    await page.getByTestId('user-menu').click()
    await expect(page.getByRole('menuitem', { name: 'Settings' })).toBeVisible()
    await expect(
      page.getByRole('menuitem', { name: 'Admin Portal' })
    ).toHaveCount(0)
    await page.keyboard.press('Escape')

    // Direct URL entry is redirected away from /admin back to the home page.
    await page.goto('/admin')
    await expect(page).toHaveURL(/^https?:\/\/[^/]+\/$/)
    // Assert the absence of chrome that the admin shell — and only the admin
    // shell — renders: AdminHeader's "Back to app" link. This used to look for
    // an `Admin Portal` *heading*, which #456 deleted, so no element could
    // produce it any more and the assertion passed even with the route guard
    // gone. A negative assertion is only worth anything if the thing it names
    // still exists on the failing path.
    await expect(page.getByRole('link', { name: 'Back to app' })).toHaveCount(0)

    // The API surface itself 404s for a non-admin (not advertised — not 403).
    const response = await page.request.get('/api/v1/admin/stats')
    expect(response.status()).toBe(404)
  })

  test('a non-admin is kept out of every admin panel v3 surface', async ({
    page,
  }) => {
    await devLogin(page, NON_ADMIN_EMAIL, 'Non Admin E2E')

    // A team that EXISTS — the user's own. The admin team-config endpoints 404
    // an unknown id even for an admin, so a made-up id could not tell the admin
    // guard apart from "no such team" and the check below could never fail.
    const teamsRes = await page.request.get('/api/v1/teams')
    expect(teamsRes.ok()).toBe(true)
    const { teams } = (await teamsRes.json()) as { teams: { id: string }[] }
    const ownTeamId = teams[0]?.id
    expect(ownTeamId, 'the non-admin has no team to probe with').toBeTruthy()

    // The v3 list pages and a detail URL (#1131) redirect home without chrome.
    for (const path of [
      '/admin/users',
      '/admin/teams',
      '/admin/projects',
      `/admin/teams/${ownTeamId}?tab=search`,
    ]) {
      await page.goto(path)
      await expect(page, `${path} was not redirected`).toHaveURL(
        /^https?:\/\/[^/]+\/$/
      )
      await expect(page.getByRole('link', { name: 'Back to app' })).toHaveCount(
        0
      )
    }

    // Presets, export and team configuration 404 like the rest of /admin.
    for (const url of [
      '/api/v1/admin/saved-filters/users',
      '/api/v1/admin/users/export',
      `/api/v1/admin/teams/${ownTeamId}/config/search`,
    ]) {
      const res = await page.request.get(url)
      expect(res.status(), `GET ${url}`).toBe(404)
    }
  })
})
