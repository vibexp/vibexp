import type { ColumnDef } from '@tanstack/react-table'
import { Eye, Pencil, Share2, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router'

import { FreshnessBadge } from '@/components/FreshnessBadge'
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
}: {
  navigate: NavigateFunction
  onDelete: (prompt: Prompt) => void
  /**
   * Whether the current user may delete this prompt — own vs any, decided per
   * row from its creator (#225). Editing needs no gate: every role holds
   * `resource.update.any`.
   */
  canDelete: (prompt: Prompt) => boolean
}): ColumnDef<Prompt>[] {
  const detailPath = (prompt: Prompt) => `/prompts/${prompt.slug}`
  return columnList<Prompt>(
    nameColumn<Prompt>({
      field: fieldOfRole(descriptor, 'name'),
      value: prompt => prompt.name,
      to: detailPath,
      navigate,
      summary: prompt => prompt.description,
      // Renders nothing when the resource is fresh.
      adornment: prompt => <FreshnessBadge freshness={prompt.freshness} />,
    }),
    statusColumn<Prompt>({
      field: fieldOfRole(descriptor, 'status'),
      value: prompt => prompt.status,
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
          visible: canDelete,
        },
      ],
    })
  )
}
