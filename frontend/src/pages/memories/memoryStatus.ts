import { statusTone } from '@/components/patterns/resource'
import type { StatusTone } from '@/components/StatusBadge'
import type { MemoryStatus } from '@/services/memoryService'

// Human-readable labels for each status (badge text, table cells).
export const MEMORY_STATUS_LABEL: Record<MemoryStatus, string> = {
  active: 'Active',
  draft: 'Draft',
  archived: 'Archived',
}

// Select options in display order. An explicit array (rather than mapping over
// labels with a computed key) keeps form/filter <Select>s free of the
// security/detect-object-injection lint warning.
export const MEMORY_STATUS_OPTIONS: readonly {
  value: MemoryStatus
  label: string
}[] = [
  { value: 'active', label: 'Active' },
  { value: 'draft', label: 'Draft' },
  { value: 'archived', label: 'Archived' },
]

// Distinct StatusBadge tones per status so the three states read differently:
// active = success (green), draft = warning (amber), archived = neutral (muted).
// The table itself lives on the memory descriptor (#903) so the detail page's
// generated Status row and this list column cannot drift apart; this stays as
// the list columns' call-site-shaped entry point into it.
export function memoryStatusTone(status: MemoryStatus): StatusTone {
  return statusTone('memory', status)
}
