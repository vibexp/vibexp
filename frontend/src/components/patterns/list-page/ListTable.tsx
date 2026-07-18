import type { ColumnDef } from '@tanstack/react-table'
import { ChevronDown, ChevronsUpDown, ChevronUp } from 'lucide-react'
import type { AriaAttributes, MouseEventHandler } from 'react'

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

import type { SortDir } from './types'

/**
 * Column meta supported by `<ListTable>`. Apply via the standard TanStack
 * `meta` field on a `ColumnDef`:
 *
 * ```ts
 * { accessorKey: 'total_runs', header: 'Total runs', meta: { align: 'right' } }
 * ```
 */
export interface ListTableColumnMeta {
  /** Horizontal alignment for header and body cells. Defaults to 'left'. */
  align?: 'left' | 'right'
}

interface ListTableProps<T, SortKey extends string = string> {
  rows: T[]
  columns: ColumnDef<T>[]
  /** Accessor keys allowed to be sorted. Header cells become click-to-sort when present. */
  sortableKeys?: readonly SortKey[]
  sortKey?: SortKey
  sortDir?: SortDir
  onSortChange?: (key: SortKey) => void
  /** Optional row click handler. Cells inside the `actions` column are excluded from row clicks. */
  onRowClick?: (row: T) => void
}

function alignClass(meta: ListTableColumnMeta | undefined): string | undefined {
  return meta?.align === 'right' ? 'text-right' : undefined
}

function sortIcon(active: boolean, sortDir: SortDir | undefined) {
  if (!active) return ChevronsUpDown
  return sortDir === 'asc' ? ChevronUp : ChevronDown
}

function sortAriaValue(
  active: boolean,
  isSortable: boolean,
  sortDir: SortDir | undefined
): AriaAttributes['aria-sort'] {
  if (active) return sortDir === 'asc' ? 'ascending' : 'descending'
  return isSortable ? 'none' : undefined
}

interface ListTableHeadCellProps {
  isActionsColumn: boolean
  header: string
  align: string | undefined
  isSortable: boolean
  active: boolean
  sortDir: SortDir | undefined
  onSort: (() => void) | undefined
}

function ListTableHeadCell({
  isActionsColumn,
  header,
  align,
  isSortable,
  active,
  sortDir,
  onSort,
}: Readonly<ListTableHeadCellProps>) {
  const Icon = sortIcon(active, sortDir)
  return (
    <TableHead
      className={cn(
        'h-9 text-xs font-medium',
        isActionsColumn && 'text-right',
        align,
        isSortable && 'cursor-pointer select-none hover:text-foreground',
        active && 'text-foreground'
      )}
      aria-sort={sortAriaValue(active, isSortable, sortDir)}
      role={isSortable ? 'button' : undefined}
      tabIndex={isSortable ? 0 : undefined}
      onClick={onSort}
      onKeyDown={
        isSortable
          ? e => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onSort?.()
              }
            }
          : undefined
      }
    >
      <span
        className={cn(
          'inline-flex items-center gap-1',
          align === 'text-right' && 'justify-end'
        )}
      >
        {header}
        {isSortable && (
          <Icon
            className={cn('size-3', active ? 'opacity-100' : 'opacity-40')}
          />
        )}
      </span>
    </TableHead>
  )
}

export function ListTable<
  T extends { id: string },
  SortKey extends string = string,
>({
  rows,
  columns,
  sortableKeys,
  sortKey,
  sortDir,
  onSortChange,
  onRowClick,
}: Readonly<ListTableProps<T, SortKey>>) {
  const sortable = new Set<string>(sortableKeys ?? [])
  const sortingEnabled = sortable.size > 0 && onSortChange !== undefined

  return (
    <Table>
      <TableHeader>
        <TableRow className="bg-muted/40 hover:bg-muted/40">
          {columns.map(col => {
            const accessorKey = (col as { accessorKey?: string }).accessorKey
            const key = col.id ?? accessorKey
            const isSortable =
              sortingEnabled &&
              accessorKey !== undefined &&
              sortable.has(accessorKey)
            const active = isSortable && sortKey === accessorKey
            const triggerSort = isSortable
              ? () => {
                  onSortChange(accessorKey as SortKey)
                }
              : undefined
            return (
              <ListTableHeadCell
                key={key}
                isActionsColumn={col.id === 'actions'}
                header={typeof col.header === 'string' ? col.header : ''}
                align={alignClass(col.meta)}
                isSortable={isSortable}
                active={active}
                sortDir={sortDir}
                onSort={triggerSort}
              />
            )
          })}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map(row => {
          const handleRowClick:
            | MouseEventHandler<HTMLTableRowElement>
            | undefined = onRowClick
            ? () => {
                onRowClick(row)
              }
            : undefined
          return (
            <TableRow
              key={row.id}
              className={cn(
                'group border-border border-b hover:bg-muted/40',
                onRowClick && 'cursor-pointer'
              )}
              onClick={handleRowClick}
            >
              {columns.map(col => {
                const render = col.cell
                const colKey =
                  col.id ?? (col as { accessorKey?: string }).accessorKey
                const meta: ListTableColumnMeta | undefined = col.meta
                const align = alignClass(meta)
                // Stop propagation inside the actions column so action buttons
                // don't also trigger the row click handler.
                const stopProp =
                  onRowClick && col.id === 'actions'
                    ? (e: React.MouseEvent) => {
                        e.stopPropagation()
                      }
                    : undefined
                if (typeof render === 'function') {
                  return (
                    <TableCell
                      key={colKey}
                      className={cn('py-3', align)}
                      onClick={stopProp}
                    >
                      {render({
                        row: { original: row },
                      } as Parameters<typeof render>[0])}
                    </TableCell>
                  )
                }
                return (
                  <TableCell key={colKey} className={cn('py-3', align)}>
                    —
                  </TableCell>
                )
              })}
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
