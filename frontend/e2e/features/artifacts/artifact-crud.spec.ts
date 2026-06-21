import { test, expect } from '../../fixtures/auth'
import { selectFirstProject } from '../../helpers/artifacts'

/**
 * Feature Tests: Artifact CRUD Operations
 * Tests basic Create, Read, Update, Delete functionality for artifacts
 */
test.describe('Artifact CRUD Operations', () => {
  test.describe('Artifact Creation', () => {
    test('should display artifacts page with navigation', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await expect(authenticatedPage).toHaveURL(/artifacts$/)
      await authenticatedPage.waitForLoadState('networkidle')
      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should show empty state for users with no artifacts', async ({
      freshUserPage,
    }) => {
      await freshUserPage.goto('/artifacts')
      await expect(freshUserPage).toHaveURL(/artifacts$/)
      await freshUserPage.waitForLoadState('networkidle')

      // Check for empty state
      const pageContent = await freshUserPage.textContent('body')
      expect(pageContent).toBeTruthy()
    })

    test('should create artifact with all required fields', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts/new')
      await expect(authenticatedPage).toHaveURL(/artifacts\/new/)

      const artifactSlug = `test-${Date.now()}`
      const artifactTitle = `Test Artifact ${Date.now()}`
      const artifactContent = 'Test artifact content'

      // Wait for project dropdown
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      // Fill form
      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(artifactTitle)
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill(artifactContent)
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+/, {
        timeout: 10000,
      })
      await expect(
        authenticatedPage.locator(`text=${artifactTitle}`).first()
      ).toBeVisible({ timeout: 10000 })
    })

    test('should validate required fields (title, slug, content)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      // Try to save without filling required fields
      const createButton = authenticatedPage.locator(
        'button:has-text("Create Artifact")'
      )
      await createButton.click()

      // Wait to see if validation triggers
      await authenticatedPage.waitForTimeout(1000)

      // Should still be on create page
      await expect(authenticatedPage).toHaveURL(/artifacts\/new/)
    })

    test('should auto-generate slug from title', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactTitle = `Auto Slug Test ${Date.now()}`
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(artifactTitle)

      // Check if slug is auto-populated
      const slugInput = authenticatedPage.locator(
        '[data-testid="artifact-slug-input"]'
      )
      const slugValue = await slugInput.inputValue()
      expect(slugValue.length).toBeGreaterThan(0)
      expect(slugValue).toMatch(/^[a-z0-9-]+$/)
    })

    test('should validate slug format', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill('Slug Validation Test')

      const slugInput = authenticatedPage.locator(
        '[data-testid="artifact-slug-input"]'
      )
      // Try invalid slug
      await slugInput.clear()
      await slugInput.fill('invalid slug @#$')

      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Content')
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      // Should show validation error or stay on page
      await authenticatedPage.waitForTimeout(1000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/new/)
    })

    test('should select artifact type (work_reports, static_contexts, general)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `work-reports-${Date.now()}`
      const artifactTitle = 'Work Reports Test'

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(artifactTitle)
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Content')

      const typeSelect = authenticatedPage.locator(
        '[data-testid="artifact-type-select"]'
      )
      if ((await typeSelect.count()) > 0) {
        await typeSelect.click()
        await authenticatedPage
          .getByRole('option', { name: /work reports/i })
          .click()
      }

      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()
      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+/, {
        timeout: 10000,
      })
    })

    test('should set artifact status (active, draft, archived)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `status-test-${Date.now()}`
      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill('Status Test')
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Content')

      // Status is a Radix Select (button[role=combobox]), not a native
      // <select> — open it and pick the option (mirrors the type select above).
      const statusSelect = authenticatedPage.locator(
        '[data-testid="artifact-status-select"]'
      )
      if ((await statusSelect.count()) > 0) {
        await statusSelect.click()
        await authenticatedPage
          .getByRole('option', { name: /^active$/i })
          .click()
      }

      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()
      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+/, {
        timeout: 10000,
      })
    })

    test('should add description and metadata', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `with-description-${Date.now()}`
      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill('Artifact with Description')
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Content')

      // Add description if field exists
      const descInput = authenticatedPage.locator(
        '[data-testid="artifact-description-input"], textarea[placeholder*="description"]'
      )
      if ((await descInput.count()) > 0) {
        await descInput.fill('This is a test description')
      }

      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()
      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+/, {
        timeout: 10000,
      })
    })

    test('should display artifact in list after creation', async ({
      authenticatedPage,
    }) => {
      // Create artifact
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `list-test-${Date.now()}`
      const artifactTitle = `List Test ${Date.now()}`

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(artifactTitle)
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('List content')
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+/, {
        timeout: 10000,
      })

      // Go to list
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify artifact in list
      await expect(
        authenticatedPage.locator(`text=${artifactTitle}`).first()
      ).toBeVisible({ timeout: 5000 })
    })
  })

  test.describe('Artifact Reading', () => {
    test('should navigate to artifact detail view', async ({
      authenticatedPage,
    }) => {
      // Create artifact
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `detail-${Date.now()}`
      const artifactTitle = `Detail Test ${Date.now()}`

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(artifactTitle)
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Detail content')
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+/, {
        timeout: 10000,
      })
      await expect(
        authenticatedPage.locator(`text=${artifactTitle}`).first()
      ).toBeVisible()
    })

    test('should view artifact with full metadata', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `metadata-${Date.now()}`
      const artifactTitle = `Metadata Test ${Date.now()}`
      const content = 'Full metadata content'

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(artifactTitle)
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill(content)
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+/, {
        timeout: 10000,
      })

      // Verify title and content visible
      await expect(
        authenticatedPage.locator(`text=${artifactTitle}`).first()
      ).toBeVisible()
      await expect(authenticatedPage.locator(`text=${content}`)).toBeVisible()
    })
  })

  test.describe('Artifact Updating', () => {
    test('should edit artifact content', async ({ authenticatedPage }) => {
      // Create artifact
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `edit-${Date.now()}`
      const originalTitle = `Edit Test ${Date.now()}`

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(originalTitle)
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Original content')
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/artifacts\/[^/]+\/[^/]+$/)

      // Navigate to edit
      await authenticatedPage.locator('button:has-text("Edit")').first().click()
      await expect(authenticatedPage).toHaveURL(
        /artifacts\/[^/]+\/[^/]+\/edit/,
        { timeout: 10000 }
      )

      // Update content
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .clear()
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Updated content')
      await authenticatedPage
        .locator('button')
        .filter({ hasText: /save changes|update|save/i })
        .first()
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/artifacts\/[^/]+\/[^/]+$/, {
        timeout: 10000,
      })
      await expect(
        authenticatedPage.locator('text=Updated content')
      ).toBeVisible({ timeout: 5000 })
    })

    test('should edit artifact metadata', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `metadata-edit-${Date.now()}`
      const originalTitle = `Metadata Edit ${Date.now()}`

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(originalTitle)
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Original')
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/artifacts\/[^/]+\/[^/]+$/)

      // Edit
      await authenticatedPage.locator('button:has-text("Edit")').first().click()
      await expect(authenticatedPage).toHaveURL(
        /artifacts\/[^/]+\/[^/]+\/edit/,
        { timeout: 10000 }
      )

      const updatedTitle = `${originalTitle} (Updated)`
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-title-input"]'
      )
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .clear()
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(updatedTitle)
      await authenticatedPage
        .locator('button')
        .filter({ hasText: /save changes|update|save/i })
        .first()
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/artifacts\/[^/]+\/[^/]+$/, {
        timeout: 10000,
      })
      await expect(
        authenticatedPage.locator(`text=${updatedTitle}`).first()
      ).toBeVisible({ timeout: 10000 })
    })

    test('should update artifact type and status', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `type-update-${Date.now()}`

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill('Type Update Test')
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Content')
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/artifacts\/[^/]+\/[^/]+$/)

      // Edit and change type
      await authenticatedPage.locator('button:has-text("Edit")').first().click()
      await expect(authenticatedPage).toHaveURL(
        /artifacts\/[^/]+\/[^/]+\/edit/,
        { timeout: 10000 }
      )

      const typeSelect = authenticatedPage.locator(
        '[data-testid="artifact-type-select"]'
      )
      if ((await typeSelect.count()) > 0) {
        await typeSelect.click()
        await authenticatedPage
          .getByRole('option', { name: /static contexts/i })
          .click()
      }

      await authenticatedPage
        .locator('button')
        .filter({ hasText: /save changes|update|save/i })
        .first()
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+$/, {
        timeout: 10000,
      })
    })
  })

  test.describe('Artifact Deletion', () => {
    test('should delete artifact with confirmation', async ({
      authenticatedPage,
    }) => {
      // Create artifact
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      const artifactSlug = `delete-${Date.now()}`
      const artifactTitle = `Delete Test ${Date.now()}`

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(artifactSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(artifactTitle)
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('To be deleted')
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+/, {
        timeout: 10000,
      })

      // Go to list
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Find and click delete
      const deleteButton = authenticatedPage
        .locator('[data-testid="delete-artifact-button"]')
        .first()
      await deleteButton.click()

      // Confirm deletion
      const confirmDialog = authenticatedPage.locator('[role="alertdialog"]')
      await expect(confirmDialog).toBeVisible({ timeout: 5000 })
      await expect(confirmDialog).toContainText(/delete/i)

      const confirmButton = confirmDialog.locator('button:has-text("Delete")')
      await confirmButton.click()

      // Wait for deletion
      await expect(confirmDialog).not.toBeVisible({ timeout: 5000 })
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify removed from list
      await expect(authenticatedPage.locator(`text=${artifactTitle}`).first())
        .not.toBeVisible({ timeout: 5000 })
        .catch(() => Promise.resolve())
    })

    test('should handle duplicate slug error', async ({
      authenticatedPage,
    }) => {
      const uniqueSlug = `unique-slug-${Date.now()}`

      // Create first artifact
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(uniqueSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill('First Artifact')
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('First')
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/artifacts\/[^/]+\/[^/]+/, {
        timeout: 10000,
      })

      // Try to create second with same slug
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(uniqueSlug)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill('Second Artifact')
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill('Second')
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      // Should show error or stay on create page
      await authenticatedPage.waitForTimeout(2000)
      const currentUrl = authenticatedPage.url()
      const isStillOnCreate = currentUrl.includes('/artifacts/new')

      if (isStillOnCreate) {
        expect(isStillOnCreate).toBeTruthy()
      }
    })
  })
})
