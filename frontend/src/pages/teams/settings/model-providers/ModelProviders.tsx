import type { ColumnDef } from '@tanstack/react-table'
import { Bot, Copy, Pencil, Plus, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { ListTable } from '@/components/patterns/list-page'
import { CopyFromTeamDialog } from '@/components/settings/CopyFromTeamDialog'
import { StatusBadge } from '@/components/StatusBadge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { toast } from '@/lib/toast'
import type { CopySource } from '@/pages/teams/settings/model-providers/ModelProviderDialog'
import { ModelProviderDialog } from '@/pages/teams/settings/model-providers/ModelProviderDialog'
import type {
  CreateModelProviderRequest,
  ModelProviderResponse,
  UpdateModelProviderRequest,
} from '@/services/modelProviderService'
import { modelProviderService } from '@/services/modelProviderService'
import type { Team } from '@/services/teamService'

/**
 * The copy endpoint moves exactly ONE provider, so picking the source team is
 * only half the choice — this is the second half, rendered in the shared
 * dialog's page-owned preview slot (#833). It doubles as the preview: the user
 * sees precisely which row will be copied before confirming.
 */
function SourceProviderPicker({
  loading,
  failed,
  providers,
  selectedId,
  onSelect,
}: Readonly<{
  loading: boolean
  failed: boolean
  providers: ModelProviderResponse[]
  selectedId: string | null
  onSelect: (provider: ModelProviderResponse) => void
}>) {
  if (loading) {
    return (
      <p className="text-muted-foreground text-sm" data-testid="copy-preview">
        Loading that team&apos;s providers…
      </p>
    )
  }
  if (failed) {
    return (
      <p className="text-destructive text-sm" data-testid="copy-preview">
        Couldn&apos;t read that team&apos;s model providers. Pick the team again
        to retry.
      </p>
    )
  }
  if (providers.length === 0) {
    return (
      <p className="text-muted-foreground text-sm" data-testid="copy-preview">
        That team has no model providers to copy.
      </p>
    )
  }
  return (
    <fieldset className="space-y-2" data-testid="copy-preview">
      <legend className="mb-2 text-sm font-medium">Provider to copy</legend>
      {providers.map(candidate => (
        <label
          key={candidate.id}
          className="hover:bg-muted/50 flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm"
        >
          <input
            type="radio"
            name="copy-source-provider"
            className="mt-1"
            data-testid="copy-source-provider"
            checked={selectedId === candidate.id}
            onChange={() => {
              onSelect(candidate)
            }}
          />
          <span className="space-y-0.5">
            <span className="block font-medium">{candidate.name}</span>
            <span className="text-muted-foreground block text-xs">
              {candidate.model}
              {candidate.base_url ? ` · ${candidate.base_url}` : ''}
            </span>
            {!candidate.has_api_key && (
              <span className="text-muted-foreground block text-xs">
                No API key stored
              </span>
            )}
          </span>
        </label>
      ))}
    </fieldset>
  )
}

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

/**
 * `team` is the team `TeamScopeLayout` resolved from the URL (#584). Do not
 * reach for `useTeam()` here: React fires child effects before parent effects,
 * so on a cold deep-link this page's load effect runs BEFORE the layout's
 * `setCurrentTeam` sync and the ambient team is still the previously persisted
 * one — it would fetch and briefly render another team's providers under this
 * team's URL. The layout guarantees a team inside the scope, which is why there
 * are no `!team` guards below.
 */
export function ModelProviders({ team }: Readonly<{ team: Team }>) {
  const { handleError } = useErrorHandler()
  // Permissions must be read off the team prop, not the ambient one — on a
  // cold deep-link they would be a different team's (#584).
  const { can } = usePermissions(team)
  // `teams` is the membership-filtered list of the user's teams rather than the
  // URL-scoped team, so it carries no cold-deep-link staleness.
  const { teams } = useTeam()
  const teamId = team.id

  const [providers, setProviders] = useState<ModelProviderResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<ModelProviderResponse | undefined>()
  const [submitting, setSubmitting] = useState(false)
  const [toDelete, setToDelete] = useState<ModelProviderResponse | null>(null)
  const [deleting, setDeleting] = useState(false)

  const [copyOpen, setCopyOpen] = useState(false)
  const [sourceProviders, setSourceProviders] = useState<
    ModelProviderResponse[]
  >([])
  const [sourceLoading, setSourceLoading] = useState(false)
  const [sourceFailed, setSourceFailed] = useState(false)
  const [selectedSource, setSelectedSource] =
    useState<ModelProviderResponse | null>(null)
  // Set once the source team + provider are chosen; puts the provider dialog
  // into copy mode and carries what the copy request needs.
  const [copyTarget, setCopyTarget] = useState<
    (CopySource & { sourceTeamId: string }) | null
  >(null)
  // Bumped on every source change so a slow response for a previously selected
  // team cannot overwrite the list of the current one.
  const sourceSeq = useRef(0)

  const loadProviders = useCallback(async () => {
    try {
      setLoading(true)
      const data = await modelProviderService.getModelProviders(teamId)
      setProviders(data)
    } catch (error) {
      handleError(error, 'Failed to load model providers')
      setProviders([])
    } finally {
      setLoading(false)
    }
  }, [handleError, teamId])

  useEffect(() => {
    void loadProviders()
  }, [loadProviders])

  const handleSourceChange = async (sourceTeam: Team | null) => {
    const seq = ++sourceSeq.current
    setSourceProviders([])
    setSelectedSource(null)
    setSourceFailed(false)
    if (!sourceTeam) {
      setSourceLoading(false)
      return
    }
    try {
      setSourceLoading(true)
      const result = await modelProviderService.getModelProviders(sourceTeam.id)
      if (seq !== sourceSeq.current) return
      setSourceProviders(result)
    } catch (error) {
      if (seq !== sourceSeq.current) return
      setSourceFailed(true)
      handleError(error, "Failed to load the other team's model providers")
    } finally {
      if (seq === sourceSeq.current) setSourceLoading(false)
    }
  }

  // The shared dialog confirms a source TEAM; the provider was picked in the
  // preview slot. Confirming hands off to the provider dialog in copy mode, so
  // the user reviews (and may override) the pre-filled values before the copy
  // is actually made.
  const handleSourceConfirmed = (sourceTeam: Team) => {
    if (!selectedSource) return
    setCopyTarget({
      provider: selectedSource,
      sourceTeamName: sourceTeam.name,
      sourceTeamId: sourceTeam.id,
    })
    setCopyOpen(false)
    setEditing(undefined)
    setDialogOpen(true)
  }

  const handleCopySubmit = async (data: CreateModelProviderRequest) => {
    if (!copyTarget) return
    try {
      setSubmitting(true)
      // Only the source identifiers and the (possibly edited) overrides go up:
      // the API key is deliberately absent, the server carries the source row's
      // ciphertext across without ever decrypting it.
      await modelProviderService.copyModelProviderFromTeam(teamId, {
        source_team_id: copyTarget.sourceTeamId,
        source_provider_id: copyTarget.provider.id,
        name: data.name,
        provider_type: data.provider_type,
        model: data.model,
        base_url: data.base_url,
      })
      toast.success(`Provider copied from ${copyTarget.sourceTeamName}`)
      setDialogOpen(false)
      setCopyTarget(null)
      await loadProviders()
    } catch (error) {
      handleError(error, 'Failed to copy provider')
    } finally {
      setSubmitting(false)
    }
  }

  const handleSubmit = async (
    data: CreateModelProviderRequest | UpdateModelProviderRequest
  ) => {
    if (copyTarget) {
      await handleCopySubmit(data as CreateModelProviderRequest)
      return
    }
    try {
      setSubmitting(true)
      if (editing) {
        await modelProviderService.updateModelProvider(
          teamId,
          editing.id,
          data as UpdateModelProviderRequest
        )
        toast.success('Provider updated')
      } else {
        await modelProviderService.createModelProvider(
          teamId,
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
    if (!toDelete) return
    try {
      setDeleting(true)
      await modelProviderService.deleteModelProvider(teamId, toDelete.id)
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

  // The copy endpoint authorizes `team.update` on BOTH teams (#830), so the
  // action is only offered when the destination grants it and at least one
  // other team the user belongs to does too. Never gate on `team.role` — the
  // matrix lives on the server and is published as `permissions` (#224).
  const canCopyFrom = (candidate: Team) =>
    candidate.permissions.includes('team.update')
  const canCopy =
    can('team.update') &&
    teams.some(other => other.id !== teamId && canCopyFrom(other))

  const openCopyDialog = () => {
    setSelectedSource(null)
    setSourceProviders([])
    setSourceFailed(false)
    setCopyOpen(true)
  }

  // Both call sites can be on screen at once (the header action is always
  // rendered, the empty-state one whenever the list is empty), so they carry
  // distinct test ids rather than a duplicated one.
  const copyButton = (testId: string) => (
    <Button variant="outline" data-testid={testId} onClick={openCopyDialog}>
      <Copy className="mr-2 size-4" />
      Copy from…
    </Button>
  )

  return (
    <div className="space-y-6">
      <PageHeader
        title="Model Providers"
        description="Configure OpenAI-compatible LLM providers for your AI applications."
        actions={
          <div className="flex flex-wrap gap-2">
            {canCopy && copyButton('copy-model-provider-button')}
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
              {canCopy && copyButton('copy-model-provider-button-empty')}
            </div>
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

      <CopyFromTeamDialog
        open={copyOpen}
        onOpenChange={setCopyOpen}
        team={team}
        title="Copy a model provider"
        description="Bring a provider over from one of your other teams, credential included. The copy is a snapshot — editing it here won't affect the other team."
        submitting={false}
        onConfirm={handleSourceConfirmed}
        onSourceChange={sourceTeam => {
          void handleSourceChange(sourceTeam)
        }}
        preview={
          <SourceProviderPicker
            loading={sourceLoading}
            failed={sourceFailed}
            providers={sourceProviders}
            selectedId={selectedSource?.id ?? null}
            onSelect={setSelectedSource}
          />
        }
        // Picking the team is only half the choice — the copy moves exactly one
        // provider, so it cannot be confirmed until that one is chosen.
        confirmDisabled={sourceLoading || !selectedSource}
        canCopyFrom={canCopyFrom}
        confirmLabel="Continue"
      />

      <ModelProviderDialog
        teamId={teamId}
        open={dialogOpen}
        onOpenChange={open => {
          setDialogOpen(open)
          if (!open) {
            setEditing(undefined)
            setCopyTarget(null)
          }
        }}
        provider={editing}
        copySource={copyTarget ?? undefined}
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
