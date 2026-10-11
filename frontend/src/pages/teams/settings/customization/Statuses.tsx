import type { ColumnDef } from '@tanstack/react-table'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { DataTable } from '@/components/DataTable'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useStatuses } from '@/hooks/useStatuses'
import { toast } from '@/lib/toast'
import type { StatusDefinition } from '@/services/statusService'
import { STATUS_DELETE_FALLBACK, statusService } from '@/services/statusService'
import type { Team } from '@/services/teamService'
import { ApiError } from '@/types/errors'

import {
  StatusDialog,
  type StatusFormErrorTarget,
  type StatusFormValues,
} from './StatusDialog'
import {
  countLabel,
  STATUS_RESOURCE_TYPES,
  type StatusResourceTypeLabel,
} from './statusResourceTypes'

// A team has at most 20 custom statuses per resource type on top of a handful
// of system ones, so one page always holds the whole list.
const PAGE_SIZE = 30

const SKELETON_ROWS = ['r1', 'r2', 'r3', 'r4']

type SetFormError = (target: StatusFormErrorTarget, message: string) => void

interface RowActions {
  onRename: (status: StatusDefinition) => void
  onDelete: (status: StatusDefinition) => void
}

function buildColumns(
  actions: RowActions | null
): ColumnDef<StatusDefinition>[] {
  const columns: ColumnDef<StatusDefinition>[] = [
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
          <Badge variant="secondary">System</Badge>
        ) : (
          <Badge variant="outline">Custom</Badge>
        ),
    },
    {
      accessorKey: 'usage_count',
      header: 'In use',
      cell: ({ row }) => (
        <span className="tabular-nums" data-testid="status-usage">
          {row.original.usage_count}
        </span>
      ),
    },
  ]
  // Without the permission there is no actions column at all, rather than an
  // empty one.
  if (!actions) return columns
  columns.push({
    id: 'actions',
    enableHiding: false,
    cell: ({ row }) =>
      // A system status is built in: it can be neither renamed nor deleted,
      // whatever the caller may do.
      row.original.is_system ? null : (
        <div className="flex justify-end gap-1">
          <Button
            variant="ghost"
            size="icon"
            data-testid="rename-status-button"
            aria-label={`Rename ${row.original.name}`}
            onClick={() => {
              actions.onRename(row.original)
            }}
          >
            <Pencil className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            data-testid="delete-status-button"
            aria-label={`Delete ${row.original.name}`}
            onClick={() => {
              actions.onDelete(row.original)
            }}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      ),
  })
  return columns
}

/**
 * Puts a failed write where the user can act on it: a 409 (reserved or
 * duplicate slug) on the slug field, any other 400 (the 20-status limit, or a
 * rule this form's copy of the validation missed) above the form — each in the
 * server's own words. Returns false for anything else, which the caller toasts.
 */
function showWriteError(error: unknown, setError: SetFormError): boolean {
  if (!(error instanceof ApiError)) return false
  if (error.status === 409) {
    setError('slug', error.getMessage())
    return true
  }
  if (error.status === 400) {
    setError('root', error.getMessage())
    return true
  }
  return false
}

function deletedMessage(
  reassignedCount: number,
  destination: string,
  type: StatusResourceTypeLabel
): string {
  if (reassignedCount === 0) return 'Status deleted'
  return `Status deleted. ${countLabel(reassignedCount, type)} moved to ${destination}`
}

function StatusPanel({
  team,
  type,
  canManage,
}: Readonly<{
  team: Team
  type: StatusResourceTypeLabel
  canManage: boolean
}>) {
  const { handleError } = useErrorHandler()
  const teamId = team.id
  const { statuses, isLoading, error, reload } = useStatuses(teamId, type.value)

  const [createOpen, setCreateOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<StatusDefinition | null>(
    null
  )
  const [deleteTarget, setDeleteTarget] = useState<StatusDefinition | null>(
    null
  )
  const [submitting, setSubmitting] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const fallbackSlug = STATUS_DELETE_FALLBACK[type.value]
  // The destination by its display name; the slug stands in if the list ever
  // arrives without that system status.
  const statusName = (slug: string) =>
    statuses.find(status => status.slug === slug)?.name ?? slug

  // The row as the latest list has it: the usage count is refetched when the
  // delete dialog opens, and the dialog must show the fresh number.
  const deleteRow =
    statuses.find(status => status.id === deleteTarget?.id) ?? deleteTarget

  const handleCreate = async (
    values: StatusFormValues,
    setError: SetFormError
  ) => {
    try {
      setSubmitting(true)
      await statusService.create(teamId, {
        resource_type: type.value,
        slug: values.slug.trim(),
        name: values.name.trim(),
      })
      toast.success('Status created')
      setCreateOpen(false)
      reload()
    } catch (err) {
      if (!showWriteError(err, setError)) {
        handleError(err, 'Failed to create status')
      }
    } finally {
      setSubmitting(false)
    }
  }

  const handleRename = async (
    values: StatusFormValues,
    setError: SetFormError
  ) => {
    if (!renameTarget?.id) return
    try {
      setSubmitting(true)
      await statusService.rename(teamId, renameTarget.id, values.name.trim())
      toast.success('Status renamed')
      setRenameTarget(null)
      reload()
    } catch (err) {
      if (!showWriteError(err, setError)) {
        handleError(err, 'Failed to rename status')
        // Nothing the form can correct — typically a 404 for a status someone
        // else just deleted. Close it and show the list as it now is.
        setRenameTarget(null)
        reload()
      }
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget?.id) return
    try {
      setDeleting(true)
      const result = await statusService.remove(teamId, deleteTarget.id)
      toast.success(
        deletedMessage(
          result.reassigned_count,
          statusName(result.reassigned_to),
          type
        )
      )
    } catch (err) {
      handleError(err, 'Failed to delete status')
    } finally {
      setDeleting(false)
      setDeleteTarget(null)
      // Also after a failure: a 404 means the status is already gone.
      reload()
    }
  }

  const columns = buildColumns(
    canManage
      ? {
          onRename: setRenameTarget,
          onDelete: status => {
            setDeleteTarget(status)
            // The count on screen may be minutes old; refresh it for the
            // confirmation.
            reload()
          },
        }
      : null
  )

  const renderBody = () => {
    if (isLoading) {
      return (
        <Card data-testid="statuses-loading">
          <CardContent className="space-y-3 p-6">
            {SKELETON_ROWS.map(key => (
              <Skeleton key={key} className="h-10 w-full" />
            ))}
          </CardContent>
        </Card>
      )
    }
    if (error) {
      return (
        <Alert variant="destructive" data-testid="statuses-error">
          <AlertTitle>
            Couldn&apos;t load the {type.singular} statuses
          </AlertTitle>
          <AlertDescription className="space-y-3">
            <p>
              {error instanceof Error ? error.message : 'Something went wrong.'}
            </p>
            <Button variant="outline" size="sm" onClick={reload}>
              Try again
            </Button>
          </AlertDescription>
        </Alert>
      )
    }
    return (
      <Card>
        <CardContent className="p-4">
          <DataTable
            columns={columns}
            data={statuses}
            pageSize={PAGE_SIZE}
            rowTestId={() => 'status-item'}
          />
        </CardContent>
      </Card>
    )
  }

  const usage = deleteRow?.usage_count ?? 0

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <p className="text-muted-foreground text-sm">
          The statuses a {type.singular} can have. System statuses are built in
          and can&apos;t be renamed or removed.
        </p>
        {canManage && (
          <Button
            data-testid="create-status-button"
            onClick={() => {
              setCreateOpen(true)
            }}
          >
            <Plus className="mr-2 size-4" />
            Create status
          </Button>
        )}
      </div>

      {renderBody()}

      <StatusDialog
        mode="create"
        open={createOpen}
        onOpenChange={setCreateOpen}
        resourceLabel={type.singular}
        submitting={submitting}
        onSubmit={handleCreate}
      />

      <StatusDialog
        mode="rename"
        open={!!renameTarget}
        onOpenChange={open => {
          if (!open) setRenameTarget(null)
        }}
        resourceLabel={type.singular}
        status={renameTarget}
        submitting={submitting}
        onSubmit={handleRename}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={open => {
          if (!open) setDeleteTarget(null)
        }}
        title="Delete status?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {deleteRow?.name ?? 'this status'}
            </span>
            .{' '}
            {usage === 0 ? (
              <>No {type.plural} use it, so nothing will move.</>
            ) : (
              <>
                {countLabel(usage, type)} will move to{' '}
                <span className="font-medium">{statusName(fallbackSlug)}</span>.
              </>
            )}
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </section>
  )
}

/**
 * Settings › Statuses — the lifecycle a team's resources can move through.
 *
 * Every member can read the list (the endpoint is open to members); creating,
 * renaming and deleting need `team.settings.update`, read from the server's
 * `permissions` array and never from `role`.
 *
 * `team` is the team `TeamScopeLayout` resolved from the URL (#584). Do not
 * reach for `useTeam()` here: on a cold deep-link the ambient team is still the
 * previously persisted one when this page's first effect runs.
 *
 * One tab per resource type. Radix unmounts inactive `TabsContent`, so each
 * list is fetched when its tab is first opened.
 */
export function Statuses({ team }: Readonly<{ team: Team }>) {
  const { can } = usePermissions(team)
  const canManage = can('team.settings.update')

  return (
    <div className="space-y-6">
      <PageHeader
        title="Statuses"
        description="Add your own statuses to the built-in ones, per resource type."
      />

      <Tabs defaultValue={STATUS_RESOURCE_TYPES[0].value} className="space-y-6">
        <TabsList>
          {STATUS_RESOURCE_TYPES.map(type => (
            <TabsTrigger key={type.value} value={type.value}>
              {type.label}
            </TabsTrigger>
          ))}
        </TabsList>

        {STATUS_RESOURCE_TYPES.map(type => (
          <TabsContent key={type.value} value={type.value}>
            <StatusPanel team={team} type={type} canManage={canManage} />
          </TabsContent>
        ))}
      </Tabs>
    </div>
  )
}
