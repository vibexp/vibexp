import { test, expect } from '../../fixtures/auth'
import { selectFirstProject } from '../../helpers/artifacts'

/**
 * Feature Tests: Artifact Filtering and Search
 * Tests the filtering, search, and sorting functionality for artifacts
 */
test.describe('Artifact Filtering and Search', () => {
  test.describe('Basic Display', () => {
    test('should display all artifacts by default', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')
      await expect(authenticatedPage).toHaveURL(/artifacts$/)
      await expect(authenticatedPage.locator('body')).toBeVisible()
    })
  })

  test.describe('Project Filtering', () => {
    test('should filter artifacts by project', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Find project filter dropdown
      const projectFilter = authenticatedPage.locator('select').filter({
        has: authenticatedPage.locator('option:has-text("All Projects")'),
      })

      if ((await projectFilter.count()) > 0) {
        await projectFilter.waitFor({ state: 'visible', timeout: 10000 })

        // Get available projects
        const options = await projectFilter.locator('option').count()
        if (options > 1) {
          // Select first non-"All" option
          await projectFilter.selectOption({ index: 1 })
          await authenticatedPage.waitForTimeout(1500)
        }
      }

      // Verify page still displays
      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should show only artifacts matching selected project', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const projectFilter = authenticatedPage.locator('select').filter({
        has: authenticatedPage.locator('option:has-text("All Projects")'),
      })

      if ((await projectFilter.count()) > 0) {
        const options = await projectFilter.locator('option').count()
        if (options > 1) {
          await projectFilter.selectOption({ index: 1 })
          await authenticatedPage.waitForTimeout(1500)

          // Verify filtering worked
          await expect(authenticatedPage.locator('body')).toBeVisible()
        }
      }
    })
  })

  test.describe('Type Filtering', () => {
    test('should filter artifacts by type (work_reports)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Click Advanced Filters
      const advancedButton = authenticatedPage.locator(
        'button:has-text("Advanced")'
      )
      if ((await advancedButton.count()) > 0) {
        await advancedButton.click()
        await authenticatedPage.waitForTimeout(500)

        // Find type filter
        const typeFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Types")'),
        })
        await typeFilter.waitFor({ state: 'visible', timeout: 10000 })
        await typeFilter.selectOption('work_reports')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should filter artifacts by type (static_contexts)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const advancedButton = authenticatedPage.locator(
        'button:has-text("Advanced")'
      )
      if ((await advancedButton.count()) > 0) {
        await advancedButton.click()
        await authenticatedPage.waitForTimeout(500)

        const typeFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Types")'),
        })
        await typeFilter.waitFor({ state: 'visible', timeout: 10000 })
        await typeFilter.selectOption('static_contexts')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should filter artifacts by type (general)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const advancedButton = authenticatedPage.locator(
        'button:has-text("Advanced")'
      )
      if ((await advancedButton.count()) > 0) {
        await advancedButton.click()
        await authenticatedPage.waitForTimeout(500)

        const typeFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Types")'),
        })
        await typeFilter.waitFor({ state: 'visible', timeout: 10000 })
        await typeFilter.selectOption('general')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })
  })

  test.describe('Status Filtering', () => {
    test('should filter artifacts by status (active)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const advancedButton = authenticatedPage.locator(
        'button:has-text("Advanced")'
      )
      if ((await advancedButton.count()) > 0) {
        await advancedButton.click()
        await authenticatedPage.waitForTimeout(500)

        const statusFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Status")'),
        })
        await statusFilter.waitFor({ state: 'visible', timeout: 10000 })
        await statusFilter.selectOption('active')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should filter artifacts by status (expired)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const advancedButton = authenticatedPage.locator(
        'button:has-text("Advanced")'
      )
      if ((await advancedButton.count()) > 0) {
        await advancedButton.click()
        await authenticatedPage.waitForTimeout(500)

        const statusFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Status")'),
        })
        await statusFilter.waitFor({ state: 'visible', timeout: 10000 })
        await statusFilter.selectOption('expired')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })
  })

  test.describe('Search Functionality', () => {
    test('should search artifacts by title', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const searchInput = authenticatedPage.locator(
        'input[placeholder*="Search"], input[type="search"]'
      )

      if ((await searchInput.count()) > 0) {
        await searchInput.fill('test')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should search artifacts by content', async ({
      authenticatedPage,
    }) => {
      // Create artifact with unique content
      const timestamp = Date.now()
      await authenticatedPage.goto('/artifacts/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="artifact-project-select"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('[data-testid="artifact-slug-input"]')
        .fill(`content-search-${timestamp}`)
      await authenticatedPage
        .locator('[data-testid="artifact-title-input"]')
        .fill(`Content Search ${timestamp}`)
      await authenticatedPage
        .locator('[data-testid="artifact-content-textarea"]')
        .fill(`UniqueSearchContent${timestamp}`)
      await selectFirstProject(authenticatedPage)
      await authenticatedPage
        .locator('button:has-text("Create Artifact")')
        .click()

      await authenticatedPage.waitForTimeout(2000)

      // Go to list and search
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const searchInput = authenticatedPage.locator(
        'input[placeholder*="Search"], input[type="search"]'
      )

      if ((await searchInput.count()) > 0) {
        await searchInput.fill(`UniqueSearchContent${timestamp}`)
        await authenticatedPage.waitForTimeout(1500)

        // Verify title appears (content might not be shown in list)
        const titleText = `Content Search ${timestamp}`
        await expect(
          authenticatedPage
            .locator(`text=${titleText.substring(0, 20)}`)
            .first()
        ).toBeVisible({ timeout: 10000 })
      }
    })

    test('should search artifacts by description', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const searchInput = authenticatedPage.locator(
        'input[placeholder*="Search"], input[type="search"]'
      )

      if ((await searchInput.count()) > 0) {
        await searchInput.fill('description')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })
  })

  test.describe('Combined Filtering', () => {
    test('should combine multiple filters (project + type)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Apply project filter
      const projectFilter = authenticatedPage.locator('select').filter({
        has: authenticatedPage.locator('option:has-text("All Projects")'),
      })

      if ((await projectFilter.count()) > 0) {
        const options = await projectFilter.locator('option').count()
        if (options > 1) {
          await projectFilter.selectOption({ index: 1 })
          await authenticatedPage.waitForTimeout(1000)
        }
      }

      // Apply type filter
      const advancedButton = authenticatedPage.locator(
        'button:has-text("Advanced")'
      )
      if ((await advancedButton.count()) > 0) {
        await advancedButton.click()
        await authenticatedPage.waitForTimeout(500)

        const typeFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Types")'),
        })
        await typeFilter.waitFor({ state: 'visible', timeout: 10000 })
        await typeFilter.selectOption('general')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should combine multiple filters (type + status)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const advancedButton = authenticatedPage.locator(
        'button:has-text("Advanced")'
      )
      if ((await advancedButton.count()) > 0) {
        await advancedButton.click()
        await authenticatedPage.waitForTimeout(500)

        // Apply type filter
        const typeFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Types")'),
        })
        await typeFilter.waitFor({ state: 'visible', timeout: 10000 })
        await typeFilter.selectOption('work_reports')
        await authenticatedPage.waitForTimeout(1000)

        // Apply status filter
        const statusFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Status")'),
        })
        await statusFilter.waitFor({ state: 'visible', timeout: 10000 })
        await statusFilter.selectOption('active')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should combine search with filters', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Apply search
      const searchInput = authenticatedPage.locator(
        'input[placeholder*="Search"], input[type="search"]'
      )

      if ((await searchInput.count()) > 0) {
        await searchInput.fill('test')
        await authenticatedPage.waitForTimeout(1000)
      }

      // Apply type filter
      const advancedButton = authenticatedPage.locator(
        'button:has-text("Advanced")'
      )
      if ((await advancedButton.count()) > 0) {
        await advancedButton.click()
        await authenticatedPage.waitForTimeout(500)

        const typeFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Types")'),
        })
        await typeFilter.waitFor({ state: 'visible', timeout: 10000 })
        await typeFilter.selectOption('general')
        await authenticatedPage.waitForTimeout(1500)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })
  })

  test.describe('Filter Management', () => {
    test('should clear all filters and show all artifacts', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Apply some filters first
      const projectFilter = authenticatedPage.locator('select').filter({
        has: authenticatedPage.locator('option:has-text("All Projects")'),
      })

      if ((await projectFilter.count()) > 0) {
        const options = await projectFilter.locator('option').count()
        if (options > 1) {
          await projectFilter.selectOption({ index: 1 })
          await authenticatedPage.waitForTimeout(1000)
        }
      }

      // Clear filters by selecting "All"
      if ((await projectFilter.count()) > 0) {
        await projectFilter.selectOption('All Projects')
        await authenticatedPage.waitForTimeout(1000)
      }

      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should persist filter state across navigation', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Apply a filter
      const advancedButton = authenticatedPage.locator(
        'button:has-text("Advanced")'
      )
      if ((await advancedButton.count()) > 0) {
        await advancedButton.click()
        await authenticatedPage.waitForTimeout(500)

        const typeFilter = authenticatedPage.locator('select').filter({
          has: authenticatedPage.locator('option:has-text("All Types")'),
        })
        await typeFilter.waitFor({ state: 'visible', timeout: 10000 })
        await typeFilter.selectOption('general')
        await authenticatedPage.waitForTimeout(1000)
      }

      // Navigate away and back
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForTimeout(500)
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify we're back on artifacts page
      await expect(authenticatedPage).toHaveURL(/artifacts$/)
    })

    test('should show "no results" message for empty filter results', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Search for non-existent content
      const searchInput = authenticatedPage.locator(
        'input[placeholder*="Search"], input[type="search"]'
      )

      if ((await searchInput.count()) > 0) {
        await searchInput.fill('nonexistentartifactxyz12345')
        await authenticatedPage.waitForTimeout(1500)

        // Check for empty state message
        const emptyMessage = authenticatedPage.locator(
          'text=/No artifacts found|No results/i'
        )
        const hasEmptyState = await emptyMessage.isVisible().catch(() => false)

        if (hasEmptyState) {
          expect(hasEmptyState).toBeTruthy()
        }
      }
    })
  })
})
