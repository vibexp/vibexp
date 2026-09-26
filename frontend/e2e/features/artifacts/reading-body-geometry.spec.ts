import type { Locator, Page } from '@playwright/test'

import { test, expect } from '../../fixtures/auth'
import { selectFirstProject } from '../../helpers/artifacts'

/**
 * Feature Test: reading-page body geometry (#1176)
 *
 * The Raw view must sit in the same column as the title and the rendered
 * prose, so toggling Rendered ↔ Raw moves nothing. It used to bleed past the
 * column by `--reading-bleed` on each side (up to 12rem at wide windows),
 * while wide blocks inside the RENDERED body keep that bleed on purpose.
 *
 * This is geometry, so only a real layout engine can see it: jsdom returns
 * 0×0 for every box, and the unit suite cannot catch a regression here.
 */

const DETAIL_URL = /artifacts\/[^/]+\/[^/]+/

// Word-separated, so the raw `whitespace-pre-wrap` view wraps it, while the
// rendered code block keeps it on one line and wants the whole bleed.
const LONG_LINE = Array.from(
  { length: 40 },
  (_, i) => `token${String(i)}`
).join(' ')

const CONTENT = `A paragraph of prose that stays in the reading column.

\`\`\`bash
echo ${LONG_LINE}
\`\`\`
`

async function createArtifact(page: Page): Promise<void> {
  await page.goto('/artifacts/new')
  await expect(page).toHaveURL(/artifacts\/new/)

  const stamp = Date.now().toString(36)
  await page.waitForSelector('[data-testid="artifact-project-select"]', {
    timeout: 10000,
  })
  await page
    .locator('[data-testid="artifact-slug-input"]')
    .fill(`geometry-${stamp}`)
  await page
    .locator('[data-testid="artifact-title-input"]')
    .fill(`Geometry ${stamp}`)
  await page.locator('[data-testid="artifact-content-textarea"]').fill(CONTENT)
  await selectFirstProject(page)
  await page.locator('button:has-text("Create Artifact")').click()

  await expect(page).toHaveURL(DETAIL_URL, { timeout: 10000 })
}

async function box(locator: Locator): Promise<{ x: number; width: number }> {
  await expect(locator).toBeVisible({ timeout: 10000 })
  const b = await locator.boundingBox()
  if (!b) throw new Error('element has no bounding box')
  return { x: b.x, width: b.width }
}

// `scrollWidth` can read BELOW `clientWidth` (a reserved scrollbar gutter), so
// "no horizontal scroll" is `<=`, not `===`.
async function expectNoPageOverflow(page: Page): Promise<void> {
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          document.documentElement.scrollWidth -
          document.documentElement.clientWidth
      )
    )
    .toBeLessThanOrEqual(0)
}

test.describe('Reading body geometry', () => {
  test('Raw sits in the reading column; rendered wide blocks still bleed', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1080 })
    await createArtifact(page)

    const title = page.getByTestId('reading-page').locator('article header h1')
    const bodyView = page.getByRole('tablist', { name: 'Body view' })

    await bodyView.getByRole('tab', { name: 'Rendered' }).click()
    const column = await box(title)
    const prose = await box(page.locator('.reading-body > p').first())
    const codeBlock = await box(page.locator('.reading-body > pre').first())

    // Rendered prose is pushed back into the column…
    expect(Math.abs(prose.x - column.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(prose.width - column.width)).toBeLessThanOrEqual(1)
    // …while a wide block still takes the bleed on both sides.
    expect(codeBlock.x).toBeLessThan(column.x - 1)
    expect(codeBlock.width).toBeGreaterThan(column.width + 2)

    await bodyView.getByRole('tab', { name: 'Raw' }).click()
    const raw = await box(page.getByTestId('resource-body-raw'))

    // Same left edge and width as the column, so the toggle shifts nothing.
    expect(Math.abs(raw.x - column.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(raw.width - column.width)).toBeLessThanOrEqual(1)
    expect(Math.abs(raw.x - prose.x)).toBeLessThanOrEqual(1)

    for (const width of [1920, 1280, 390]) {
      await page.setViewportSize({ width, height: 1080 })
      await expect(page.getByTestId('resource-body-raw')).toBeVisible()
      await expectNoPageOverflow(page)
    }
  })
})
