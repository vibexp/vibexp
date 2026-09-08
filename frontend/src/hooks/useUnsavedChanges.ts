import { useCallback, useEffect } from 'react'

/** What the confirm dialog asks before an in-app exit discards edits. */
export const UNSAVED_CHANGES_MESSAGE =
  'You have unsaved changes. Leave without saving?'

export interface UnsavedChangesGuard {
  /**
   * Ask before an exit the page owns (Cancel, Back, a link it renders).
   * Returns `true` when it is safe to leave — either nothing is dirty, or the
   * reader confirmed. Returns `false` to stay put.
   */
  confirmLeave: () => boolean
}

/**
 * Warn before unsaved edits are thrown away (#916).
 *
 * While `isDirty`, a `beforeunload` listener makes the browser show its own
 * "leave site?" prompt on a tab close, a reload, or navigation to another
 * origin. `confirmLeave()` covers the exits the *page* owns.
 *
 * ## What this deliberately does NOT cover
 *
 * Arbitrary in-app navigation — a sidebar link, the browser Back button, a
 * breadcrumb somewhere up the tree. Blocking those needs react-router's
 * `useBlocker`, which only exists under a data router; the app mounts
 * `BrowserRouter` (`App.tsx`), so the hook is unavailable and migrating is an
 * app-wide change well out of proportion to a guard. The chosen coverage is
 * tab-close plus the page's own exit paths, decided during #916's refinement.
 * **Do not assume a call to `useUnsavedChanges` makes a route un-leavable.**
 *
 * @param isDirty whether the form currently differs from its saved state.
 */
export function useUnsavedChanges(isDirty: boolean): UnsavedChangesGuard {
  useEffect(() => {
    if (!isDirty) return

    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      // Both halves are load-bearing across engines: Chrome/Safari honour
      // `preventDefault()`, older Firefox keys off a non-empty `returnValue`.
      // The string itself is never shown — every browser renders its own copy.
      event.preventDefault()
      event.returnValue = UNSAVED_CHANGES_MESSAGE
      return UNSAVED_CHANGES_MESSAGE
    }

    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload)
    }
  }, [isDirty])

  const confirmLeave = useCallback(() => {
    if (!isDirty) return true
    return window.confirm(UNSAVED_CHANGES_MESSAGE)
  }, [isDirty])

  return { confirmLeave }
}
