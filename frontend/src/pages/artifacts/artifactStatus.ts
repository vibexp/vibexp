import type { ArtifactStatus } from '@/services/artifactService'

// Labels are NOT here either: the descriptor's `valueLabels` is what badges and
// table cells read (#903), through `fieldLabel`. The map this module exported
// lost its last caller with #907, for the same reason the tone wrapper did.

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

// Tones are NOT here: they live on the artifact descriptor's `status` FieldSpec
// (#903) and are read by `statusColumn`/`ResourceMetadataSection` through
// `fieldTone`. The wrapper this module used to export for the list column went
// with #907, when the column started reading the descriptor directly.
