import type { ColumnDef } from '@tanstack/react-table'
import type { ReactNode } from 'react'
import { Link } from 'react-router'

import { type LastWrite, marksLastWrite } from '@/components/lastWrite'
import { LastWriteIndicator } from '@/components/LastWriteIndicator'
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
  /**
   * The row's last write (#1452). When it came from MCP, the CLI or the API,
   * the line under the name says so ("via MCP · Claude Code"), after the
   * summary when there is one; a web write and an unrecorded one add nothing.
   */
  lastWrite?: (row: T) => LastWrite
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
  lastWrite,
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
      const write = lastWrite?.(item)
      const marked =
        write !== undefined && marksLastWrite(write.last_write_source)
      const hasMetaLine = Boolean(description) || marked
      const textClass = cn(
        baseTextClass,
        subdued?.(item) && 'text-muted-foreground font-normal'
      )
      return (
        <div
          className={cn(
            'min-w-0 max-w-md',
            // A wrapping name is set with looser leading, which is already the
            // gap; adding one would push a one-line row past the height the
            // row actions give it.
            !multiline && 'space-y-0.5',
            className
          )}
        >
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
          {hasMetaLine && (
            // One line, whatever it holds: the summary gives way first, then
            // the assistant's name, so the channel itself is always legible.
            <p className="text-muted-foreground flex min-w-0 text-xs">
              {description && <span className="truncate">{description}</span>}
              {description && marked && (
                <span aria-hidden="true" className="shrink-0 px-1.5">
                  ·
                </span>
              )}
              {marked && (
                <LastWriteIndicator
                  variant="inline"
                  source={write.last_write_source}
                  assistantName={write.last_write_assistant_name}
                  className="max-w-40 shrink-0 truncate"
                />
              )}
            </p>
          )}
        </div>
      )
    },
  }
}
