import { test, expect, type Page } from '../../fixtures/auth'
import { generateBlueprintData } from '../../fixtures/test-data'

/**
 * Feature Tests: Blueprint CRUD Operations
 *
 * Blueprints live at /blueprints with :project/:slug detail URLs. Creation
 * requires picking a project in a Radix Select — every dev-login team ships
 * with a default project, so the first option is always available.
 */

/** Fill and submit the blueprint create form; resolves on the detail page. */
async function createBlueprint(
  page: Page,
  blueprint: { title: string; slug: string; content: string }
): Promise<void> {
  await page.goto('/blueprints/new')
  await expect(page.getByLabel('Title')).toBeVisible({ timeout: 10000 })

  await page.getByLabel('Title').fill(blueprint.title)
  await page.getByLabel('Slug').fill(blueprint.slug)
  await page.getByLabel('Content', { exact: true }).fill(blueprint.content)

  // Project is a Radix Select with no default — pick the first project.
  // (Targeted by testid: the header project switcher's accessible name also
  // contains "Project", so a role+name lookup is ambiguous since #78.)
  await page.getByTestId('blueprint-project-select').click()
  await page.getByRole('option').first().click()

  await page.getByRole('button', { name: 'Create blueprint' }).click()

  // Success navigates to /blueprints/:project/:slug
  await page.waitForURL(/\/blueprints\/[^/]+\/[^/]+$/, { timeout: 15000 })
}

test.describe('Blueprint CRUD Operations', () => {
  test('should display blueprints list page', async ({ authenticatedPage }) => {
    await authenticatedPage.goto('/blueprints')
    await expect(authenticatedPage).toHaveURL(/blueprints$/)

    await expect(
      authenticatedPage.getByRole('heading', {
        name: 'Blueprints',
        exact: true,
      })
    ).toBeVisible({ timeout: 10000 })
    await expect(
      authenticatedPage.getByRole('button', { name: 'New blueprint' }).first()
    ).toBeVisible()
  })

  test('should show empty state for a fresh user', async ({
    authenticatedPage,
  }) => {
    // Dev login mints a new user per test, so the list starts empty.
    await authenticatedPage.goto('/blueprints')
    await expect(authenticatedPage.getByText('No blueprints yet')).toBeVisible({
      timeout: 10000,
    })
  })

  test('should create a blueprint and land on its detail view', async ({
    authenticatedPage,
  }) => {
    const blueprint = generateBlueprintData()
    await createBlueprint(authenticatedPage, blueprint)

    await expect(
      authenticatedPage.getByRole('heading', { name: blueprint.title })
    ).toBeVisible({ timeout: 10000 })
    expect(authenticatedPage.url()).toContain(blueprint.slug)
  })

  test('should validate required fields on create', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/blueprints/new')
    await expect(authenticatedPage.getByLabel('Title')).toBeVisible({
      timeout: 10000,
    })

    await authenticatedPage
      .getByRole('button', { name: 'Create blueprint' })
      .click()

    // Zod validation keeps us on the create page and surfaces messages.
    await expect(authenticatedPage.getByText('Title is required')).toBeVisible({
      timeout: 5000,
    })
    await expect(authenticatedPage).toHaveURL(/blueprints\/new/)
  })

  test('should show the created blueprint in the list', async ({
    authenticatedPage,
  }) => {
    const blueprint = generateBlueprintData()
    await createBlueprint(authenticatedPage, blueprint)

    await authenticatedPage.goto('/blueprints')
    await expect(
      authenticatedPage.getByText(blueprint.title).first()
    ).toBeVisible({ timeout: 10000 })
  })

  test('should edit blueprint content', async ({ authenticatedPage }) => {
    const blueprint = generateBlueprintData()
    await createBlueprint(authenticatedPage, blueprint)

    // The detail page's Edit action navigates to .../edit
    await authenticatedPage.getByRole('button', { name: 'Edit' }).click()
    await expect(authenticatedPage).toHaveURL(
      /blueprints\/[^/]+\/[^/]+\/edit$/,
      { timeout: 10000 }
    )

    const contentField = authenticatedPage.getByLabel('Content', {
      exact: true,
    })
    await expect(contentField).toBeVisible({ timeout: 10000 })
    await contentField.fill('Updated blueprint content from E2E')

    await authenticatedPage
      .getByRole('button', { name: 'Save changes' })
      .click()

    await authenticatedPage.waitForURL(/\/blueprints\/[^/]+\/[^/]+$/, {
      timeout: 15000,
    })
    await expect(
      authenticatedPage.getByText('Updated blueprint content from E2E')
    ).toBeVisible({ timeout: 10000 })
  })

  test('should delete a blueprint from the list', async ({
    authenticatedPage,
  }) => {
    const blueprint = generateBlueprintData()
    await createBlueprint(authenticatedPage, blueprint)

    await authenticatedPage.goto('/blueprints')
    await expect(
      authenticatedPage.getByText(blueprint.title).first()
    ).toBeVisible({ timeout: 10000 })

    // Row actions: View / Edit / Delete icon buttons (aria-labels).
    await authenticatedPage
      .getByRole('row')
      .filter({ hasText: blueprint.title })
      .getByRole('button', { name: 'Delete' })
      .click()

    const confirmDialog = authenticatedPage.getByRole('alertdialog')
    await expect(confirmDialog).toBeVisible({ timeout: 5000 })
    await expect(confirmDialog).toContainText('Delete blueprint?')

    await confirmDialog.getByRole('button', { name: 'Delete' }).click()
    await expect(confirmDialog).not.toBeVisible({ timeout: 5000 })

    await expect(authenticatedPage.getByText('No blueprints yet')).toBeVisible({
      timeout: 10000,
    })
  })
})
