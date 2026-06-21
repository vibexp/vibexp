import { test, expect } from '../fixtures/auth'

/**
 * Journey 4: Prompt Gallery to Custom Prompt
 *
 * Based on manual browser testing on 2026-02-07.
 *
 * Actual workflow discovered:
 * 1. Gallery shows 5 category CARDS (not simple links)
 * 2. Each card is clickable and shows category name + prompt count
 * 3. Clicking card navigates to category detail page
 * 4. Category page shows prompts in that category
 * 5. "Use This Prompt" button redirects to /prompts/new with pre-filled content
 * 6. **CRITICAL**: Project is required before saving
 * 7. Save redirects to /prompts list
 */
test.describe('Journey 4: Prompt Gallery to Custom Prompt', () => {
  test.describe('Gallery Navigation', () => {
    test('should display Prompt Gallery page', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')

      // Should see gallery heading
      await expect(
        authenticatedPage.getByRole('heading', { name: /prompt gallery/i })
      ).toBeVisible()

      // Should see subtitle
      await expect(
        authenticatedPage.getByText(/explore pre-defined reusable prompts/i)
      ).toBeVisible()
    })

    test('should display 5 category cards', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForTimeout(1000)

      // Should see all 5 categories
      await expect(
        authenticatedPage.getByText('Customer Support')
      ).toBeVisible()
      await expect(authenticatedPage.getByText('Data Analysis')).toBeVisible()
      await expect(authenticatedPage.getByText('Engineering')).toBeVisible()
      await expect(authenticatedPage.getByText('Marketing')).toBeVisible()
      await expect(
        authenticatedPage.getByText('Product Management')
      ).toBeVisible()
    })
  })

  test.describe('Category Navigation', () => {
    test('should navigate to Data Analysis category', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForTimeout(1000)

      // Click Data Analysis card
      await authenticatedPage.getByText('Data Analysis').click()
      await authenticatedPage.waitForTimeout(1000)

      // Should navigate to category page (URLs use title case with spaces)
      await expect(authenticatedPage).toHaveURL(
        /\/prompt-gallery\/Data%20Analysis/
      )

      // Should see category heading
      await expect(
        authenticatedPage.getByRole('heading', { name: /data analysis/i })
      ).toBeVisible()
    })

    test('should display prompts in Data Analysis category', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForTimeout(1000)

      // Navigate to Data Analysis
      await authenticatedPage.getByText('Data Analysis').click()
      await authenticatedPage.waitForTimeout(1000)

      // Should see at least one prompt (SQL Query Optimization)
      await expect(
        authenticatedPage.getByText('SQL Query Optimization')
      ).toBeVisible()
    })
  })

  test.describe('Prompt Detail and Cloning', () => {
    test('should view SQL Query Optimization prompt details', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForTimeout(1000)

      // Navigate to Data Analysis → SQL Query Optimization
      await authenticatedPage.getByText('Data Analysis').click()
      await authenticatedPage.waitForTimeout(1000)
      await authenticatedPage.getByText('SQL Query Optimization').click()
      await authenticatedPage.waitForTimeout(1000)

      // Should see prompt heading
      await expect(
        authenticatedPage.getByRole('heading', {
          name: /sql query optimization/i,
        })
      ).toBeVisible()

      // Should see description
      await expect(
        authenticatedPage.getByText(/analyze and optimize sql queries/i)
      ).toBeVisible()

      // Should see "Use This Prompt" button
      await expect(
        authenticatedPage.getByRole('button', { name: /use this prompt/i })
      ).toBeVisible()
    })

    test('should clone prompt with pre-filled content', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForTimeout(1000)

      // Navigate to prompt
      await authenticatedPage.getByText('Data Analysis').click()
      await authenticatedPage.waitForTimeout(1000)
      await authenticatedPage.getByText('SQL Query Optimization').click()
      await authenticatedPage.waitForTimeout(1000)

      // Click Use This Prompt
      await authenticatedPage
        .getByRole('button', { name: /use this prompt/i })
        .click()
      await authenticatedPage.waitForTimeout(2000)

      // Should redirect to /prompts/new
      await expect(authenticatedPage).toHaveURL('/prompts/new')

      // Should have pre-filled name with "Based on:" prefix
      const nameField = authenticatedPage.getByPlaceholder(/enter prompt name/i)
      const nameValue = await nameField.inputValue()
      expect(nameValue).toContain('Based on:')
      expect(nameValue).toContain('SQL Query Optimization')
    })

    test('should preserve placeholders when cloning', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForTimeout(1000)

      // Navigate to SQL Query Optimization
      await authenticatedPage.getByText('Data Analysis').click()
      await authenticatedPage.waitForTimeout(1000)
      await authenticatedPage.getByText('SQL Query Optimization').click()
      await authenticatedPage.waitForTimeout(1000)

      // Click Use This Prompt
      await authenticatedPage
        .getByRole('button', { name: /use this prompt/i })
        .click()
      await authenticatedPage.waitForTimeout(2000)

      // Verify placeholders in content
      const contentField = authenticatedPage.locator('textarea').first()
      const content = await contentField.inputValue()

      expect(content).toContain('{{query}}')
      expect(content).toContain('{{database_type}}')
      expect(content).toContain('{{schema}}')
    })
  })

  test.describe('Complete Workflow with Project', () => {
    test('should complete full workflow: clone → customize → save', async ({
      authenticatedPage,
    }) => {
      // STEP 1: Navigate to gallery and clone prompt
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForTimeout(1000)

      await authenticatedPage.getByText('Data Analysis').click()
      await authenticatedPage.waitForTimeout(1000)

      await authenticatedPage.getByText('SQL Query Optimization').click()
      await authenticatedPage.waitForTimeout(1000)

      await authenticatedPage
        .getByRole('button', { name: /use this prompt/i })
        .click()
      await authenticatedPage.waitForTimeout(2000)

      // STEP 2: Customize prompt
      const nameField = authenticatedPage.getByPlaceholder(/enter prompt name/i)
      await nameField.clear()
      await nameField.fill('My Automated Test Prompt')

      // STEP 3: Select project (E2E Test Project should exist from setup)
      const projectSelect = authenticatedPage.locator('select').first()

      // Wait for project dropdown to be ready
      await authenticatedPage.waitForTimeout(1000)

      // Select the first available project (or specific project if it exists)
      const options = await projectSelect.locator('option').count()
      if (options > 1) {
        // Select first non-empty option
        await projectSelect.selectOption({ index: 1 })
      }

      // STEP 4: Save
      await authenticatedPage
        .getByRole('button', { name: /save as draft/i })
        .click()
      await authenticatedPage.waitForTimeout(3000)

      // STEP 5: Verify prompt was saved (redirects to detail page)
      await expect(authenticatedPage).toHaveURL(
        /\/prompts\/my-automated-test-prompt/
      )

      // Should see prompt name as heading
      await expect(
        authenticatedPage.getByRole('heading', {
          name: /my automated test prompt/i,
        })
      ).toBeVisible()

      // Navigate to My Prompts list to verify it's there
      await authenticatedPage.goto('/prompts')
      await expect(
        authenticatedPage.getByText('My Automated Test Prompt')
      ).toBeVisible()

      // Should show Draft status (there are multiple "Draft" elements, be more specific)
      await expect(
        authenticatedPage.locator('span').filter({ hasText: 'Draft' }).first()
      ).toBeVisible()
    })
  })

  test.describe('Other Categories', () => {
    test('should navigate to Customer Support category', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForTimeout(1000)

      await authenticatedPage.getByText('Customer Support').click()
      await authenticatedPage.waitForTimeout(1000)

      await expect(authenticatedPage).toHaveURL(
        /\/prompt-gallery\/Customer%20Support/
      )
    })

    test('should navigate to Engineering category', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForTimeout(1000)

      await authenticatedPage.getByText('Engineering').click()
      await authenticatedPage.waitForTimeout(1000)

      await expect(authenticatedPage).toHaveURL(/\/prompt-gallery\/Engineering/)
    })
  })
})
