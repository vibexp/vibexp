import type { ColumnDef } from '@tanstack/react-table'
import { Eye, Pencil, Trash2 } from 'lucide-react'
import type { NavigateFunction } from 'react-router-dom'

import { RelativeTime } from '@/components/RelativeTime'
import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  ARTIFACT_STATUS_LABEL,
  artifactStatusTone,
} from '@/pages/artifacts/artifactStatus'
import type { Artifact } from '@/services/artifactService'

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
  return [
    {
      accessorKey: 'title',
      header: 'Title',
      cell: ({ row }) => {
        const a = row.original
        const base = `/artifacts/${encodeURIComponent(a.project_id)}/${encodeURIComponent(a.slug)}`
        return (
          <div className="max-w-md space-y-0.5">
            <button
              type="button"
              className="hover:text-primary text-left text-sm font-medium underline-offset-2 hover:underline"
              onClick={() => {
                void navigate(base)
              }}
            >
              {a.title}
            </button>
            {a.description && (
              <p className="text-muted-foreground text-xs">
                {a.description.slice(0, 100)}
                {a.description.length > 100 ? '…' : ''}
              </p>
            )}
          </div>
        )
      },
    },
    {
      accessorKey: 'type',
      header: 'Type',
      cell: ({ row }) => {
        const slug = row.original.type
        return (
          <Badge variant="outline">
            {typeNames?.get(slug) ?? humanizeSlug(slug)}
          </Badge>
        )
      },
    },
    {
      accessorKey: 'status',
      header: 'Status',
      cell: ({ row }) => (
        <StatusBadge tone={artifactStatusTone(row.original.status)}>
          {ARTIFACT_STATUS_LABEL[row.original.status]}
        </StatusBadge>
      ),
    },
    {
      accessorKey: 'updated_at',
      header: 'Updated',
      cell: ({ row }) => (
        <RelativeTime
          value={row.original.updated_at}
          className="text-muted-foreground text-sm"
        />
      ),
    },
    {
      id: 'actions',
      cell: ({ row }) => {
        const a = row.original
        const base = `/artifacts/${encodeURIComponent(a.project_id)}/${encodeURIComponent(a.slug)}`
        return (
          <div className="flex justify-end gap-1 opacity-60 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
            <Button
              variant="ghost"
              size="icon"
              aria-label="View"
              onClick={() => {
                void navigate(base)
              }}
            >
              <Eye className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Edit"
              onClick={() => {
                void navigate(`${base}/edit`)
              }}
            >
              <Pencil className="size-4" />
            </Button>
            {canDelete(a) && (
              <Button
                variant="ghost"
                size="icon"
                aria-label="Delete"
                data-testid="delete-artifact-button"
                onClick={() => {
                  onDelete(a)
                }}
              >
                <Trash2 className="size-4" />
              </Button>
            )}
          </div>
        )
      },
    },
  ]
}
