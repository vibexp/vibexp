import type { Locator, Page } from '@playwright/test'

import { test, expect } from '../../fixtures/auth'
import { createArtifactWithContent } from '../../helpers/artifacts'

/**
 * Feature Test: reading-page body geometry (#1176)
 *
 * One constant left edge: the title, the rendered prose, wide blocks and the
 * Raw view all start at the same x, so toggling Rendered ↔ Raw moves nothing
 * sideways. Prose stops at the prose measure; a wide block (code, a table)
 * extends to the RIGHT of it, up to the article's width, and never out on
 * both sides as it used to.
 *
 * This is geometry, so only a real layout engine can see it: jsdom returns
 * 0×0 for every box, and the unit suite cannot catch a regression here.
 */

// Word-separated, so the raw `whitespace-pre-wrap` view wraps it, while the
// rendered code block keeps it on one line and wants all the width it can get.
const LONG_LINE = Array.from(
  { length: 40 },
  (_, i) => `token${String(i)}`
).join(' ')

const CONTENT = `A paragraph of prose that stays in the reading column.

\`\`\`bash
echo ${LONG_LINE}
\`\`\`
`

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
  test('one left edge: prose stops at its measure, wide blocks extend right', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1080 })
    await createArtifactWithContent(page, 'Geometry', 'geometry', CONTENT)

    const title = page.getByTestId('reading-page').locator('article header h1')
    const bodyView = page.getByRole('tablist', { name: 'Body view' })

    await bodyView.getByRole('tab', { name: 'Rendered' }).click()
    const column = await box(title)
    const prose = await box(page.locator('.reading-body > p').first())
    const codeBlock = await box(
      page.locator('.reading-body > .code-block').first()
    )

    // Rendered prose starts at the column's left edge and stops short of its
    // right one: at this width the article is wider than the prose measure.
    expect(Math.abs(prose.x - column.x)).toBeLessThanOrEqual(1)
    expect(prose.width).toBeLessThan(column.width - 2)
    // A wide block starts at the SAME left edge and extends to the right of
    // the prose, never past the article.
    expect(Math.abs(codeBlock.x - column.x)).toBeLessThanOrEqual(1)
    expect(codeBlock.width).toBeGreaterThan(prose.width + 2)
    expect(codeBlock.width).toBeLessThanOrEqual(column.width + 1)

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
