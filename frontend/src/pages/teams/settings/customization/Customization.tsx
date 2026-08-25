import type { ColumnDef } from '@tanstack/react-table'
import { Copy, Plus, Shapes, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { DataTable } from '@/components/DataTable'
import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { CopyFromTeamDialog } from '@/components/settings/CopyFromTeamDialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { toast } from '@/lib/toast'
import type { Team } from '@/services/teamService'
import type { CreateTypeRequest, Type } from '@/services/typeService'
import { typeService } from '@/services/typeService'

import { CreateTypeDialog, type CreateTypeFormValues } from './CreateTypeDialog'

const ARTIFACTS_RESOURCE = 'artifacts'

interface CopyPreview {
  willAdd: Type[]
  alreadyExists: Type[]
}

function buildColumns(onDelete: (type: Type) => void): ColumnDef<Type>[] {
  return [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <span className="font-medium">{row.original.name}</span>
      ),
    },
    {
      accessorKey: 'slug',
      header: 'Slug',
      cell: ({ row }) => (
        <code className="bg-muted rounded px-2 py-0.5 font-mono text-xs">
          {row.original.slug}
        </code>
      ),
    },
    {
      id: 'origin',
      header: 'Origin',
      cell: ({ row }) =>
        row.original.is_system ? (
          <Badge variant="secondary">Default</Badge>
        ) : (
          <Badge variant="outline">Custom</Badge>
        ),
    },
    {
      id: 'actions',
      enableHiding: false,
      cell: ({ row }) =>
        row.original.is_system ? null : (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="icon"
              data-testid="delete-type-button"
              aria-label={`Delete ${row.original.name}`}
              onClick={() => {
                onDelete(row.original)
              }}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        ),
    },
  ]
}

/**
 * Client-side dry run of the copy: the endpoint has no preview mode, so the
 * split is derived the way the server derives it — system defaults are never
 * part of the source set, and a source slug the destination already uses is
 * skipped rather than failing the copy (#829).
 *
 * Scope note: the server copies every entry of `copyableResourceTypes()`
 * (`internal/services/type.go`), which today is `artifacts` alone — the same
 * single resource this page lists. Widening that map without widening the
 * preview's source query would make the preview under-report what the copy did.
 */
function buildCopyPreview(
  sourceTypes: Type[],
  destinationTypes: Type[]
): CopyPreview {
  const taken = new Set(
    destinationTypes.map(t => `${t.resource_type}:${t.slug}`)
  )
  const preview: CopyPreview = { willAdd: [], alreadyExists: [] }
  for (const type of sourceTypes) {
    if (type.is_system) continue
    if (taken.has(`${type.resource_type}:${type.slug}`)) {
      preview.alreadyExists.push(type)
    } else {
      preview.willAdd.push(type)
    }
  }
  return preview
}

function CopyPreviewSummary({
  loading,
  failed,
  preview,
}: Readonly<{
  loading: boolean
  failed: boolean
  preview: CopyPreview | null
}>) {
  if (loading) {
    return (
      <p className="text-muted-foreground text-sm" data-testid="copy-preview">
        Checking what this will add…
      </p>
    )
  }
  if (failed) {
    return (
      <p className="text-destructive text-sm" data-testid="copy-preview">
        Couldn&apos;t read that team&apos;s artifact types, so there is nothing
        to preview. Pick the team again to retry.
      </p>
    )
  }
  if (!preview) return null

  const { willAdd, alreadyExists } = preview
  return (
    <div className="space-y-2 text-sm" data-testid="copy-preview">
      <p>
        <span className="font-medium">{willAdd.length}</span> type
        {willAdd.length === 1 ? '' : 's'} will be added
        {alreadyExists.length > 0 && (
          <>
            , <span className="font-medium">{alreadyExists.length}</span>{' '}
            already exist{alreadyExists.length === 1 ? 's' : ''} here and will
            be skipped
          </>
        )}
        .
      </p>
      {willAdd.length > 0 && (
        <ul className="text-muted-foreground list-disc space-y-0.5 pl-5">
          {willAdd.map(type => (
            <li key={type.id} data-testid="copy-preview-add">
              {type.name}
            </li>
          ))}
        </ul>
      )}
      {willAdd.length === 0 && (
        <p className="text-muted-foreground">
          Nothing new to copy from this team.
        </p>
      )}
    </div>
  )
}

/**
 * `team` is the team `TeamScopeLayout` resolved from the URL (#584). Do not
 * reach for `useTeam()` here: on a cold deep-link the ambient team is still the
 * previously persisted one when this page's first effect runs, so it would
 * fetch another team's artifact types under this team's URL.
 */
export function Customization({ team }: Readonly<{ team: Team }>) {
  const { handleError } = useErrorHandler()
  // The team came in as a prop, so the permissions must be read off it — the
  // ambient team's would be the wrong team's on a cold deep-link.
  const { can } = usePermissions(team)
  // `teams` is the membership-filtered list of the user's teams, not the
  // URL-scoped team, so reading it here is safe (unlike `currentTeam`).
  const { teams } = useTeam()

  const [types, setTypes] = useState<Type[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [isCreating, setIsCreating] = useState(false)
  const [typeToDelete, setTypeToDelete] = useState<Type | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [copyOpen, setCopyOpen] = useState(false)
  const [copying, setCopying] = useState(false)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewFailed, setPreviewFailed] = useState(false)
  const [preview, setPreview] = useState<CopyPreview | null>(null)
  // Bumped on every source change so a slow response for a previously selected
  // team cannot overwrite the preview of the current one.
  const previewSeq = useRef(0)

  const teamId = team.id

  const loadTypes = useCallback(async () => {
    try {
      setIsLoading(true)
      const result = await typeService.getTypes(teamId, ARTIFACTS_RESOURCE)
      setTypes(result)
    } catch (error) {
      handleError(error, 'Failed to load artifact types')
      setTypes([])
    } finally {
      setIsLoading(false)
    }
  }, [teamId, handleError])

  useEffect(() => {
    void loadTypes()
  }, [loadTypes])

  const handleCreate = async (
    values: CreateTypeFormValues,
    setFieldError: (field: 'name' | 'slug', message: string) => void
  ) => {
    try {
      setIsCreating(true)
      const request: CreateTypeRequest = {
        resource_type: ARTIFACTS_RESOURCE,
        slug: values.slug.trim(),
        name: values.name.trim(),
      }
      await typeService.createType(teamId, request)
      toast.success('Artifact type created')
      setCreateOpen(false)
      await loadTypes()
    } catch (error) {
      const errors = handleError(error, 'Failed to create artifact type')
      Object.entries(errors).forEach(([field, message]) => {
        if (field === 'name' || field === 'slug') {
          setFieldError(field, message)
        }
      })
    } finally {
      setIsCreating(false)
    }
  }

  const handleSourceChange = async (sourceTeam: Team | null) => {
    const seq = ++previewSeq.current
    setPreview(null)
    setPreviewFailed(false)
    if (!sourceTeam) {
      setPreviewLoading(false)
      return
    }
    try {
      setPreviewLoading(true)
      const sourceTypes = await typeService.getTypes(
        sourceTeam.id,
        ARTIFACTS_RESOURCE
      )
      if (seq !== previewSeq.current) return
      setPreview(buildCopyPreview(sourceTypes, types))
    } catch (error) {
      if (seq !== previewSeq.current) return
      setPreviewFailed(true)
      handleError(error, "Failed to load the other team's artifact types")
    } finally {
      if (seq === previewSeq.current) setPreviewLoading(false)
    }
  }

  const handleCopy = async (sourceTeam: Team) => {
    try {
      setCopying(true)
      const result = await typeService.copyTypesFromTeam(teamId, sourceTeam.id)
      toast.success(
        `Copied ${String(result.added_count)} artifact type${
          result.added_count === 1 ? '' : 's'
        } from ${sourceTeam.name}` +
          (result.skipped_count > 0
            ? ` (${String(result.skipped_count)} already existed)`
            : '')
      )
      setCopyOpen(false)
      await loadTypes()
    } catch (error) {
      handleError(error, 'Failed to copy artifact types')
    } finally {
      setCopying(false)
    }
  }

  const handleDelete = async () => {
    if (!typeToDelete) return
    try {
      setDeleting(true)
      await typeService.deleteType(teamId, typeToDelete.id)
      toast.success('Artifact type deleted')
      await loadTypes()
    } catch (error) {
      handleError(error, 'Failed to delete artifact type')
    } finally {
      setDeleting(false)
      setTypeToDelete(null)
    }
  }

  const columns = buildColumns(setTypeToDelete)

  // The endpoint authorizes the copy on membership of both teams alone (#829):
  // it has no cell in the authz matrix, and `resource.create` is the published
  // permission every role holds, so gating on it mirrors the server exactly
  // rather than hiding the action from members who may in fact perform it.
  // Never gate on `team.role` — the matrix lives on the server.
  // With no other team to copy from there is nothing to offer.
  const canCopy =
    can('resource.create') && teams.some(other => other.id !== teamId)

  const renderTypes = () => {
    if (types.length === 0) {
      return (
        <EmptyState
          icon={Shapes}
          title="No artifact types"
          description="Create your first custom type to organize artifacts your way."
          actions={
            <Button
              onClick={() => {
                setCreateOpen(true)
              }}
            >
              <Plus className="mr-2 size-4" />
              Create type
            </Button>
          }
        />
      )
    }
    return (
      <Card>
        <CardContent className="p-4">
          <DataTable
            columns={columns}
            data={types}
            rowTestId={() => 'type-item'}
          />
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-8">
      <PageHeader
        title="Customization"
        description="Tailor VibeXP to how your team works."
        actions={
          canCopy ? (
            <Button
              variant="outline"
              data-testid="copy-types-button"
              onClick={() => {
                setCopyOpen(true)
              }}
            >
              <Copy className="mr-2 size-4" />
              Copy from another team…
            </Button>
          ) : undefined
        }
      />

      <section className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold">Artifact types</h2>
            <p className="text-muted-foreground text-sm">
              Categories your team can assign to artifacts. The built-in
              defaults can&apos;t be removed.
            </p>
          </div>
          <Button
            data-testid="create-type-button"
            onClick={() => {
              setCreateOpen(true)
            }}
          >
            <Plus className="mr-2 size-4" />
            Create type
          </Button>
        </div>

        {isLoading ? (
          <Card>
            <CardContent className="space-y-3 p-6">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </CardContent>
          </Card>
        ) : (
          renderTypes()
        )}
      </section>

      <CreateTypeDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        submitting={isCreating}
        onSubmit={handleCreate}
      />

      <CopyFromTeamDialog
        open={copyOpen}
        onOpenChange={setCopyOpen}
        team={team}
        title="Copy artifact types"
        description="Bring another team's custom artifact types into this one. The copy is a snapshot — editing them here won't affect the other team."
        submitting={copying}
        onConfirm={handleCopy}
        onSourceChange={sourceTeam => {
          void handleSourceChange(sourceTeam)
        }}
        preview={
          <CopyPreviewSummary
            loading={previewLoading}
            failed={previewFailed}
            preview={preview}
          />
        }
        // The copy is only confirmable once the user has actually seen what it
        // will do — #833 requires the preview *before* confirming.
        confirmDisabled={previewLoading || !preview}
        confirmLabel="Copy types"
      />

      <ConfirmDialog
        open={!!typeToDelete}
        onOpenChange={open => {
          if (!open) setTypeToDelete(null)
        }}
        title="Delete artifact type?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {typeToDelete?.name ?? 'this type'}
            </span>
            . Any artifacts using it will be moved to{' '}
            <span className="font-medium">General</span>.
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </div>
  )
}
