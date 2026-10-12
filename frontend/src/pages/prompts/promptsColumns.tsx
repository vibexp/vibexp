import type { ColumnDef } from '@tanstack/react-table'
import { Eye, Pencil, Share2, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router'

import { FreshnessBadge } from '@/components/FreshnessBadge'
import { LockIndicator } from '@/components/LockIndicator'
import {
  actionsColumn,
  columnList,
  nameColumn,
  statusColumn,
  taxonomyColumn,
  updatedColumn,
} from '@/components/patterns/list-page'
import {
  fieldOfRole,
  getResourceDescriptor,
} from '@/components/patterns/resource'
import type { Prompt } from '@/services/promptService'

const descriptor = getResourceDescriptor('prompt')

/** Prompts are the only list with a "Shared" column, so it stays a plain def. */
const sharedColumn: ColumnDef<Prompt> = {
  id: 'shared',
  header: 'Shared',
  cell: ({ row }) =>
    row.original.is_shared ? (
      <span className="text-muted-foreground inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs">
        <Share2 className="size-3" />
        Shared
      </span>
    ) : (
      <span className="text-muted-foreground text-xs">—</span>
    ),
}

export function buildPromptsColumns({
  navigate,
  onDelete,
  canDelete,
  statusLabel,
}: {
  navigate: NavigateFunction
  onDelete: (prompt: Prompt) => void
  /**
   * Whether the current user may delete this prompt — own vs any, decided per
   * row from its creator (#225). Editing needs no gate: every role holds
   * `resource.update.any`.
   */
  canDelete: (prompt: Prompt) => boolean
  /**
   * Display text for a status value, so a team's custom status reads by its
   * display name (#1263). Defaults to the descriptor's own labels.
   */
  statusLabel?: (status: string) => string
}): ColumnDef<Prompt>[] {
  const detailPath = (prompt: Prompt) => `/prompts/${prompt.slug}`
  return columnList<Prompt>(
    nameColumn<Prompt>({
      field: fieldOfRole(descriptor, 'name'),
      value: prompt => prompt.name,
      to: detailPath,
      summary: prompt => prompt.description,
      // Each renders nothing by itself: the badge when the resource is fresh,
      // the icon when it is unlocked, so a plain row pays no layout for either.
      adornment: prompt => (
        <>
          <FreshnessBadge freshness={prompt.freshness} />
          <LockIndicator lock={prompt.lock} />
        </>
      ),
    }),
    statusColumn<Prompt>({
      field: fieldOfRole(descriptor, 'status'),
      value: prompt => prompt.status,
      label: statusLabel,
    }),
    sharedColumn,
    taxonomyColumn<Prompt>({
      field: fieldOfRole(descriptor, 'taxonomy'),
      values: prompt => prompt.labels ?? [],
    }),
    updatedColumn<Prompt>({ value: prompt => prompt.updated_at }),
    actionsColumn<Prompt>({
      singular: descriptor.singular,
      actions: [
        {
          key: 'view',
          label: 'View',
          icon: Eye,
          onSelect: prompt => {
            void navigate(detailPath(prompt))
          },
        },
        {
          key: 'edit',
          label: 'Edit',
          icon: Pencil,
          onSelect: prompt => {
            // Editor still lives in v1 until Slice 5b lands
            void navigate(`${detailPath(prompt)}/edit`)
          },
        },
        {
          key: 'delete',
          label: 'Delete',
          icon: Trash2,
          onSelect: onDelete,
          // A locked resource cannot be deleted by anyone, owners and admins
          // included: the server answers 423, so the action is not offered.
          visible: prompt => canDelete(prompt) && !prompt.lock,
        },
      ],
    })
  )
}
