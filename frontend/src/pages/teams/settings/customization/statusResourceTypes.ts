import type { StatusResourceType } from '@/services/statusService'

/**
 * How the Statuses page names each resource type.
 *
 * A sibling data module rather than exports from `Statuses.tsx`, so that file
 * keeps exporting components only (`react-refresh/only-export-components`).
 */
export interface StatusResourceTypeLabel {
  value: StatusResourceType
  /** Tab label. */
  label: string
  /** "12 prompts will move". */
  plural: string
  /** "1 prompt will move", "Create prompt status". */
  singular: string
}

/** One tab per resource type, in the order the resources appear in the app. */
export const STATUS_RESOURCE_TYPES: readonly StatusResourceTypeLabel[] = [
  { value: 'prompt', label: 'Prompts', plural: 'prompts', singular: 'prompt' },
  {
    value: 'blueprint',
    label: 'Blueprints',
    plural: 'blueprints',
    singular: 'blueprint',
  },
  {
    value: 'memory',
    label: 'Memories',
    plural: 'memories',
    singular: 'memory',
  },
  {
    value: 'artifact',
    label: 'Artifacts',
    plural: 'artifacts',
    singular: 'artifact',
  },
]

/** "1 prompt", "12 prompts", "0 prompts". */
export function countLabel(
  count: number,
  type: StatusResourceTypeLabel
): string {
  return `${String(count)} ${count === 1 ? type.singular : type.plural}`
}
