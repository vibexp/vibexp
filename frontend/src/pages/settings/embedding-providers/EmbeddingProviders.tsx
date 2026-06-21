import type { ColumnDef } from '@tanstack/react-table'
import { Cpu, Pencil, Plus, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { ListTable } from '@/components/patterns/list-page'
import { StatusBadge } from '@/components/StatusBadge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import { EmbeddingProviderDialog } from '@/pages/settings/embedding-providers/EmbeddingProviderDialog'
import { embeddingProviderService } from '@/services/embeddingProviderService'
import type {
  CreateEmbeddingProviderRequest,
  EmbeddingProviderResponse,
  UpdateEmbeddingProviderRequest,
} from '@/types'

function formatDate(value: string) {
  return new Date(value).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function EmbeddingProviders() {
  const { handleError } = useErrorHandler()

  const [providers, setProviders] = useState<EmbeddingProviderResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<
    EmbeddingProviderResponse | undefined
  >()
  const [submitting, setSubmitting] = useState(false)
  const [toDelete, setToDelete] = useState<EmbeddingProviderResponse | null>(
    null
  )
  const [deleting, setDeleting] = useState(false)

  const loadProviders = useCallback(async () => {
    try {
      setLoading(true)
      const data = await embeddingProviderService.getEmbeddingProviders()
      setProviders(data)
    } catch (error) {
      handleError(error, 'Failed to load embedding providers')
      setProviders([])
    } finally {
      setLoading(false)
    }
  }, [handleError])

  useEffect(() => {
    void loadProviders()
  }, [loadProviders])

  const handleSubmit = async (
    data: CreateEmbeddingProviderRequest | UpdateEmbeddingProviderRequest
  ) => {
    try {
      setSubmitting(true)
      if (editing) {
        await embeddingProviderService.updateEmbeddingProvider(editing.id, data)
        toast.success('Provider updated')
      } else {
        await embeddingProviderService.createEmbeddingProvider(
          data as CreateEmbeddingProviderRequest
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
    if (!toDelete) return
    try {
      setDeleting(true)
      await embeddingProviderService.deleteEmbeddingProvider(toDelete.id)
      toast.success('Provider deleted')
      await loadProviders()
    } catch (error) {
      handleError(error, 'Failed to delete provider')
    } finally {
      setDeleting(false)
      setToDelete(null)
    }
  }

  const columns = useMemo<ColumnDef<EmbeddingProviderResponse>[]>(
    () => [
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
                setEditing(row.original)
                setDialogOpen(true)
              }}
            >
              <Pencil className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Delete"
              onClick={() => {
                setToDelete(row.original)
              }}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        ),
      },
    ],
    []
  )

  return (
    <div className="space-y-6">
      <PageHeader
        title="Embedding Providers"
        description="Configure providers used for vector embeddings and semantic search."
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

      {loading ? (
        <Card>
          <CardContent className="space-y-3 p-6">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : providers.length === 0 ? (
        <EmptyState
          icon={Cpu}
          title="No embedding providers yet"
          description="Add your first provider to start generating vector embeddings."
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
      ) : (
        <Card>
          <CardContent className="p-4">
            <ListTable rows={providers} columns={columns} />
          </CardContent>
        </Card>
      )}

      <EmbeddingProviderDialog
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
            . Anything using it for embeddings will stop working.
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
