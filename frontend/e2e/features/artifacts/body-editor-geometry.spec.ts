import type { Locator, Page } from '@playwright/test'

import { test, expect } from '../../fixtures/auth'
import { createArtifactWithContent } from '../../helpers/artifacts'

/**
 * Feature Test: the body editor matches the reading body (#1178)
 *
 * View → Edit is the same document, so the Rendered | Raw switch and the body
 * box must not move: same x / y / width on `/artifacts/:p/:s` (Raw) and on
 * `/artifacts/:p/:s/edit`, and the Raw block and the textarea share one
 * surface (14px type, same padding and background).
 *
 * `y` is measured from the bottom of the article header. The two headers are
 * not identical yet (their alignment is a sibling issue of #1178, out of its
 * scope), and an absolute `y` would make this spec about the header rather
 * than about the body.
 *
 * Geometry and computed style, so only a real layout engine can see it: jsdom
 * returns 0×0 for every box and computes no stylesheet.
 */

const CONTENT = `A paragraph of prose.

- one
- two
`

interface Box {
  x: number
  y: number
  width: number
}

async function box(locator: Locator): Promise<Box> {
  await expect(locator).toBeVisible({ timeout: 10000 })
  const b = await locator.boundingBox()
  if (!b) throw new Error('element has no bounding box')
  const header = await locator
    .page()
    .getByTestId('reading-page')
    .locator('article header')
    .boundingBox()
  if (!header) throw new Error('article header has no bounding box')
  return { x: b.x, y: b.y - (header.y + header.height), width: b.width }
}

function expectSameBox(actual: Box, expected: Box): void {
  expect(Math.abs(actual.x - expected.x)).toBeLessThanOrEqual(1)
  expect(Math.abs(actual.y - expected.y)).toBeLessThanOrEqual(1)
  expect(Math.abs(actual.width - expected.width)).toBeLessThanOrEqual(1)
}

async function surface(locator: Locator) {
  return locator.evaluate(el => {
    const s = getComputedStyle(el)
    return {
      fontSize: s.fontSize,
      padding: s.padding,
      background: s.backgroundColor,
    }
  })
}

function bodyView(page: Page): Locator {
  return page.getByRole('tablist', { name: 'Body view' })
}

test.describe('Body editor geometry', () => {
  test('the switch and the body box do not move from view to edit', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1080 })
    await createArtifactWithContent(page, 'Editor', 'editor-geom', CONTENT)

    await bodyView(page).getByRole('tab', { name: 'Raw' }).click()
    const raw = page.getByTestId('resource-body-raw')
    const viewToggle = await box(bodyView(page))
    const viewBody = await box(raw)
    const viewSurface = await surface(raw)

    await page.getByTestId('edit-artifact-button').click()
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+\/edit/)

    const textarea = page.getByTestId('artifact-content-textarea')
    await expect(textarea).toHaveValue(CONTENT.trim())
    expectSameBox(await box(bodyView(page)), viewToggle)
    expectSameBox(await box(textarea), viewBody)

    const editSurface = await surface(textarea)
    expect(viewSurface.fontSize).toBe('14px')
    expect(editSurface).toEqual(viewSurface)

    // The page scrolled nowhere between the two measurements.
    expect(await page.evaluate(() => window.scrollY)).toBe(0)
  })

  test('edit opens on Raw, and its choice carries back to the reading page', async ({
    authenticatedPage: page,
  }) => {
    await createArtifactWithContent(page, 'Editor', 'editor-pref', CONTENT)
    const viewUrl = page.url()

    // The reader prefers Rendered…
    await bodyView(page).getByRole('tab', { name: 'Rendered' }).click()

    // …yet the editor always opens on the source.
    await page.getByTestId('edit-artifact-button').click()
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+\/edit/)
    await expect(
      bodyView(page).getByRole('tab', { name: 'Raw' })
    ).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByTestId('artifact-content-textarea')).toBeVisible()

    // Switching to Raw then Rendered in edit writes the shared preference.
    await bodyView(page).getByRole('tab', { name: 'Raw' }).click()
    await page.goto(viewUrl)
    await expect(page.getByTestId('resource-body-raw')).toBeVisible()

    await page.getByTestId('edit-artifact-button').click()
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+\/edit/)
    await bodyView(page).getByRole('tab', { name: 'Rendered' }).click()
    await expect(page.locator('.reading-body').first()).toBeVisible()

    await page.goto(viewUrl)
    await expect(
      bodyView(page).getByRole('tab', { name: 'Rendered' })
    ).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByTestId('resource-body-raw')).toHaveCount(0)
  })
})
