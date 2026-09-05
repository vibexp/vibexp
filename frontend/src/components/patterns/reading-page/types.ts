import type { ComponentType, ReactNode } from 'react'

/** Any lucide-style icon component. */
export type ReadingIcon = ComponentType<{ className?: string }>

/**
 * A document-level action (Back, Copy, Edit, Delete, …). Declared as data so
 * the same list renders as buttons in the details column, icon buttons on the
 * folded rail, and chips under the title on phones — one source of truth.
 */
export interface ReadingAction {
  id: string
  label: string
  icon: ReadingIcon
  onClick: () => void
  /** `destructive` renders in the destructive role (Delete). */
  tone?: 'default' | 'destructive'
  /** `primary` renders as the solid call-to-action ("Use this prompt"). */
  emphasis?: 'primary' | 'secondary'
  disabled?: boolean
  /** Forwarded as `data-testid`; the same id follows the action across layouts. */
  testId?: string
}

/**
 * One block of the details panel. The rail shows `icon` + `label`; the open
 * column renders `content` under a `data-section` anchor so the rail can
 * scroll to it. Sections whose `content` is null/false are omitted entirely.
 */
export interface ReadingSection {
  id: string
  label: string
  icon: ReadingIcon
  content: ReactNode
}
