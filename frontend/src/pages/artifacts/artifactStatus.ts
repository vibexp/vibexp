import type { ArtifactStatus } from '@/types'

// Human-readable labels for each status (badge text, table cells).
export const ARTIFACT_STATUS_LABEL: Record<ArtifactStatus, string> = {
  active: 'Active',
  draft: 'Draft',
  archived: 'Archived',
}

// Select options in display order. An explicit array (rather than mapping over
// labels with a computed key) keeps form/filter <Select>s free of the
// security/detect-object-injection lint warning.
export const ARTIFACT_STATUS_OPTIONS: readonly {
  value: ArtifactStatus
  label: string
}[] = [
  { value: 'active', label: 'Active' },
  { value: 'draft', label: 'Draft' },
  { value: 'archived', label: 'Archived' },
]

// Distinct StatusBadge tones per status so the three states read differently:
// active = success (green), draft = warning (amber), archived = neutral (muted).
export function artifactStatusTone(
  status: ArtifactStatus
): 'success' | 'warning' | 'neutral' {
  switch (status) {
    case 'active':
      return 'success'
    case 'draft':
      return 'warning'
    case 'archived':
      return 'neutral'
  }
}
