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

// Tones are NOT here: they live on the memory descriptor's `status` FieldSpec
// (#903) and are read by `statusColumn`/`ResourceMetadataSection` through
// `fieldTone`. The wrapper this module used to export for the list column went
// with #907, when the column started reading the descriptor directly.
