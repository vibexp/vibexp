import type { ColumnDef } from '@tanstack/react-table'
import { Bot, Pencil, Plus, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { ListTable } from '@/components/patterns/list-page'
import { StatusBadge } from '@/components/StatusBadge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import { ModelProviderDialog } from '@/pages/settings/model-providers/ModelProviderDialog'
import type {
  CreateModelProviderRequest,
  ModelProviderResponse,
  UpdateModelProviderRequest,
} from '@/services/modelProviderService'
import { modelProviderService } from '@/services/modelProviderService'

function formatDate(value: string) {
  return new Date(value).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function buildProviderColumns(
  onEdit: (provider: ModelProviderResponse) => void,
  onDelete: (provider: ModelProviderResponse) => void
): ColumnDef<ModelProviderResponse>[] {
  return [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">{row.original.name}</span>
          {row.original.is_default && (
            <StatusBadge tone="success">Default</StatusBadge>
          )}
        </div>
      ),
    },
    {
      accessorKey: 'provider_type',
      header: 'Type',
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {row.original.provider_type}
        </span>
      ),
    },
    {
      accessorKey: 'model',
      header: 'Model',
      cell: ({ row }) => <span className="text-sm">{row.original.model}</span>,
    },
    {
      id: 'base_url',
      header: 'Base URL',
      cell: ({ row }) =>
        row.original.base_url ? (
          <code className="bg-muted rounded px-2 py-0.5 font-mono text-xs">
            {row.original.base_url}
          </code>
        ) : (
          <span className="text-muted-foreground text-xs">—</span>
        ),
    },
    {
      id: 'api_key',
      header: 'API key',
      cell: ({ row }) =>
        row.original.has_api_key ? (
          <StatusBadge tone="success">Set</StatusBadge>
        ) : (
          <StatusBadge tone="warning">Not set</StatusBadge>
        ),
    },
    {
      accessorKey: 'updated_at',
      header: 'Updated',
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {formatDate(row.original.updated_at)}
        </span>
      ),
    },
    {
      id: 'actions',
      cell: ({ row }) => (
        <div className="flex justify-end gap-1">
          <Button
            variant="ghost"
            size="icon"
            aria-label="Edit"
            onClick={() => {
              onEdit(row.original)
            }}
          >
            <Pencil className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Delete"
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

export function ModelProviders() {
  const { handleError } = useErrorHandler()
  const { currentTeam } = useTeam()

  const [providers, setProviders] = useState<ModelProviderResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<ModelProviderResponse | undefined>()
  const [submitting, setSubmitting] = useState(false)
  const [toDelete, setToDelete] = useState<ModelProviderResponse | null>(null)
  const [deleting, setDeleting] = useState(false)

  const loadProviders = useCallback(async () => {
    if (!currentTeam) return
    try {
      setLoading(true)
      const data = await modelProviderService.getModelProviders(currentTeam.id)
      setProviders(data)
    } catch (error) {
      handleError(error, 'Failed to load model providers')
      setProviders([])
    } finally {
      setLoading(false)
    }
  }, [handleError, currentTeam])

  useEffect(() => {
    void loadProviders()
  }, [loadProviders])

  const handleSubmit = async (
    data: CreateModelProviderRequest | UpdateModelProviderRequest
  ) => {
    if (!currentTeam) return
    try {
      setSubmitting(true)
      if (editing) {
        await modelProviderService.updateModelProvider(
          currentTeam.id,
          editing.id,
          data as UpdateModelProviderRequest
        )
        toast.success('Provider updated')
      } else {
        await modelProviderService.createModelProvider(
          currentTeam.id,
          data as CreateModelProviderRequest
        )
        toast.success('Provider created')
      }
      setDialogOpen(false)
      setEditing(undefined)
      await loadProviders()
    } catch (error) {
      handleError(
        error,
        editing ? 'Failed to update provider' : 'Failed to create provider'
      )
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async () => {
    if (!toDelete || !currentTeam) return
    try {
      setDeleting(true)
      await modelProviderService.deleteModelProvider(
        currentTeam.id,
        toDelete.id
      )
      toast.success('Provider deleted')
      await loadProviders()
    } catch (error) {
      handleError(error, 'Failed to delete provider')
    } finally {
      setDeleting(false)
      setToDelete(null)
    }
  }

  const columns = useMemo<ColumnDef<ModelProviderResponse>[]>(
    () =>
      buildProviderColumns(
        provider => {
          setEditing(provider)
          setDialogOpen(true)
        },
        provider => {
          setToDelete(provider)
        }
      ),
    []
  )

  return (
    <div className="space-y-6">
      <PageHeader
        title="Model Providers"
        description="Configure OpenAI-compatible LLM providers for your AI applications."
        actions={
          <Button
            onClick={() => {
              setEditing(undefined)
              setDialogOpen(true)
            }}
          >
            <Plus className="mr-2 size-4" />
            Add provider
          </Button>
        }
      />

      {loading && (
        <Card>
          <CardContent className="space-y-3 p-6">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </CardContent>
        </Card>
      )}
      {!loading && providers.length === 0 && (
        <EmptyState
          icon={Bot}
          title="No model providers yet"
          description="Add your first provider to point VibeXP at your own model backend."
          actions={
            <Button
              onClick={() => {
                setEditing(undefined)
                setDialogOpen(true)
              }}
            >
              <Plus className="mr-2 size-4" />
              Add provider
            </Button>
          }
        />
      )}
      {!loading && providers.length > 0 && (
        <Card>
          <CardContent className="p-4">
            <ListTable rows={providers} columns={columns} />
          </CardContent>
        </Card>
      )}

      <ModelProviderDialog
        teamId={currentTeam?.id ?? ''}
        open={dialogOpen}
        onOpenChange={open => {
          setDialogOpen(open)
          if (!open) setEditing(undefined)
        }}
        provider={editing}
        submitting={submitting}
        onSubmit={handleSubmit}
      />

      <ConfirmDialog
        open={!!toDelete}
        onOpenChange={open => {
          if (!open) setToDelete(null)
        }}
        title="Delete provider?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {toDelete?.name ?? 'this provider'}
            </span>
            {'. Anything using it as a model backend will stop working.'}
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
