import type { Locator, Page } from '@playwright/test'

import { expect, test, devLogin } from '../../fixtures/auth'
import { ADMIN_EMAIL } from '../admin/admin-emails'

/**
 * Collapsed-rail tooltips are anchored to the hovered nav item (#891).
 *
 * This is the only layer that can see the regression. Radix renders
 * `TooltipTrigger asChild` as the popper's ANCHOR, so slotting a
 * `display: contents` wrapper gives floating-ui a 0x0 rect at (0,0) and every
 * tooltip lands on the logo in the top-left corner — with no error, no warning
 * and a fully green unit suite (jsdom has no layout engine, so it cannot
 * measure a box at all). The assertions below are geometric on purpose.
 */

/** Vertical centring tolerance between the tooltip and the item it labels. */
const CENTRE_TOLERANCE = 8

interface Box {
  x: number
  y: number
  width: number
  height: number
}

/**
 * Measuring an entering tooltip is a race: `TooltipContent` opens with
 * `slide-in-from-left-2` + `zoom-in-95`, so an immediate `boundingBox()` reads
 * a mid-flight position — on CI this measured x = 49.8 / 52.5 / 54.1 across
 * three attempts for a bubble whose resting x is 64. Collapse the durations to
 * zero instead of sleeping. `animation-duration: 0s` (not `animation: none`)
 * because Radix's `Presence` unmounts on `animationend`, which a zero-duration
 * animation still fires.
 */
async function neutraliseAnimations(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const inject = () => {
      const style = document.createElement('style')
      style.textContent =
        '*, *::before, *::after { animation-duration: 0s !important; animation-delay: 0s !important; transition-duration: 0s !important; }'
      document.head.appendChild(style)
    }
    if (document.head) inject()
    else document.addEventListener('DOMContentLoaded', inject)
  })
}

async function boxOf(locator: Locator, what: string): Promise<Box> {
  const box = await locator.boundingBox()
  expect(box, `${what} has no layout box`).not.toBeNull()
  return box as Box
}

/**
 * Waits for the bubble to sit past the hovered item's right edge, then
 * measures it.
 *
 * The threshold is derived from the item, NOT a constant: the rail is 60px but
 * its `nav` has `px-2`, so the `<a>` ends at x = 52 and a correctly placed
 * `side="right"` bubble starts at 52 + `sideOffset` (4) = 56. A literal
 * `x >= 60` is unreachable even with the fix working — the first CI run failed
 * exactly there, and #891's own acceptance criterion carries the same wrong
 * number. Against the bug the anchor is the viewport origin, so `tooltip.x` is
 * ~4 and this still discriminates cleanly.
 *
 * Polling rather than a single read because the popper positions itself in a
 * post-layout effect.
 */
async function settledTooltipBox(
  tooltip: Locator,
  item: Box,
  label: string
): Promise<Box> {
  const itemRight = item.x + item.width
  await expect
    .poll(async () => (await tooltip.boundingBox())?.x ?? -1, {
      message: `${label}: tooltip never moved past the item's right edge (${itemRight}) — it is anchored on the viewport origin, not on the hovered item`,
    })
    .toBeGreaterThanOrEqual(itemRight)
  return boxOf(tooltip, `${label} tooltip`)
}

function assertCentredOnItem(tooltip: Box, item: Box, label: string): void {
  const tooltipCentreY = tooltip.y + tooltip.height / 2
  const itemCentreY = item.y + item.height / 2
  expect(
    Math.abs(tooltipCentreY - itemCentreY),
    `${label}: tooltip centre y=${tooltipCentreY} vs item centre y=${itemCentreY}`
  ).toBeLessThanOrEqual(CENTRE_TOLERANCE)
}

test.describe('Collapsed sidebar tooltips (#891)', () => {
  test('the main rail labels the hovered item, not the logo', async ({
    page,
  }) => {
    await neutraliseAnimations(page)
    await devLogin(page)

    const sidebar = page.getByTestId('app-sidebar')
    await expect(sidebar).toBeVisible()

    // Fold the sidebar into the 60px icon rail — the form the tooltips exist
    // for, since the labels are hidden there.
    await page.getByTestId('nav-toggle').click()
    await expect(sidebar).toHaveAttribute('data-state', 'collapsed')

    // Two items far apart vertically: a mis-anchored tooltip parks BOTH in the
    // same top-left spot, so a single item cannot distinguish the two states.
    // Located by href — in the collapsed rail the label span is display:none,
    // so these links have no accessible name to query by.
    const tooltipTops: number[] = []
    for (const item of [
      { href: '/prompts', label: 'Prompts' },
      { href: '/settings', label: 'Settings' },
    ]) {
      const link = sidebar.locator(`a[href="${item.href}"]`)
      await expect(link).toBeVisible()
      await link.hover()

      const tooltip = page.getByRole('tooltip').filter({ hasText: item.label })
      await expect(tooltip).toBeVisible()

      const linkBox = await boxOf(link, `${item.label} link`)
      const tooltipBox = await settledTooltipBox(tooltip, linkBox, item.label)
      assertCentredOnItem(tooltipBox, linkBox, item.label)
      tooltipTops.push(tooltipBox.y)

      // Move away so the next hover re-opens rather than reusing this bubble.
      await page.mouse.move(960, 540)
      await expect(tooltip).toBeHidden()
    }

    // The regression's signature: every tooltip in the SAME place regardless of
    // which item is hovered.
    const [first = 0, second = 0] = tooltipTops
    expect(
      Math.abs(first - second),
      'both tooltips rendered at the same height — they are not tracking the hovered item'
    ).toBeGreaterThan(CENTRE_TOLERANCE)
  })

  test('the admin rail labels the hovered item below the lg breakpoint', async ({
    page,
  }) => {
    // The admin rail is 60px only below `lg` (1024px); at `lg+` it is expanded
    // and the tooltip is deliberately suppressed.
    await page.setViewportSize({ width: 900, height: 900 })
    await neutraliseAnimations(page)
    await devLogin(page, ADMIN_EMAIL, 'Admin E2E')

    await page.goto('/admin')
    const link = page.locator(
      'nav[aria-label="Admin sections"] a[href="/admin/teams"]'
    )
    await expect(link).toBeVisible()
    await link.hover()

    const tooltip = page.getByRole('tooltip').filter({ hasText: 'Teams' })
    await expect(tooltip).toBeVisible()

    const linkBox = await boxOf(link, 'admin Teams link')
    assertCentredOnItem(
      await settledTooltipBox(tooltip, linkBox, 'admin Teams'),
      linkBox,
      'admin Teams'
    )
  })
})
