import type { RefObject } from 'react'

import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { toast } from '@/lib/toast'

/** Marks the heading that takes focus when the step changes. */
export const STEP_HEADING_ATTR = 'data-step-heading'

/** The small uppercase overline above a step's heading. */
export const EYEBROW =
  'text-xs font-semibold tracking-[0.06em] text-muted-foreground uppercase'

/** How long a copy button says "Copied". */
const COPIED_MS = 1600

export const MANUAL_COPY_MESSAGE = 'Press Ctrl+C (⌘C) to copy'

/**
 * A copy button's state and action (the app's `useCopyToClipboard`). Where
 * the clipboard is refused or missing (a self-hosted instance on plain http
 * is not a secure context), the command on screen is selected instead, with
 * a toast saying how to copy it.
 */
export function useCopyCommand(target: RefObject<HTMLElement | null>) {
  const selectForManualCopy = () => {
    const element = target.current
    if (element) window.getSelection()?.selectAllChildren(element)
    toast.info(MANUAL_COPY_MESSAGE)
  }
  return useCopyToClipboard(COPIED_MS, selectForManualCopy)
}
