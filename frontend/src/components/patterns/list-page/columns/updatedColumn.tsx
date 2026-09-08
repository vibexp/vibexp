import type { ColumnDef } from '@tanstack/react-table'

import { RelativeTime } from '@/components/RelativeTime'

export interface UpdatedColumnOptions<T> {
  value: (row: T) => string | null | undefined
  /** Defaults to `updated_at`; agents' "Last run" reuses the column as `last_run`. */
  accessorKey?: string
  header?: string
}

/**
 * A timestamp column, relative everywhere.
 *
 * Before #907 this column had four implementations and two of them printed an
 * absolute date, so the same "Updated" read "3d ago" on one list and
 * "12 Aug 2026" on the next. `<RelativeTime>` is now the only renderer, and it
 * carries the absolute value in `title` so nothing is lost.
 */
export function updatedColumn<T>({
  value,
  accessorKey = 'updated_at',
  header = 'Updated',
}: UpdatedColumnOptions<T>): ColumnDef<T> {
  return {
    accessorKey,
    header,
    cell: ({ row }) => (
      <RelativeTime
        value={value(row.original)}
        className="text-muted-foreground whitespace-nowrap text-sm"
      />
    ),
  }
}
