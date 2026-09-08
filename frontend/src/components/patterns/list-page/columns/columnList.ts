import type { ColumnDef } from '@tanstack/react-table'

/**
 * Assembles a column list, dropping the gaps.
 *
 * The factories return `undefined` for a column the descriptor cannot describe
 * (a prompt has no `type` field, a gallery prompt no `status`), and every list
 * has at least one conditional column of its own (memory's `project`/`tags`).
 * Composing through this helper keeps both cases a single expression instead of
 * the `const columns = […]; if (…) columns.push(…)` shape the five files used to
 * repeat.
 */
export function columnList<T>(
  ...items: readonly (ColumnDef<T> | false | null | undefined)[]
): ColumnDef<T>[] {
  return items.filter((item): item is ColumnDef<T> => Boolean(item))
}
