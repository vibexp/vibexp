import type { Locator, Page } from '@playwright/test'

import { test, expect } from '../../fixtures/auth'
import { createArtifactWithContent } from '../../helpers/artifacts'

/**
 * Feature Test: the edit page's header is the reading page's header (#1179)
 *
 * View → Edit keeps the header on ONE LEFT EDGE: the title, the badge row and
 * the description start at the same x on `/artifacts/:p/:s` and on
 * `/artifacts/:p/:s/edit`, where the title and description are now inputs.
 *
 * It used to pin y as well. The reading page's title is now larger than the
 * edit page's title input and carries a toolbar beside it, so the rows below
 * it no longer sit at the same height on the two pages; only the horizontal
 * alignment is still a shared contract.
 *
 * An inline input keeps its TEXT on the view's x by pairing a negative margin
 * with matching padding, so for an input the text origin is its box plus its
 * padding — that is what is compared, not the (deliberately wider) box.
 *
 * Geometry, so only a real layout engine can see it: jsdom returns 0×0 for
 * every box.
 */

const DESCRIPTION = 'A short lead describing the artifact.'

interface Point {
  x: number
  y: number
}

async function origin(locator: Locator): Promise<Point> {
  await expect(locator).toBeVisible({ timeout: 10000 })
  const b = await locator.boundingBox()
  if (!b) throw new Error('element has no bounding box')
  const { left, top } = await locator.evaluate(el => {
    const s = getComputedStyle(el)
    return {
      left: parseFloat(s.paddingLeft) + parseFloat(s.borderLeftWidth),
      top: parseFloat(s.paddingTop) + parseFloat(s.borderTopWidth),
    }
  })
  return { x: b.x + left, y: b.y + top }
}

function expectSameLeftEdge(actual: Point, expected: Point): void {
  expect(Math.abs(actual.x - expected.x)).toBeLessThanOrEqual(1)
}

/** The article: the title bar, the meta row and the lead all live in it. */
function header(page: Page): Locator {
  return page.getByTestId('reading-page').locator('article')
}

test.describe('Edit header geometry', () => {
  test('the title, badge row and description keep one left edge from view to edit', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1080 })
    await createArtifactWithContent(page, 'Header', 'header-geom', 'Body')
    const viewUrl = page.url()

    // Give it a description through the header input itself.
    await page.getByTestId('edit-artifact-button').click()
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+\/edit/)
    await page.getByTestId('artifact-description-input').fill(DESCRIPTION)
    await page.getByRole('button', { name: 'Save changes' }).first().click()
    await expect(page).toHaveURL(viewUrl, { timeout: 10000 })

    const viewTitle = await origin(
      header(page).getByRole('heading', { level: 1 })
    )
    const viewBadges = await origin(
      header(page).getByTestId('resource-header-meta').locator('> div').first()
    )
    const viewLead = await origin(header(page).getByText(DESCRIPTION))

    await page.getByTestId('edit-artifact-button').click()
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+\/edit/)
    const description = page.getByTestId('artifact-description-input')
    await expect(description).toHaveValue(DESCRIPTION)

    expectSameLeftEdge(
      await origin(page.getByTestId('artifact-title-input')),
      viewTitle
    )
    expectSameLeftEdge(
      await origin(
        header(page)
          .getByTestId('resource-header-meta')
          .locator('> div')
          .first()
      ),
      viewBadges
    )
    expectSameLeftEdge(await origin(description), viewLead)

    // The details column no longer carries the two fields.
    const column = page.getByTestId('details-column')
    await expect(column.getByTestId('artifact-title-input')).toHaveCount(0)
    await expect(column.getByTestId('artifact-description-input')).toHaveCount(
      0
    )
  })

  test('a long title wraps on a phone instead of scrolling sideways', async ({
    authenticatedPage: page,
  }) => {
    await createArtifactWithContent(page, 'Header', 'header-wrap', 'Body')
    await page.getByTestId('edit-artifact-button').click()
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+\/edit/)
    await page.setViewportSize({ width: 390, height: 844 })

    const title = page.getByTestId('artifact-title-input')
    await title.fill(
      'A deliberately long artifact title that cannot possibly fit on one line of a phone'
    )
    const box = await title.boundingBox()
    if (!box) throw new Error('title has no bounding box')
    // More than one 32px line tall, and never wider than the viewport.
    expect(box.height).toBeGreaterThan(40)
    expect(box.x + box.width).toBeLessThanOrEqual(390)
  })
})
