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
  /**
   * Display text for a raw value — `useStatusOptions(descriptor).labelOf`, so
   * a team's custom status reads by its display name (#1263). Defaults to the
   * descriptor's own labels, which is all a closed status (an agent's) has.
   */
  label?: (raw: string) => string
}

/**
 * Lifecycle state as the app's one status component.
 *
 * Both the tone and the wording of a system status come off the `FieldSpec`
 * (#903), so a new one is a descriptor edit and never a second tone table. A
 * custom status has no tone and reads `neutral`. Returns `undefined` for a kind
 * with no status field.
 */
export function statusColumn<T>({
  field,
  value,
  label,
}: StatusColumnOptions<T>): ColumnDef<T> | undefined {
  if (!field) return undefined
  const text = label ?? ((raw: string) => fieldLabel(field, raw))
  return {
    accessorKey: field.key,
    header: field.label,
    cell: ({ row }) => {
      const raw = value(row.original)
      return <StatusBadge tone={fieldTone(field, raw)}>{text(raw)}</StatusBadge>
    },
  }
}
