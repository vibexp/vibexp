import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

/**
 * Opens the reading toolbar's "More" menu, where every action that is not a
 * labelled toolbar button lives (Lock, Version history, Delete…).
 *
 * A menu item is only in the DOM while the menu is open, so a test must open
 * it before asserting that an action is — or, just as importantly, is NOT —
 * offered: asserting absence against a closed menu passes for the wrong
 * reason. Returns `false` when the page offers no menu at all, which is itself
 * a statement that it has no such actions.
 */
export async function openMoreActions(): Promise<boolean> {
  const trigger = screen.queryByRole('button', { name: 'More actions' })
  if (!trigger) return false
  if (trigger.getAttribute('aria-expanded') !== 'true') {
    await userEvent.setup().click(trigger)
  }
  return true
}
