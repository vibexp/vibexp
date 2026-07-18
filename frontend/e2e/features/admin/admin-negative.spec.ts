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
    await expect(
      page.getByRole('heading', { name: 'Admin Portal' })
    ).toHaveCount(0)

    // The API surface itself 404s for a non-admin (not advertised — not 403).
    const response = await page.request.get('/api/v1/admin/stats')
    expect(response.status()).toBe(404)
  })
})
