import type { ColumnDef } from '@tanstack/react-table'
import type { ReactNode } from 'react'
import { Link } from 'react-router'

import type { FieldSpec } from '@/components/patterns/resource'
import { cn } from '@/lib/utils'

export interface NameColumnOptions<T> {
  /** The descriptor's `role: 'name'` field — `fieldOfRole(descriptor, 'name')`. */
  field: FieldSpec | undefined
  /** The row's display text. Passed explicitly so no cell indexes a row by key. */
  value: (row: T) => string
  /** Header override for a list whose column is titled differently (memory: "Content"). */
  header?: string
  /**
   * Detail route for the row. Renders the name as a real link to it — an
   * anchor with an `href`, so middle-click, Ctrl/Cmd-click and "open in new
   * tab" work and the URL shows on hover, none of which a scripted button
   * gives.
   */
  to?: (row: T) => string
  /**
   * Rows whose name is a stand-in rather than a chosen one (an untitled
   * memory's body excerpt). Rendered subdued.
   */
  subdued?: (row: T) => boolean
  /** One-line description under the name. Falsy renders nothing. */
  summary?: (row: T) => string | null | undefined
  /** Per-resource cell rendered beside the name: `<FreshnessBadge>`, `<LockIndicator>`. */
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
  subdued,
  summary,
  adornment,
  className,
  multiline,
}: NameColumnOptions<T>): ColumnDef<T> | undefined {
  if (!field) return undefined
  const baseTextClass = cn(
    'block min-w-0 flex-1 text-left text-sm font-medium',
    multiline ? 'leading-relaxed' : 'truncate'
  )
  return {
    accessorKey: field.key,
    header: header ?? field.label,
    cell: ({ row }) => {
      const item = row.original
      const description = summary?.(item)
      const textClass = cn(
        baseTextClass,
        subdued?.(item) && 'text-muted-foreground font-normal'
      )
      return (
        <div className={cn('min-w-0 max-w-md space-y-0.5', className)}>
          <div className="flex min-w-0 items-center gap-2">
            {to ? (
              <Link
                to={to(item)}
                className={cn(
                  textClass,
                  'hover:text-primary underline-offset-2 hover:underline'
                )}
              >
                {value(item)}
              </Link>
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
