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
import type { Artifact } from '@/services/artifactService'

const descriptor = getResourceDescriptor('artifact')

// Types are team-customizable, so the artifact row carries only the slug.
// Fall back to a readable label derived from the slug (e.g. "work-reports" ->
// "Work reports") when the type's display name isn't available.
function humanizeSlug(slug: string): string {
  if (!slug) return ''
  const spaced = slug.replaceAll('-', ' ')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

export function buildArtifactsColumns({
  navigate,
  onDelete,
  canDelete,
  typeNames,
}: {
  navigate: NavigateFunction
  onDelete: (artifact: Artifact) => void
  /**
   * Whether the current user may delete this artifact — own vs any, decided per
   * row from its creator (#225). Editing needs no gate: every role holds
   * `resource.update.any`.
   */
  canDelete: (artifact: Artifact) => boolean
  // Maps a type slug to its display name so the badge shows the team's chosen
  // name (matching the form/filter); a Map keeps the lookup free of the
  // object-injection lint. Falls back to the humanized slug when absent.
  typeNames?: Map<string, string>
}): ColumnDef<Artifact>[] {
  const detailPath = (artifact: Artifact) =>
    `/artifacts/${encodeURIComponent(artifact.project_id)}/${encodeURIComponent(artifact.slug)}`
  return columnList<Artifact>(
    nameColumn<Artifact>({
      field: fieldOfRole(descriptor, 'name'),
      value: artifact => artifact.title,
      to: detailPath,
      navigate,
      summary: artifact => artifact.description,
      // Renders nothing when the resource is fresh, so no fresh row pays any
      // layout for this.
      adornment: artifact => <FreshnessBadge freshness={artifact.freshness} />,
    }),
    typeColumn<Artifact>({
      field: fieldOfRole(descriptor, 'type'),
      value: artifact => artifact.type,
      label: slug => typeNames?.get(slug) ?? humanizeSlug(slug),
    }),
    statusColumn<Artifact>({
      field: fieldOfRole(descriptor, 'status'),
      value: artifact => artifact.status,
    }),
    updatedColumn<Artifact>({ value: artifact => artifact.updated_at }),
    actionsColumn<Artifact>({
      singular: descriptor.singular,
      actions: [
        {
          key: 'view',
          label: 'View',
          icon: Eye,
          onSelect: artifact => {
            void navigate(detailPath(artifact))
          },
        },
        {
          key: 'edit',
          label: 'Edit',
          icon: Pencil,
          onSelect: artifact => {
            void navigate(`${detailPath(artifact)}/edit`)
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
