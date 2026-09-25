import type { ColumnDef } from '@tanstack/react-table'
import { useCallback, useMemo, useState } from 'react'

import { formatDate } from '@/lib/time'
import type {
  AdminUserListItem,
  AdminUserListParams,
} from '@/services/adminService'
import { storage, STORAGE_KEYS } from '@/utils/storage'

/**
 * Count and last-activity columns of the admin users list (#1134).
 *
 * Numbers and a timestamp only: the list never renders a resource's title or
 * content (epic #1131's counts-only rule).
 */

export type UserSortKey = NonNullable<
  NonNullable<AdminUserListParams>['sort_by']
>

/** A count column; `id` doubles as the `sort_by` value `ListTable` sends. */
interface CountColumnSpec {
  id: UserSortKey
  header: string
  get: (user: AdminUserListItem) => number
  /** Shown until the viewer hides it through the column chooser. */
  defaultVisible: boolean
}

export const COUNT_COLUMNS: readonly CountColumnSpec[] = [
  {
    id: 'team_count',
    header: 'Teams',
    get: user => user.team_count,
    defaultVisible: true,
  },
  {
    id: 'project_count',
    header: 'Projects',
    get: user => user.project_count,
    defaultVisible: true,
  },
  {
    id: 'total_resource_count',
    header: 'Total resources',
    get: user => user.resource_counts.total,
    defaultVisible: true,
  },
  {
    id: 'prompt_count',
    header: 'Prompts',
    get: user => user.resource_counts.prompts,
    defaultVisible: true,
  },
  {
    id: 'memory_count',
    header: 'Memories',
    get: user => user.resource_counts.memories,
    defaultVisible: true,
  },
  {
    id: 'artifact_count',
    header: 'Artifacts',
    get: user => user.resource_counts.artifacts,
    defaultVisible: true,
  },
  {
    id: 'blueprint_count',
    header: 'Blueprints',
    get: user => user.resource_counts.blueprints,
    defaultVisible: false,
  },
  {
    id: 'agent_count',
    header: 'Agents',
    get: user => user.resource_counts.agents,
    defaultVisible: false,
  },
  {
    id: 'feed_count',
    header: 'Feeds',
    get: user => user.resource_counts.feeds,
    defaultVisible: false,
  },
  {
    id: 'feed_item_count',
    header: 'Feed items',
    get: user => user.resource_counts.feed_items,
    defaultVisible: false,
  },
  {
    id: 'comment_count',
    header: 'Comments',
    get: user => user.resource_counts.comments,
    defaultVisible: false,
  },
  {
    id: 'attachment_count',
    header: 'Attachments',
    get: user => user.resource_counts.attachments,
    defaultVisible: false,
  },
]

export const LAST_RESOURCE_COLUMN_ID = 'last_resource_created_at'

/** Every column the chooser toggles: the counts plus "Last resource". */
export const CHOOSABLE_COLUMNS: readonly {
  id: string
  header: string
  defaultVisible: boolean
}[] = [
  ...COUNT_COLUMNS,
  {
    id: LAST_RESOURCE_COLUMN_ID,
    header: 'Last resource',
    defaultVisible: true,
  },
]

export type ColumnVisibility = Record<string, boolean>

const DEFAULT_VISIBILITY: ColumnVisibility = Object.fromEntries(
  CHOOSABLE_COLUMNS.map(col => [col.id, col.defaultVisible])
)

/** A stored map is trusted per key, and only for booleans on known columns. */
function readStoredVisibility(): ColumnVisibility {
  const stored = storage.getJSON<unknown>(STORAGE_KEYS.ADMIN_USERS_COLUMNS)
  const visibility = { ...DEFAULT_VISIBILITY }
  if (stored === null || typeof stored !== 'object' || Array.isArray(stored)) {
    return visibility
  }
  for (const [id, value] of Object.entries(stored)) {
    if (id in visibility && typeof value === 'boolean') visibility[id] = value
  }
  return visibility
}

/**
 * Which choosable columns this viewer shows, remembered per browser. The active
 * sort column is always visible, so sorting by a hidden count never leaves the
 * admin looking at an order they cannot see.
 */
export function useUserColumnVisibility(sortBy: string) {
  const [stored, setStored] = useState<ColumnVisibility>(readStoredVisibility)

  const setVisible = useCallback((id: string, visible: boolean) => {
    setStored(prev => {
      const next = { ...prev, [id]: visible }
      storage.set(STORAGE_KEYS.ADMIN_USERS_COLUMNS, next)
      return next
    })
  }, [])

  const visibility = useMemo<ColumnVisibility>(
    () => ({ ...stored, [sortBy]: true }),
    [stored, sortBy]
  )
  return { visibility, setVisible }
}

function countColumn(spec: CountColumnSpec): ColumnDef<AdminUserListItem> {
  return {
    id: spec.id,
    header: spec.header,
    meta: { align: 'right' },
    cell: ({ row }) => (
      <span className="text-sm tabular-nums">{spec.get(row.original)}</span>
    ),
  }
}

const LAST_RESOURCE_COLUMN: ColumnDef<AdminUserListItem> = {
  id: LAST_RESOURCE_COLUMN_ID,
  header: 'Last resource',
  cell: ({ row }) => {
    const at = row.original.last_resource_created_at
    return (
      <span className="text-muted-foreground whitespace-nowrap text-xs">
        {at ? formatDate(at) : '—'}
      </span>
    )
  },
}

/** The visible count columns followed by "Last resource", in chooser order. */
export function userActivityColumns(
  visibility: ColumnVisibility
): ColumnDef<AdminUserListItem>[] {
  return [
    ...COUNT_COLUMNS.filter(spec => visibility[spec.id]).map(countColumn),
    ...(visibility[LAST_RESOURCE_COLUMN_ID] ? [LAST_RESOURCE_COLUMN] : []),
  ]
}
