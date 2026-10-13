import { useEffect } from 'react'

import { useShell } from '@/components/layout/ShellContext'

/** The key that toggles focus mode — shown in the control's tooltip. */
export const FOCUS_MODE_KEY = 'F'

/** True while the user is typing somewhere a bare letter is text, not a command. */
function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    target.tagName === 'INPUT' ||
    target.tagName === 'TEXTAREA' ||
    target.tagName === 'SELECT'
  )
}

/** A menu, dialog or sheet is open: Escape belongs to it, not to focus mode. */
function overlayIsOpen(): boolean {
  return (
    document.querySelector(
      '[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]'
    ) !== null
  )
}

/**
 * Focus mode for a reading page: one toggle that hides the navigation and the
 * details panel and leaves the text. The shell owns the state (it hides its own
 * sidebar); this adds the keyboard: a bare `F` toggles it — never while typing
 * in a field, and never with a modifier, so it cannot collide with a browser
 * shortcut or with text entry — and Escape leaves it.
 *
 * Focus mode overlays the two stored panel preferences without touching them,
 * so leaving it restores each as it was; and it is not stored, so a page load
 * never starts in it.
 */
export function useFocusMode(enabled: boolean): {
  focusMode: boolean
  toggleFocusMode: () => void
} {
  const { focusMode, setFocusMode } = useShell()

  useEffect(() => {
    if (!enabled) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return
      if (event.ctrlKey || event.metaKey || event.altKey) return
      if (event.key === 'Escape') {
        if (focusMode && !overlayIsOpen()) setFocusMode(false)
        return
      }
      if (event.key.toUpperCase() !== FOCUS_MODE_KEY) return
      if (isTyping(event.target) || overlayIsOpen()) return
      event.preventDefault()
      setFocusMode(!focusMode)
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [enabled, focusMode, setFocusMode])

  return {
    focusMode,
    toggleFocusMode: () => {
      setFocusMode(!focusMode)
    },
  }
}
