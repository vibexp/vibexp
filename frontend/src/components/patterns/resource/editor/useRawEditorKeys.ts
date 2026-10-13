import type { KeyboardEvent } from 'react'
import { useCallback, useRef } from 'react'

import { indentSelection } from './indent'

/** What the sr-only hint beside the field says about the Tab key. */
export const RAW_EDITOR_TAB_HINT =
  'Tab indents the line and Shift+Tab outdents it. Press Escape, then Tab, to leave the editor.'

/**
 * Replaces `[from, to)` of a textarea through the browser's own editing
 * command, so the change lands on the native undo stack and fires a real
 * `input` event. Returns false where that is unavailable (jsdom, or a browser
 * that has dropped `execCommand`), leaving the caller to set the value itself.
 */
function replaceThroughBrowser(
  el: HTMLTextAreaElement,
  from: number,
  to: number,
  text: string
): boolean {
  if (typeof document.execCommand !== 'function') return false
  el.setSelectionRange(from, to)
  try {
    return text === ''
      ? document.execCommand('delete')
      : document.execCommand('insertText', false, text)
  } catch {
    return false
  }
}

/** The smallest replacement that turns `before` into `after`. */
export function changedSpan(
  before: string,
  after: string
): { from: number; to: number; text: string } {
  const limit = Math.min(before.length, after.length)
  let prefix = 0
  while (prefix < limit && before[prefix] === after[prefix]) prefix += 1
  let suffix = 0
  while (
    suffix < limit - prefix &&
    before[before.length - 1 - suffix] === after[after.length - 1 - suffix]
  ) {
    suffix += 1
  }
  return {
    from: prefix,
    to: before.length - suffix,
    text: after.slice(prefix, after.length - suffix),
  }
}

/**
 * Tab handling for the Raw body editor.
 *
 * Tab and Shift+Tab indent and outdent instead of moving focus, which is what
 * anyone writing markdown lists or code expects. That alone would trap a
 * keyboard user in the field (WCAG 2.1.2), so Escape releases the key: the
 * next Tab moves focus as usual. Any other key, or leaving the field, re-arms
 * indentation.
 */
export function useRawEditorKeys(onChange: (value: string) => void): {
  onKeyDown: (event: KeyboardEvent<HTMLTextAreaElement>) => void
  onBlur: () => void
} {
  const released = useRef(false)

  const onKeyDown = useCallback(
    (event: KeyboardEvent<HTMLTextAreaElement>) => {
      if (event.key === 'Escape') {
        released.current = true
        return
      }
      if (event.key !== 'Tab') {
        // Modifier keys on their own (Shift, on the way to Shift+Tab) do not
        // re-arm; anything else does.
        if (!['Shift', 'Control', 'Alt', 'Meta'].includes(event.key)) {
          released.current = false
        }
        return
      }
      if (event.ctrlKey || event.altKey || event.metaKey) return
      if (released.current) {
        released.current = false
        return
      }

      event.preventDefault()
      const el = event.currentTarget
      const before = {
        value: el.value,
        start: el.selectionStart,
        end: el.selectionEnd,
      }
      const after = indentSelection(before, event.shiftKey)
      if (after.value === before.value) return

      // Only the span that differs is replaced, so undo restores exactly it.
      const edit = changedSpan(before.value, after.value)

      if (!replaceThroughBrowser(el, edit.from, edit.to, edit.text)) {
        onChange(after.value)
      }
      // After React has written the new value back, or the caret jumps to the
      // end of the field.
      const restore = () => {
        el.setSelectionRange(after.start, after.end)
      }
      restore()
      requestAnimationFrame(restore)
    },
    [onChange]
  )

  const onBlur = useCallback(() => {
    released.current = false
  }, [])

  return { onKeyDown, onBlur }
}
