import type { ColumnDef } from '@tanstack/react-table'
import type { ReactNode } from 'react'

import type { FieldSpec } from '@/components/patterns/resource'
import { cn } from '@/lib/utils'

export interface NameColumnOptions<T> {
  /** The descriptor's `role: 'name'` field — `fieldOfRole(descriptor, 'name')`. */
  field: FieldSpec | undefined
  /** The row's display text. Passed explicitly so no cell indexes a row by key. */
  value: (row: T) => string
  /** Header override for a list whose column is titled differently (memory: "Content"). */
  header?: string
  /** Detail route for the row. Renders the name as a link when given with `navigate`. */
  to?: (row: T) => string
  /**
   * `useNavigate()`'s function. Typed loosely on the return so react-router's
   * `void | Promise<void>` signature does not trip `no-misused-promises` at
   * every call site.
   */
  navigate?: (to: string) => unknown
  /** One-line description under the name. Falsy renders nothing. */
  summary?: (row: T) => string | null | undefined
  /** Per-resource cell rendered beside the name — today only `<FreshnessBadge>`. */
  adornment?: (row: T) => ReactNode
  /** Width override for the cell (`max-w-md` by default). */
  className?: string
  /** Let the name wrap instead of truncating — memory's excerpt is a paragraph. */
  multiline?: boolean
}

/**
 * The primary column of a resource list: the row's name, optionally linked to
 * its detail route, with a summary line under it and room for one per-resource
 * adornment.
 *
 * The `role: 'name'` field is what makes this one function serve five lists —
 * a prompt calls it `name`, an artifact `title`, a memory `text`, and the
 * descriptor is where that difference lives.
 */
export function nameColumn<T>({
  field,
  value,
  header,
  to,
  navigate,
  summary,
  adornment,
  className,
  multiline,
}: NameColumnOptions<T>): ColumnDef<T> | undefined {
  if (!field) return undefined
  const linked = to !== undefined && navigate !== undefined
  const textClass = cn(
    'block min-w-0 flex-1 text-left text-sm font-medium',
    multiline ? 'leading-relaxed' : 'truncate'
  )
  return {
    accessorKey: field.key,
    header: header ?? field.label,
    cell: ({ row }) => {
      const item = row.original
      const description = summary?.(item)
      return (
        <div className={cn('min-w-0 max-w-md space-y-0.5', className)}>
          <div className="flex min-w-0 items-center gap-2">
            {linked ? (
              <button
                type="button"
                className={cn(
                  textClass,
                  'hover:text-primary underline-offset-2 hover:underline'
                )}
                onClick={() => {
                  void navigate(to(item))
                }}
              >
                {value(item)}
              </button>
            ) : (
              <span className={textClass}>{value(item)}</span>
            )}
            {adornment?.(item)}
          </div>
          {description && (
            <p className="text-muted-foreground truncate text-xs">
              {description}
            </p>
          )}
        </div>
      )
    },
  }
}
