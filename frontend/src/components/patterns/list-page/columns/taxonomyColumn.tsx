import type { ColumnDef } from '@tanstack/react-table'

import type { FieldSpec } from '@/components/patterns/resource'
import { TaxonomyChips } from '@/components/TaxonomyChips'
import { Badge } from '@/components/ui/badge'

export interface TaxonomyColumnOptions<T> {
  /**
   * The `role: 'taxonomy'` field. Memory's list tags are lifted out of its
   * `metadata` bag rather than declared as a field (#904), so that page passes
   * a local `FieldSpec` instead of one off the descriptor.
   */
  field: FieldSpec | undefined
  values: (row: T) => readonly string[]
  /** How many chips before the "+N" overflow badge. */
  max?: number
}

/**
 * A grouping field as chips, with the "+N" overflow the lists already had.
 *
 * The chips themselves are `<TaxonomyChips>` (#904) so a label on a list and
 * the same label in the details column read identically; only the overflow
 * badge, which is list-specific, lives here.
 */
export function taxonomyColumn<T>({
  field,
  values,
  max = 3,
}: TaxonomyColumnOptions<T>): ColumnDef<T> | undefined {
  if (!field) return undefined
  return {
    id: field.key,
    header: field.label,
    cell: ({ row }) => {
      const all = values(row.original)
      if (all.length === 0) {
        return <span className="text-muted-foreground text-xs">—</span>
      }
      const overflow = all.length - max
      return (
        <div className="flex flex-wrap items-center gap-1.5">
          <TaxonomyChips values={all.slice(0, max)} />
          {overflow > 0 && <Badge variant="outline">+{overflow}</Badge>}
        </div>
      )
    },
  }
}
