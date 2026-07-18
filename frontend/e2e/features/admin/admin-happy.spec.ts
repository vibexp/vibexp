import { test, expect, devLogin } from '../../fixtures/auth'

import { ADMIN_EMAIL } from './admin-emails'

/**
 * Instance-admin happy path (#317, epic #309): an admin sees the Admin Portal
 * menu entry and can browse the whole read-only surface — stats → users → user
 * detail → teams → team detail.
 */
test.describe('Admin portal — happy path', () => {
  test('an instance admin can browse stats, users, and teams', async ({
    page,
  }) => {
    await devLogin(page, ADMIN_EMAIL, 'Admin E2E')

    // The Admin Portal entry is visible in the user menu for an instance admin.
    await page.getByTestId('user-menu').click()
    const adminItem = page.getByRole('menuitem', { name: 'Admin Portal' })
    await expect(adminItem).toBeVisible()
    await adminItem.click()

    // Lands on the stats dashboard.
    await expect(page).toHaveURL(/\/admin$/)
    await expect(
      page.getByRole('heading', { name: 'Admin Portal' })
    ).toBeVisible()
    await expect(page.getByText('Version')).toBeVisible()

    // Users list → the admin's own row → user detail (team memberships).
    await page.getByRole('link', { name: 'Users' }).click()
    await expect(page).toHaveURL(/\/admin\/users$/)
    // Exact match: nonadmin-e2e@vibexp.test contains admin-e2e@vibexp.test as a
    // substring, so a non-exact getByText would match both rows.
    await page.getByText(ADMIN_EMAIL, { exact: true }).click()
    await expect(page).toHaveURL(/\/admin\/users\/[^/]+$/)
    await expect(
      page.getByRole('heading', { name: 'Team memberships' })
    ).toBeVisible()

    // Teams list → first team → team detail (member list).
    await page.getByRole('link', { name: 'Teams' }).click()
    await expect(page).toHaveURL(/\/admin\/teams$/)
    const firstTeamRow = page.locator('tbody tr').first()
    await expect(firstTeamRow).toBeVisible()
    await firstTeamRow.click()
    await expect(page).toHaveURL(/\/admin\/teams\/[^/]+$/)
    await expect(page.getByRole('heading', { name: 'Members' })).toBeVisible()
  })
})
