import { expect, type Page } from '@playwright/test'

/**
 * Opens the reading toolbar's "More" menu, where every action that is not a
 * labelled toolbar button lives (Lock, Unlock, Version history, Delete).
 *
 * A menu item only exists while the menu is open, so open it before asserting
 * that an action is — or is NOT — offered: `toHaveCount(0)` against a closed
 * menu passes for the wrong reason. Returns `false` when the page has no menu
 * at all, which itself means it offers no such action. Idempotent: an already
 * open menu is left open.
 *
 * Returns only once the menu is really open. Choosing an item closes the menu,
 * but its element stays in the page for the length of its exit animation, and
 * a click on the trigger during that window opens a menu that the closing one
 * dismisses again a few milliseconds later. A person cannot click that fast;
 * a test straight after a quick mutation can. So a menu that is on its way
 * out is waited for before the trigger is clicked.
 */
export async function openMoreActions(page: Page): Promise<boolean> {
  const trigger = page.getByTestId('reading-more-actions')
  if ((await trigger.count()) === 0) return false
  const menu = page.getByRole('menu')
  if ((await trigger.getAttribute('aria-expanded')) !== 'true') {
    await expect(menu).toHaveCount(0)
    await trigger.click()
  }
  await expect(trigger).toHaveAttribute('aria-expanded', 'true')
  await expect(menu).toBeVisible()
  return true
}

/** Closes the "More" menu if it is open, so the page behind it is clickable. */
export async function closeMoreActions(page: Page): Promise<void> {
  const trigger = page.getByTestId('reading-more-actions')
  if ((await trigger.count()) === 0) return
  if ((await trigger.getAttribute('aria-expanded')) === 'true') {
    await page.keyboard.press('Escape')
  }
  // Gone, not just closing: what comes next may click what it covered.
  await expect(page.getByRole('menu')).toHaveCount(0)
}
