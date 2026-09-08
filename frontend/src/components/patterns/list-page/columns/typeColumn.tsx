import type { ColumnDef } from '@tanstack/react-table'

import { fieldLabel, type FieldSpec } from '@/components/patterns/resource'
import { Badge } from '@/components/ui/badge'

export interface TypeColumnOptions<T> {
  /** The descriptor's `role: 'type'` field — `fieldOfRole(descriptor, 'type')`. */
  field: FieldSpec | undefined
  value: (row: T) => string
  /**
   * Display text override. Artifacts need it: their types are team-defined, so
   * the name comes from the team's own type registry rather than from the
   * descriptor's (partial, built-in-only) `valueLabels`.
   */
  label?: (raw: string) => string
}

/**
 * The resource's own sub-classification as an outline badge.
 *
 * Returns `undefined` for a kind with no `type` field (prompts, memories), so a
 * page can compose it unconditionally through {@link columnList}.
 */
export function typeColumn<T>({
  field,
  value,
  label,
}: TypeColumnOptions<T>): ColumnDef<T> | undefined {
  if (!field) return undefined
  const text = label ?? ((raw: string) => fieldLabel(field, raw))
  return {
    accessorKey: field.key,
    header: field.label,
    cell: ({ row }) => (
      <Badge variant="outline">{text(value(row.original))}</Badge>
    ),
  }
}
