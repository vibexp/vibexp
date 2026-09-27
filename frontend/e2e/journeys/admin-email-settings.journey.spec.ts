import { expect, type Page, test } from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'

/**
 * Admin → Settings → Email, end to end (issue #1191, epic #1185).
 *
 * As the instance admin: open the page from the sidebar's Settings group, see
 * the unconfigured state, configure SMTP against the stack's Mailpit, send a
 * test (success shown), save, find the save in the change history, then remove
 * the configuration and land back on the unconfigured state.
 *
 * Mailpit shares the app container's network namespace in
 * `docker-compose.e2e.yml`, so the app reaches it at 127.0.0.1:1025 — the SMTP
 * provider authenticates with net/smtp's PlainAuth, which only sends
 * credentials in the clear to localhost. Mailpit accepts any credentials. The
 * dev stack (`make backend-run-dev`) publishes Mailpit on the same port, so the
 * spec runs there too.
 *
 * The instance provider is a singleton, so the spec clears it before and after
 * itself (a retry starts from the same state). Its values are scoped per
 * attempt so the history assertions can only match this attempt's entries.
 */

const UI_TIMEOUT = 20_000
const stamp = Date.now().toString(36)
const FROM_ADDRESS = `instance-${stamp}@vibexp.test`

/** Remove any stored configuration; a 409 means there was none. */
async function clearInstanceEmail(page: Page): Promise<void> {
  const response = await page.request.delete('/api/v1/admin/settings/email')
  expect([204, 409]).toContain(response.status())
}

test.describe.serial('Admin → Settings → Email', () => {
  test.beforeEach(async ({ page }) => {
    await devLogin(page, ADMIN_EMAIL, ADMIN_NAME)
    await clearInstanceEmail(page)
  })

  test.afterEach(async ({ page }) => {
    await clearInstanceEmail(page)
  })

  test('configure Mailpit, send a test, audit the save, then remove it', async ({
    page,
  }) => {
    test.setTimeout(90_000)

    // --- Reach the page from the sidebar's Settings group -----------------
    await page.goto('/admin')
    const nav = page.getByRole('navigation', { name: 'Admin sections' })
    await nav.getByRole('link', { name: 'Email' }).click()
    await expect(page).toHaveURL(/\/admin\/settings\/email$/)
    await expect(
      page.getByRole('heading', { name: 'Email', level: 1 })
    ).toBeVisible()

    const status = page.getByTestId('instance-email-status')
    await expect(status).toContainText('Instance email is not configured', {
      timeout: UI_TIMEOUT,
    })
    await expect(
      page.getByRole('button', { name: 'Remove configuration' })
    ).toHaveCount(0)

    // --- Configure SMTP → Mailpit -----------------------------------------
    await expect(page.getByRole('radio', { name: /SMTP/ })).toBeChecked()
    await page.getByLabel('Host', { exact: true }).fill('127.0.0.1')
    await page.getByLabel('Port', { exact: true }).fill('1025')
    await page.getByLabel('Username', { exact: true }).fill('e2e')
    await page
      .getByLabel('SMTP password', { exact: true })
      .fill('e2e-mailpit-password')
    await page.getByLabel('From address', { exact: true }).fill(FROM_ADDRESS)
    await page.getByLabel('Display name', { exact: true }).fill('VibeXP E2E')

    // --- Test send of the unsaved values ----------------------------------
    await page.getByRole('button', { name: 'Send test email' }).click()
    const result = page.getByTestId('instance-email-test-result')
    await expect(result).toContainText('Test email sent', {
      timeout: UI_TIMEOUT,
    })
    await expect(result).toContainText(`Sent to ${ADMIN_EMAIL}.`)

    // --- Save ---------------------------------------------------------------
    await page.getByRole('button', { name: 'Save changes' }).click()
    await expect(status).toContainText('Instance provider', {
      timeout: UI_TIMEOUT,
    })
    await expect(status).toContainText(
      `Sending through SMTP as ${FROM_ADDRESS}`
    )
    await expect(status).toContainText('A credential is stored.')
    await expect(
      page.getByRole('button', { name: 'Remove configuration' })
    ).toBeVisible()

    // A reload shows the stored values, with the secret left blank.
    await page.reload()
    await expect(page.getByLabel('From address', { exact: true })).toHaveValue(
      FROM_ADDRESS,
      {
        timeout: UI_TIMEOUT,
      }
    )
    await expect(page.getByLabel('Host', { exact: true })).toHaveValue(
      '127.0.0.1'
    )
    await expect(page.getByLabel('SMTP password', { exact: true })).toHaveValue(
      ''
    )

    // --- The save is in the change history --------------------------------
    const saved = page
      .getByTestId('instance-email-audit-entry')
      .filter({ hasText: 'Saved' })
      .filter({ hasText: FROM_ADDRESS })
    await expect(saved).toHaveCount(1, { timeout: UI_TIMEOUT })
    await expect(saved).toContainText(ADMIN_NAME)
    await expect(saved).toContainText('Credential changed')
    await expect(saved).toContainText('SMTP host')

    // --- Remove ---------------------------------------------------------------
    await page.getByRole('button', { name: 'Remove configuration' }).click()
    const dialog = page.getByRole('alertdialog')
    await expect(dialog).toContainText(
      'Teams with their own email provider are unaffected'
    )
    await dialog.getByRole('button', { name: 'Remove' }).click()

    await expect(status).toContainText('Instance email is not configured', {
      timeout: UI_TIMEOUT,
    })
    await expect(page.getByLabel('From address', { exact: true })).toHaveValue(
      ''
    )
    await expect(
      page
        .getByTestId('instance-email-audit-entry')
        .filter({ hasText: 'Removed' })
        .filter({ hasText: FROM_ADDRESS })
    ).toHaveCount(1, { timeout: UI_TIMEOUT })
  })
})
