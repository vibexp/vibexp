import type { Page } from '@playwright/test'

/**
 * Opens the reading toolbar's "More" menu, where every action that is not a
 * labelled toolbar button lives (Lock, Unlock, Version history, Delete).
 *
 * A menu item only exists while the menu is open, so open it before asserting
 * that an action is — or is NOT — offered: `toHaveCount(0)` against a closed
 * menu passes for the wrong reason. Returns `false` when the page has no menu
 * at all, which itself means it offers no such action. Idempotent: an already
 * open menu is left open.
 */
export async function openMoreActions(page: Page): Promise<boolean> {
  const trigger = page.getByTestId('reading-more-actions')
  if ((await trigger.count()) === 0) return false
  if ((await trigger.getAttribute('aria-expanded')) !== 'true') {
    await trigger.click()
  }
  return true
}

/** Closes the "More" menu if it is open, so the page behind it is clickable. */
export async function closeMoreActions(page: Page): Promise<void> {
  const trigger = page.getByTestId('reading-more-actions')
  if ((await trigger.count()) === 0) return
  if ((await trigger.getAttribute('aria-expanded')) === 'true') {
    await page.keyboard.press('Escape')
  }
}
