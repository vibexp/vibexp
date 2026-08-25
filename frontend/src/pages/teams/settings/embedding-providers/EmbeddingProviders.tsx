import type { ColumnDef } from '@tanstack/react-table'
import { Copy, Cpu, Pencil, Plus, Trash2 } from 'lucide-react'
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
import { usePermissions } from '@/hooks/usePermissions'
import { toast } from '@/lib/toast'
import { CopyEmbeddingProviderFlow } from '@/pages/teams/settings/embedding-providers/CopyEmbeddingProviderFlow'
import { canCopyEmbeddingProviderFrom } from '@/pages/teams/settings/embedding-providers/copyPermissions'
import { CoverageSection } from '@/pages/teams/settings/embedding-providers/EmbeddingCoverageSection'
import { EmbeddingProviderDialog } from '@/pages/teams/settings/embedding-providers/EmbeddingProviderDialog'
import type {
  CreateEmbeddingProviderRequest,
  EmbeddingCoverageResponse,
  EmbeddingProviderResponse,
  UpdateEmbeddingProviderRequest,
} from '@/services/embeddingProviderService'
import { embeddingProviderService } from '@/services/embeddingProviderService'
import type { Team } from '@/services/teamService'
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

// The page's two destructive confirmations (delete a provider, clear every
// embedding). Extracted so the page component stays under the
// max-lines-per-function cap.
function DestructiveConfirmDialogs({
  toDelete,
  deleting,
  onCancelDelete,
  onConfirmDelete,
  clearOpen,
  clearing,
  onClearOpenChange,
  onConfirmClear,
}: Readonly<{
  toDelete: EmbeddingProviderResponse | null
  deleting: boolean
  onCancelDelete: () => void
  onConfirmDelete: () => Promise<void>
  clearOpen: boolean
  clearing: boolean
  onClearOpenChange: (open: boolean) => void
  onConfirmClear: () => Promise<void>
}>) {
  return (
    <>
      <ConfirmDialog
        open={!!toDelete}
        onOpenChange={open => {
          if (!open) onCancelDelete()
        }}
        title="Delete provider?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {toDelete?.name ?? 'this provider'}
            </span>
            {'. Anything using it for embeddings will stop working.'}
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={onConfirmDelete}
      />

      <ConfirmDialog
        open={clearOpen}
        onOpenChange={open => {
          if (!clearing) onClearOpenChange(open)
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
        onConfirm={onConfirmClear}
      />
    </>
  )
}

/**
 * `team` is the team `TeamScopeLayout` resolved from the URL (#584). Do not
 * reach for `useTeam()` here: React fires child effects before parent effects,
 * so on a cold deep-link this page's load effects run BEFORE the layout's
 * `setCurrentTeam` sync and the ambient team is still the previously persisted
 * one — it would fetch and briefly render another team's providers and coverage
 * under this team's URL. The layout guarantees a team inside the scope, which
 * is why there are no `!team` guards below.
 */
export function EmbeddingProviders({ team }: Readonly<{ team: Team }>) {
  const { handleError } = useErrorHandler()
  // Permissions must be read off the team prop, not the ambient one — on a cold
  // deep-link they would be a different team's (#584).
  const { can } = usePermissions(team)
  // `teams` is the membership-filtered list of the user's teams rather than the
  // URL-scoped team, so it carries no cold-deep-link staleness.
  const { teams } = useTeam()
  const teamId = team.id

  const [copyOpen, setCopyOpen] = useState(false)
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
    try {
      setLoading(true)
      const data = await embeddingProviderService.getEmbeddingProviders(teamId)
      setProviders(data)
    } catch (error) {
      handleError(error, 'Failed to load embedding providers')
      setProviders([])
    } finally {
      setLoading(false)
    }
  }, [handleError, teamId])

  // Coverage failures surface inline (Alert) rather than as a toast so a status
  // hiccup never blanks the providers table below.
  const loadCoverage = useCallback(async () => {
    try {
      setCoverageLoading(true)
      setCoverageError(null)
      const data = await embeddingProviderService.getEmbeddingCoverage(teamId)
      setCoverage(data)
    } catch (error) {
      setCoverage(null)
      setCoverageError(getErrorMessage(error))
    } finally {
      setCoverageLoading(false)
    }
  }, [teamId])

  useEffect(() => {
    void loadProviders()
  }, [loadProviders])

  useEffect(() => {
    void loadCoverage()
  }, [loadCoverage])

  const handleSubmit = async (
    data: CreateEmbeddingProviderRequest | UpdateEmbeddingProviderRequest
  ) => {
    try {
      setSubmitting(true)
      if (editing) {
        await embeddingProviderService.updateEmbeddingProvider(
          teamId,
          editing.id,
          data as UpdateEmbeddingProviderRequest
        )
        toast.success('Provider updated')
      } else {
        await embeddingProviderService.createEmbeddingProvider(
          teamId,
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
    if (!toDelete) return
    try {
      setDeleting(true)
      await embeddingProviderService.deleteEmbeddingProvider(
        teamId,
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
    if (!defaultProviderId) return
    try {
      setReprocessing(true)
      await embeddingProviderService.reprocessEmbeddingProvider(
        teamId,
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
    try {
      setClearing(true)
      const { deleted_count } =
        await embeddingProviderService.clearEmbeddings(teamId)
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

  const canCopy =
    can('team.update') &&
    teams.some(
      other => other.id !== teamId && canCopyEmbeddingProviderFrom(other)
    )

  // Both call sites can be on screen at once (the header action is always
  // rendered, the empty-state one whenever the list is empty), so they carry
  // distinct test ids rather than a duplicated one.
  const copyButton = (testId: string) => (
    <Button
      variant="outline"
      data-testid={testId}
      onClick={() => {
        setCopyOpen(true)
      }}
    >
      <Copy className="mr-2 size-4" />
      Copy from…
    </Button>
  )

  const providersContent =
    providers.length === 0 ? (
      <EmptyState
        icon={Cpu}
        title="No embedding providers yet"
        description="Add your first provider to start generating vector embeddings."
        actions={
          <div className="flex flex-wrap justify-center gap-2">
            <Button
              onClick={() => {
                setEditing(undefined)
                setDialogOpen(true)
              }}
            >
              <Plus className="mr-2 size-4" />
              Add provider
            </Button>
            {canCopy && copyButton('copy-embedding-provider-button-empty')}
          </div>
        }
      />
    ) : (
      <Card>
        <CardContent className="p-4">
          <ListTable rows={providers} columns={columns} />
        </CardContent>
      </Card>
    )

  return (
    <div className="space-y-6">
      <PageHeader
        title="Embedding Providers"
        description="Configure providers used for vector embeddings and semantic search."
        actions={
          <div className="flex flex-wrap gap-2">
            {canCopy && copyButton('copy-embedding-provider-button')}
            <Button
              onClick={() => {
                setEditing(undefined)
                setDialogOpen(true)
              }}
            >
              <Plus className="mr-2 size-4" />
              Add provider
            </Button>
          </div>
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
      ) : (
        providersContent
      )}

      <EmbeddingProviderDialog
        teamId={teamId}
        teamName={team.name}
        open={dialogOpen}
        onOpenChange={open => {
          setDialogOpen(open)
          if (!open) setEditing(undefined)
        }}
        provider={editing}
        submitting={submitting}
        onSubmit={handleSubmit}
      />

      <CopyEmbeddingProviderFlow
        team={team}
        open={copyOpen}
        onOpenChange={setCopyOpen}
        onCopied={async () => {
          await loadProviders()
          await loadCoverage()
        }}
      />

      <DestructiveConfirmDialogs
        toDelete={toDelete}
        deleting={deleting}
        onCancelDelete={() => {
          setToDelete(null)
        }}
        onConfirmDelete={handleDelete}
        clearOpen={clearOpen}
        clearing={clearing}
        onClearOpenChange={setClearOpen}
        onConfirmClear={handleClearEmbeddings}
      />
    </div>
  )
}
