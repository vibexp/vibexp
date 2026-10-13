import type { Locator, Page } from '@playwright/test'

import { test, expect } from '../../fixtures/auth'
import { createArtifactWithContent } from '../../helpers/artifacts'

/**
 * Feature Test: the body editor matches the reading body (#1178)
 *
 * View → Edit is the same document, so the body box must not move: same
 * x / y / width on `/artifacts/:p/:s` (Raw) and on `/artifacts/:p/:s/edit`,
 * and the Raw block and the textarea set their text the same way (14px type,
 * same padding). The fill is not shared: the editor is an input on the page
 * background, the Raw block a muted read-only surface.
 *
 * The body's `y` is measured from the bottom of the article header. The two
 * headers are not the same height (the edit page adds the description input),
 * and an absolute `y` would make this spec about the header rather than about
 * the body.
 *
 * The Rendered | Raw switch is drawn in the header on both pages — in the
 * reading page's toolbar, and at the end of the edit page's meta row — so it
 * takes no row above the body on either. It is checked for that: inside the
 * header, on the same line under the title (`y` from the header's TOP). Its
 * `x` is not compared: the reading toolbar has buttons after it (More, focus
 * mode) that an edit page does not have.
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

/** The switch's box, with `y` from the TOP of the article header. */
async function switchInHeader(page: Page): Promise<Box & { inside: boolean }> {
  const toggle = bodyView(page)
  await expect(toggle).toBeVisible({ timeout: 10000 })
  const header = page.getByTestId('reading-page').locator('article header')
  const b = await toggle.boundingBox()
  const h = await header.boundingBox()
  if (!b || !h) throw new Error('switch or article header has no bounding box')
  return {
    x: b.x,
    y: b.y - h.y,
    width: b.width,
    inside:
      b.x >= h.x &&
      b.x + b.width <= h.x + h.width + 1 &&
      b.y >= h.y &&
      b.y + b.height <= h.y + h.height + 1,
  }
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
    const viewToggle = await switchInHeader(page)
    const viewBody = await box(raw)
    const viewSurface = await surface(raw)

    await page.getByTestId('edit-artifact-button').click()
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+\/edit/)

    const textarea = page.getByTestId('artifact-content-textarea')
    await expect(textarea).toHaveValue(CONTENT.trim())
    expectSameBox(await box(textarea), viewBody)

    // The switch is in the header on both pages, on the same line.
    const editToggle = await switchInHeader(page)
    expect(viewToggle.inside).toBe(true)
    expect(editToggle.inside).toBe(true)
    expect(Math.abs(editToggle.y - viewToggle.y)).toBeLessThanOrEqual(1)
    expect(Math.abs(editToggle.width - viewToggle.width)).toBeLessThanOrEqual(1)

    // Same type and padding, so the text itself does not shift. The fill
    // differs on purpose: the Raw block is a muted read-only surface, and the
    // editor is an input on the page background (a grey fill read as a
    // disabled field).
    const editSurface = await surface(textarea)
    expect(viewSurface.fontSize).toBe('14px')
    expect(editSurface.fontSize).toBe(viewSurface.fontSize)
    expect(editSurface.padding).toBe(viewSurface.padding)
    const pageBackground = await page.evaluate(() => {
      const probe = document.createElement('div')
      probe.style.backgroundColor = 'var(--background)'
      document.querySelector('.v2-root')?.append(probe)
      const color = getComputedStyle(probe).backgroundColor
      probe.remove()
      return color
    })
    expect(editSurface.background).toBe(pageBackground)
    expect(viewSurface.background).not.toBe(pageBackground)

    // The page scrolled nowhere between the two measurements.
    expect(await page.evaluate(() => window.scrollY)).toBe(0)
  })

  test('edit opens on Raw, and its choice never reaches the reading page', async ({
    authenticatedPage: page,
  }) => {
    await createArtifactWithContent(page, 'Editor', 'editor-pref', CONTENT)
    const viewUrl = page.url()

    // A reading page always opens on Rendered…
    await expect(
      bodyView(page).getByRole('tab', { name: 'Rendered' })
    ).toHaveAttribute('aria-selected', 'true')

    // …and the editor always opens on the source.
    await page.getByTestId('edit-artifact-button').click()
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+\/edit/)
    await expect(
      bodyView(page).getByRole('tab', { name: 'Raw' })
    ).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByTestId('artifact-content-textarea')).toBeVisible()

    // The editor's Rendered pane is the reading body…
    await bodyView(page).getByRole('tab', { name: 'Rendered' }).click()
    await expect(page.locator('.reading-body').first()).toBeVisible()

    // …but whatever is chosen in edit stays in edit: the reading page opens
    // on Rendered regardless, and nothing is remembered.
    await bodyView(page).getByRole('tab', { name: 'Raw' }).click()
    await page.goto(viewUrl)
    await expect(
      bodyView(page).getByRole('tab', { name: 'Rendered' })
    ).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByTestId('resource-body-raw')).toHaveCount(0)
  })
})
