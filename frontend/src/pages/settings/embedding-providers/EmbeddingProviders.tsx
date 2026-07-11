import type { ColumnDef } from '@tanstack/react-table'
import {
  AlertCircle,
  Cpu,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { ListTable } from '@/components/patterns/list-page'
import { StatusBadge } from '@/components/StatusBadge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import { EmbeddingProviderDialog } from '@/pages/settings/embedding-providers/EmbeddingProviderDialog'
import type {
  CreateEmbeddingProviderRequest,
  EmbeddingCoverageItem,
  EmbeddingCoverageResponse,
  EmbeddingProviderResponse,
  UpdateEmbeddingProviderRequest,
} from '@/services/embeddingProviderService'
import { embeddingProviderService } from '@/services/embeddingProviderService'
import { getErrorMessage } from '@/utils/errorHandling'

function formatDate(value: string) {
  return new Date(value).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const ENTITY_TYPE_LABELS: Record<EmbeddingCoverageItem['entity_type'], string> =
  {
    prompt: 'Prompts',
    artifact: 'Artifacts',
    memory: 'Memories',
    blueprint: 'Blueprints',
    feed_item: 'Feed items',
  }

function entityTypeLabel(type: EmbeddingCoverageItem['entity_type']) {
  return ENTITY_TYPE_LABELS[type]
}

// Percentage guarded against a zero denominator so N=0 renders 0%, never NaN.
function percent(embedded: number, total: number) {
  if (total <= 0) return 0
  return Math.round((embedded / total) * 100)
}

function buildProviderColumns(
  onEdit: (provider: EmbeddingProviderResponse) => void,
  onDelete: (provider: EmbeddingProviderResponse) => void
): ColumnDef<EmbeddingProviderResponse>[] {
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

interface StatCardProps {
  label: string
  value: string
  hint?: string
}

function StatCard({ label, value, hint }: StatCardProps) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground text-sm font-medium">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold">{value}</p>
        {hint && <p className="text-muted-foreground mt-1 text-xs">{hint}</p>}
      </CardContent>
    </Card>
  )
}

function EmbeddingCoverageCards({
  coverage,
}: {
  coverage: EmbeddingCoverageResponse
}) {
  const totals = coverage.coverage.reduce(
    (acc, item) => ({
      total: acc.total + item.total,
      embedded: acc.embedded + item.embedded,
      pending: acc.pending + item.pending,
    }),
    { total: 0, embedded: 0, pending: 0 }
  )
  const overallPercent = percent(totals.embedded, totals.total)

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
        <StatCard
          label="Embedded"
          value={totals.embedded.toLocaleString()}
          hint={`of ${totals.total.toLocaleString()} items`}
        />
        <StatCard
          label="Pending"
          value={totals.pending.toLocaleString()}
          hint="waiting for an embedding"
        />
        <StatCard label="% embedded" value={`${String(overallPercent)}%`} />
      </div>

      <div>
        <h3 className="text-muted-foreground mb-2 text-sm font-medium">
          By type
        </h3>
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-5">
          {coverage.coverage.map(item => (
            <StatCard
              key={item.entity_type}
              label={entityTypeLabel(item.entity_type)}
              value={`${String(item.embedded_percent)}%`}
              hint={`${item.embedded.toLocaleString()} / ${item.total.toLocaleString()} · ${item.pending.toLocaleString()} pending`}
            />
          ))}
        </div>
      </div>

      <p className="text-muted-foreground text-xs">
        Pending is the number of items still waiting for an embedding. If it
        isn&rsquo;t going down over time, embedding may be stuck &mdash; use
        &ldquo;Reprocess pending&rdquo; to re-drive it.
      </p>
    </div>
  )
}

interface CoverageSectionProps {
  coverage: EmbeddingCoverageResponse | null
  coverageLoading: boolean
  coverageError: string | null
  canReprocess: boolean
  reprocessing: boolean
  onReprocess: () => void
  canClear: boolean
  clearing: boolean
  onClear: () => void
}

// Coverage summary plus its two maintenance actions (reprocess missing, clear
// all). Extracted from EmbeddingProviders so the page component stays within the
// max-lines-per-function budget.
function CoverageSection({
  coverage,
  coverageLoading,
  coverageError,
  canReprocess,
  reprocessing,
  onReprocess,
  canClear,
  clearing,
  onClear,
}: CoverageSectionProps) {
  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">Embedding coverage</h2>
          <p className="text-muted-foreground text-sm">
            {coverage?.has_active_provider && coverage.active_model
              ? `Measured against ${coverage.active_model}.`
              : 'Embedding status across your content.'}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={!canReprocess || reprocessing}
            onClick={onReprocess}
          >
            {reprocessing ? (
              <Loader2 className="mr-2 size-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 size-4" />
            )}
            Reprocess pending
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="text-destructive hover:text-destructive"
            disabled={!canClear || clearing}
            onClick={onClear}
          >
            {clearing ? (
              <Loader2 className="mr-2 size-4 animate-spin" />
            ) : (
              <Trash2 className="mr-2 size-4" />
            )}
            Clear all embeddings
          </Button>
        </div>
      </div>

      {coverageLoading ? (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full" />
          ))}
        </div>
      ) : coverageError ? (
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Couldn&rsquo;t load embedding coverage</AlertTitle>
          <AlertDescription>{coverageError}</AlertDescription>
        </Alert>
      ) : coverage ? (
        <EmbeddingCoverageCards coverage={coverage} />
      ) : null}
    </div>
  )
}

export function EmbeddingProviders() {
  const { handleError } = useErrorHandler()
  const { currentTeam } = useTeam()

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

  const [coverage, setCoverage] = useState<EmbeddingCoverageResponse | null>(
    null
  )
  const [coverageLoading, setCoverageLoading] = useState(true)
  const [coverageError, setCoverageError] = useState<string | null>(null)
  const [reprocessing, setReprocessing] = useState(false)
  const [clearOpen, setClearOpen] = useState(false)
  const [clearing, setClearing] = useState(false)

  const loadProviders = useCallback(async () => {
    if (!currentTeam) return
    try {
      setLoading(true)
      const data = await embeddingProviderService.getEmbeddingProviders(
        currentTeam.id
      )
      setProviders(data)
    } catch (error) {
      handleError(error, 'Failed to load embedding providers')
      setProviders([])
    } finally {
      setLoading(false)
    }
  }, [handleError, currentTeam])

  // Coverage failures surface inline (Alert) rather than as a toast so a status
  // hiccup never blanks the providers table below.
  const loadCoverage = useCallback(async () => {
    if (!currentTeam) return
    try {
      setCoverageLoading(true)
      setCoverageError(null)
      const data = await embeddingProviderService.getEmbeddingCoverage(
        currentTeam.id
      )
      setCoverage(data)
    } catch (error) {
      setCoverage(null)
      setCoverageError(getErrorMessage(error))
    } finally {
      setCoverageLoading(false)
    }
  }, [currentTeam])

  useEffect(() => {
    void loadProviders()
  }, [loadProviders])

  useEffect(() => {
    void loadCoverage()
  }, [loadCoverage])

  const handleSubmit = async (
    data: CreateEmbeddingProviderRequest | UpdateEmbeddingProviderRequest
  ) => {
    if (!currentTeam) return
    try {
      setSubmitting(true)
      if (editing) {
        await embeddingProviderService.updateEmbeddingProvider(
          currentTeam.id,
          editing.id,
          data as UpdateEmbeddingProviderRequest
        )
        toast.success('Provider updated')
      } else {
        await embeddingProviderService.createEmbeddingProvider(
          currentTeam.id,
          data as CreateEmbeddingProviderRequest
        )
        toast.success('Provider created')
      }
      setDialogOpen(false)
      setEditing(undefined)
      await loadProviders()
      await loadCoverage()
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
      await embeddingProviderService.deleteEmbeddingProvider(
        currentTeam.id,
        toDelete.id
      )
      toast.success('Provider deleted')
      await loadProviders()
      await loadCoverage()
    } catch (error) {
      handleError(error, 'Failed to delete provider')
    } finally {
      setDeleting(false)
      setToDelete(null)
    }
  }

  const defaultProviderId = providers.find(p => p.is_default)?.id
  const canReprocess =
    !!coverage?.has_active_provider && !!defaultProviderId && !coverageLoading

  const handleReprocess = async () => {
    if (!currentTeam || !defaultProviderId) return
    try {
      setReprocessing(true)
      await embeddingProviderService.reprocessEmbeddingProvider(
        currentTeam.id,
        defaultProviderId
      )
      toast.success('Reprocessing started', {
        description:
          'Missing embeddings are being regenerated in the background.',
      })
      await loadCoverage()
    } catch (error) {
      handleError(error, 'Failed to start reprocessing')
    } finally {
      setReprocessing(false)
    }
  }

  // Clearing is allowed whenever there is something embedded to remove; it is a
  // team-wide truncate, so it does not depend on an active provider.
  const embeddedTotal =
    coverage?.coverage.reduce((sum, item) => sum + item.embedded, 0) ?? 0
  const canClear = embeddedTotal > 0 && !coverageLoading

  const handleClearEmbeddings = async () => {
    if (!currentTeam) return
    try {
      setClearing(true)
      const { deleted_count } = await embeddingProviderService.clearEmbeddings(
        currentTeam.id
      )
      toast.success('Embeddings cleared', {
        description: `Removed ${deleted_count.toLocaleString()} embedding${
          deleted_count === 1 ? '' : 's'
        }. Content stays unembedded until you reprocess.`,
      })
      await loadCoverage()
    } catch (error) {
      handleError(error, 'Failed to clear embeddings')
    } finally {
      setClearing(false)
      setClearOpen(false)
    }
  }

  const columns = useMemo<ColumnDef<EmbeddingProviderResponse>[]>(
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

      {!loading && providers.length > 0 && (
        <CoverageSection
          coverage={coverage}
          coverageLoading={coverageLoading}
          coverageError={coverageError}
          canReprocess={canReprocess}
          reprocessing={reprocessing}
          onReprocess={() => {
            void handleReprocess()
          }}
          canClear={canClear}
          clearing={clearing}
          onClear={() => {
            setClearOpen(true)
          }}
        />
      )}

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
            . Anything using it for embeddings will stop working.
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />

      <ConfirmDialog
        open={clearOpen}
        onOpenChange={open => {
          if (!clearing) setClearOpen(open)
        }}
        title="Clear all embeddings?"
        description={
          <>
            This permanently deletes{' '}
            <span className="font-medium">every stored embedding</span> for this
            team. Semantic search will return nothing until you re-embed with
            &ldquo;Reprocess pending&rdquo;. This can&rsquo;t be undone.
          </>
        }
        confirmLabel="Clear all"
        variant="destructive"
        loading={clearing}
        onConfirm={handleClearEmbeddings}
      />
    </div>
  )
}
