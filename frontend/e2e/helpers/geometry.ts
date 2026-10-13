import { expect, type Locator } from '@playwright/test'

/**
 * Reads an element's geometry in ONE round trip to the page, and retries until
 * the element is visible and the read succeeds.
 *
 * Two separate calls (`toBeVisible`, then `boundingBox`) can straddle a route
 * change: the first resolves a node of the page being left, the second finds
 * it unmounted and returns nothing (#1423). Everything a measurement needs is
 * therefore read inside a single `evaluate`, from the one node it was handed,
 * and a node that goes away mid-read just means another attempt.
 *
 * `read` runs in the browser: it can use the DOM and nothing from this file.
 * Coordinates from `getBoundingClientRect()` are viewport-relative, the same
 * frame `locator.boundingBox()` reports in.
 */
export async function measure<T>(
  locator: Locator,
  read: (element: Element) => T
): Promise<T> {
  let result: T | undefined
  await expect(async () => {
    await expect(locator).toBeVisible()
    result = await locator.evaluate(read)
  }).toPass({ timeout: 10000 })
  return result as T
}
