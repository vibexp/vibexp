import type { ColumnDef } from '@tanstack/react-table'
import { Eye, Pencil, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router'

import { FreshnessBadge } from '@/components/FreshnessBadge'
import {
  actionsColumn,
  columnList,
  nameColumn,
  statusColumn,
  typeColumn,
  updatedColumn,
} from '@/components/patterns/list-page'
import {
  fieldOfRole,
  getResourceDescriptor,
} from '@/components/patterns/resource'
import type { Blueprint } from '@/services/blueprintService'

const descriptor = getResourceDescriptor('blueprint')

export function buildBlueprintsColumns({
  navigate,
  onDelete,
  canDelete,
}: {
  navigate: NavigateFunction
  onDelete: (blueprint: Blueprint) => void
  /**
   * Whether the current user may delete this blueprint — own vs any, decided
   * per row from its creator (#225). Editing needs no gate: every role holds
   * `resource.update.any`.
   */
  canDelete: (blueprint: Blueprint) => boolean
}): ColumnDef<Blueprint>[] {
  const detailPath = (blueprint: Blueprint) =>
    `/blueprints/${encodeURIComponent(blueprint.project_id)}/${encodeURIComponent(blueprint.slug)}`
  return columnList<Blueprint>(
    nameColumn<Blueprint>({
      field: fieldOfRole(descriptor, 'name'),
      value: blueprint => blueprint.title,
      to: detailPath,
      navigate,
      summary: blueprint => blueprint.description,
      // Renders nothing when the resource is fresh.
      adornment: blueprint => (
        <FreshnessBadge freshness={blueprint.freshness} />
      ),
    }),
    typeColumn<Blueprint>({
      field: fieldOfRole(descriptor, 'type'),
      value: blueprint => blueprint.type,
    }),
    statusColumn<Blueprint>({
      field: fieldOfRole(descriptor, 'status'),
      value: blueprint => blueprint.status,
    }),
    updatedColumn<Blueprint>({ value: blueprint => blueprint.updated_at }),
    actionsColumn<Blueprint>({
      singular: descriptor.singular,
      actions: [
        {
          key: 'view',
          label: 'View',
          icon: Eye,
          onSelect: blueprint => {
            void navigate(detailPath(blueprint))
          },
        },
        {
          key: 'edit',
          label: 'Edit',
          icon: Pencil,
          onSelect: blueprint => {
            void navigate(`${detailPath(blueprint)}/edit`)
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
