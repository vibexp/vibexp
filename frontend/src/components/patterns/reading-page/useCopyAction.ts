import { Check, Copy } from 'lucide-react'
import { useMemo } from 'react'

import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'

import type { ReadingAction } from './types'

interface CopyActionOptions {
  id?: string
  label?: string
  testId?: string
}

/**
 * A `ReadingAction` that copies `value` and flips to a "Copied!" check for a
 * moment — the action-list counterpart of `CopyButton`, sharing its hook so
 * both behave identically.
 */
export function useCopyAction(
  value: string,
  { id = 'copy', label = 'Copy content', testId }: CopyActionOptions = {}
): ReadingAction {
  const { copied, copy } = useCopyToClipboard()

  return useMemo<ReadingAction>(
    () => ({
      id,
      label: copied ? 'Copied!' : label,
      icon: copied ? Check : Copy,
      onClick: () => {
        copy(value)
      },
      testId,
    }),
    [copied, copy, id, label, testId, value]
  )
}
