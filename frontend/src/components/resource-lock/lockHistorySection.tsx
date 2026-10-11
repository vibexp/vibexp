import { Lock } from 'lucide-react'

import type { ReadingSection } from '@/components/patterns/reading-page'

import { LockHistoryPanel, type LockHistoryState } from './LockHistoryPanel'

/**
 * The "Lock history" section of the details column, for a detail page's
 * `extraSections`. A resource that was never locked has no events and gets no
 * section, the same way the prompt's "Used by" section stays away while empty.
 */
export function lockHistorySection(state: LockHistoryState): ReadingSection[] {
  if (state.events.length === 0 && state.error === null) return []
  return [
    {
      id: 'lock-history',
      label: 'Lock history',
      icon: Lock,
      content: <LockHistoryPanel {...state} />,
    },
  ]
}
