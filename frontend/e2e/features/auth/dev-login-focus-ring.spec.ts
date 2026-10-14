import { expect, test } from '@playwright/test'

/**
 * Feature Test: the dev login fields' focus ring is not clipped (#1498)
 *
 * The fields take the design-system input focus style, a ring drawn outside
 * the field's box. A clipping container around the form cut that ring into a
 * stray line above and below the field. This walks up from each field to the
 * sign-in card and fails if any ancestor in between clips content that close
 * to the field.
 *
 * Geometry and computed style, so only a real layout engine can see it.
 */

/** How far outside the field the ring is drawn (`ring-2` + `ring-offset-2`). */
const RING_REACH = 4

for (const width of [1280, 390]) {
  test(`the email and name focus rings are not clipped at ${String(width)}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/login?dev_login=true')
    await page.getByRole('button', { name: /development login/i }).click()

    for (const id of ['dev-email', 'dev-name']) {
      const field = page.locator(`#${id}`)
      await field.focus()
      await expect(field).toBeFocused()

      const clipping = await field.evaluate((el, reach) => {
        const box = el.getBoundingClientRect()
        const found: string[] = []
        let node = el.parentElement
        // Up to the card that frames the form: that one is padded, and is
        // where clipping would be legitimate.
        while (node && !node.classList.contains('bg-card')) {
          const style = getComputedStyle(node)
          const clips = [style.overflowX, style.overflowY].some(
            value => value !== 'visible'
          )
          if (clips) {
            const r = node.getBoundingClientRect()
            const tooClose =
              box.top - r.top < reach ||
              r.bottom - box.bottom < reach ||
              box.left - r.left < reach ||
              r.right - box.right < reach
            if (tooClose) {
              found.push(`${node.tagName.toLowerCase()}.${node.className}`)
            }
          }
          node = node.parentElement
        }
        return found
      }, RING_REACH)

      expect(clipping, `${id}: containers clipping its focus ring`).toEqual([])
    }
  })
}
