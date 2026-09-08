import type { ColumnDef } from '@tanstack/react-table'

import {
  fieldLabel,
  type FieldSpec,
  fieldTone,
} from '@/components/patterns/resource'
import { StatusBadge } from '@/components/StatusBadge'

export interface StatusColumnOptions<T> {
  /**
   * The descriptor's `role: 'status'` field — `fieldOfRole(descriptor,
   * 'status')`, or the equivalent `statusFieldOf(descriptor)`.
   */
  field: FieldSpec | undefined
  value: (row: T) => string
}

/**
 * Lifecycle state as the app's one status component.
 *
 * Both the tone and the wording come off the `FieldSpec` (#903), so a new
 * status value is a descriptor edit and never a second tone table. Returns
 * `undefined` for a kind with no status field.
 */
export function statusColumn<T>({
  field,
  value,
}: StatusColumnOptions<T>): ColumnDef<T> | undefined {
  if (!field) return undefined
  return {
    accessorKey: field.key,
    header: field.label,
    cell: ({ row }) => {
      const raw = value(row.original)
      return (
        <StatusBadge tone={fieldTone(field, raw)}>
          {fieldLabel(field, raw)}
        </StatusBadge>
      )
    },
  }
}
