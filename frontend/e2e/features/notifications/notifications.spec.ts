import {
  buildMockNotification,
  mockNotificationsApi,
} from '../../fixtures/api-mocks'
import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: Notifications
 *
 * In-app notifications are produced by backend domain events that E2E can't
 * reliably trigger, so list/mark-as-read flows run on a stateful route mock.
 * The /settings/notifications preferences page talks to the real backend.
 */
test.describe('Notifications', () => {
  test('should show empty state for a fresh user', async ({
    authenticatedPage,
  }) => {
    // Fresh dev-login user — no mock needed, the real list is empty.
    await authenticatedPage.goto('/notifications')

    await expect(
      authenticatedPage.getByRole('heading', { name: 'Notifications' })
    ).toBeVisible({ timeout: 10000 })
    await expect(
      authenticatedPage.getByText("You're all caught up")
    ).toBeVisible({ timeout: 10000 })
  })

  test('should list notifications and filter unread', async ({
    authenticatedPage,
  }) => {
    await mockNotificationsApi(authenticatedPage, [
      buildMockNotification({ id: 'n-1', title: 'Unread notification one' }),
      buildMockNotification({
        id: 'n-2',
        title: 'Already read notification',
        read_at: new Date().toISOString(),
      }),
    ])

    await authenticatedPage.goto('/notifications')
    await expect(
      authenticatedPage.getByText('Unread notification one')
    ).toBeVisible({ timeout: 10000 })
    await expect(
      authenticatedPage.getByText('Already read notification')
    ).toBeVisible()

    await authenticatedPage
      .getByRole('button', { name: 'Unread', exact: true })
      .click()

    await expect(
      authenticatedPage.getByText('Unread notification one')
    ).toBeVisible({ timeout: 10000 })
    await expect(
      authenticatedPage.getByText('Already read notification')
    ).not.toBeVisible()
  })

  test('should mark all notifications as read', async ({
    authenticatedPage,
  }) => {
    await mockNotificationsApi(authenticatedPage, [
      buildMockNotification({ id: 'n-1', title: 'Mark-all target one' }),
      buildMockNotification({ id: 'n-2', title: 'Mark-all target two' }),
    ])

    await authenticatedPage.goto('/notifications')
    await expect(
      authenticatedPage.getByText('Mark-all target one')
    ).toBeVisible({ timeout: 10000 })

    await authenticatedPage
      .getByRole('button', { name: 'Mark all read' })
      .click()

    // The action button disappears once nothing is unread.
    await expect(
      authenticatedPage.getByRole('button', { name: 'Mark all read' })
    ).not.toBeVisible({ timeout: 10000 })

    // The unread filter is now empty.
    await authenticatedPage
      .getByRole('button', { name: 'Unread', exact: true })
      .click()
    await expect(
      authenticatedPage.getByText('No unread notifications.')
    ).toBeVisible({ timeout: 10000 })
  })

  test('should render notification preferences with toggles', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/settings/notifications')

    await expect(
      authenticatedPage.getByText('Email me about activity in my teams')
    ).toBeVisible({ timeout: 15000 })
    // Preference rows are Radix Switches.
    expect(await authenticatedPage.getByRole('switch').count()).toBeGreaterThan(
      0
    )
  })

  test('should save a preference change', async ({ authenticatedPage }) => {
    await authenticatedPage.goto('/settings/notifications')

    const teamActivityToggle = authenticatedPage.getByRole('switch', {
      name: 'Email me about activity in my teams',
    })
    await expect(teamActivityToggle).toBeVisible({ timeout: 15000 })

    await teamActivityToggle.click()
    await authenticatedPage
      .getByRole('button', { name: 'Save changes' })
      .click()

    await expect(
      authenticatedPage.getByText('Preferences saved successfully.')
    ).toBeVisible({ timeout: 10000 })
  })
})
