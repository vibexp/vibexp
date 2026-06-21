import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: v2 Shell Showcase
 *
 * /showcase is a static component gallery (sample data, no API calls) used
 * to validate the shadcn v2 primitives end-to-end: page header, data table
 * with sorting, form validation, confirm dialog, and toasts.
 */
test.describe('Showcase', () => {
  test('should render the showcase page with its data table', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/showcase')
    await expect(authenticatedPage).toHaveURL(/showcase$/)

    await expect(
      authenticatedPage.getByRole('heading', { name: 'v2 shell showcase' })
    ).toBeVisible({ timeout: 10000 })

    // Sample table rows render.
    await expect(
      authenticatedPage.getByText('Customer onboarding')
    ).toBeVisible()
    await expect(authenticatedPage.getByText('Invoice generator')).toBeVisible()
  })

  test('should open and confirm the dialog from "New item"', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/showcase')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'v2 shell showcase' })
    ).toBeVisible({ timeout: 10000 })

    await authenticatedPage
      .getByRole('button', { name: 'New item' })
      .first()
      .click()

    const dialog = authenticatedPage.getByRole('alertdialog')
    await expect(dialog).toBeVisible({ timeout: 5000 })
    await expect(dialog).toContainText('Create new item?')

    await dialog.getByRole('button', { name: 'Create' }).click()
    await expect(dialog).not.toBeVisible({ timeout: 10000 })

    // Confirmation fires a sonner toast.
    await expect(authenticatedPage.getByText('Confirmed')).toBeVisible({
      timeout: 5000,
    })
  })

  test('should show a toast from the "Toast me" action', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/showcase')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'v2 shell showcase' })
    ).toBeVisible({ timeout: 10000 })

    await authenticatedPage.getByRole('button', { name: 'Toast me' }).click()
    await expect(authenticatedPage.getByText('Hello from sonner')).toBeVisible({
      timeout: 5000,
    })
  })

  test('should validate the sample form', async ({ authenticatedPage }) => {
    await authenticatedPage.goto('/showcase')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'v2 shell showcase' })
    ).toBeVisible({ timeout: 10000 })

    // The sample form lives in the "Form" tab.
    await authenticatedPage.getByRole('tab', { name: 'Form' }).click()

    // Submitting the sample form empty triggers zod validation.
    await authenticatedPage.getByRole('button', { name: 'Submit' }).click()
    await expect(
      authenticatedPage.getByText('Title must be at least 2 characters')
    ).toBeVisible({ timeout: 5000 })
  })
})
