import type { ColumnDef } from '@tanstack/react-table'
import type { ComponentType } from 'react'

import { Button } from '@/components/ui/button'

export interface RowAction<T> {
  /**
   * Verb naming the action. It is also half the test id:
   * `<key>-<singular>-button`, e.g. `delete-prompt-button`.
   */
  key: string
  /** Accessible name; a function when it interpolates the row (agents do). */
  label: string | ((row: T) => string)
  icon: ComponentType<{ className?: string }>
  onSelect: (row: T) => void
  /** Permission gate — omitted means always shown. */
  visible?: (row: T) => boolean
}

export interface ActionsColumnOptions<T> {
  /** The resource noun, from `descriptor.singular`. */
  singular: string
  actions: readonly RowAction<T>[]
}

/**
 * The trailing row-action column.
 *
 * Every button carries `data-testid="<action>-<singular>-button"`. Before #907
 * only artifacts and prompts had a test id at all, and only on delete, so e2e
 * specs reached for the other three lists by icon or position.
 *
 * The `actions` id is load-bearing: `<ListTable>` right-aligns that header and
 * stops row-click propagation inside the cell.
 */
export function actionsColumn<T>({
  singular,
  actions,
}: ActionsColumnOptions<T>): ColumnDef<T> {
  return {
    id: 'actions',
    cell: ({ row }) => {
      const item = row.original
      return (
        <div className="flex justify-end gap-1 opacity-60 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
          {actions
            .filter(action => action.visible?.(item) ?? true)
            .map(action => {
              const Icon = action.icon
              const label =
                typeof action.label === 'function'
                  ? action.label(item)
                  : action.label
              return (
                <Button
                  key={action.key}
                  variant="ghost"
                  size="icon"
                  aria-label={label}
                  data-testid={`${action.key}-${singular}-button`}

                  onClick={() => {
                    action.onSelect(item)
                  }}
                >
                  <Icon className="size-4" />
                </Button>
              )
            })}
        </div>
      )
    },
  }
}
