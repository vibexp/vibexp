import type { ReactNode } from 'react'
import { useRef, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CopyFromTeamDialog } from '@/components/settings/CopyFromTeamDialog'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import { canCopyEmbeddingProviderFrom } from '@/pages/teams/settings/embedding-providers/copyPermissions'
import { EmbeddingProviderDialog } from '@/pages/teams/settings/embedding-providers/EmbeddingProviderDialog'
import type {
  CopySource,
  CopySubmitValues,
} from '@/pages/teams/settings/embedding-providers/embeddingProviderForm'
import type {
  EmbeddingProviderCopyActivation,
  EmbeddingProviderResponse,
} from '@/services/embeddingProviderService'
import { embeddingProviderService } from '@/services/embeddingProviderService'
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
  providers: EmbeddingProviderResponse[]
  selectedId: string | null
  onSelect: (provider: EmbeddingProviderResponse) => void
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
        Couldn&apos;t read that team&apos;s embedding providers. Pick the team
        again to retry.
      </p>
    )
  }
  if (providers.length === 0) {
    return (
      <p className="text-muted-foreground text-sm" data-testid="copy-preview">
        That team has no embedding providers to copy.
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
            name="copy-source-embedding-provider"
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

/** The server's activation verdict, plus the copy it is about. */
interface CopyActivationState {
  activation: EmbeddingProviderCopyActivation
  providerId: string
  providerName: string
}

function activationDescription(
  state: CopyActivationState,
  teamName: string
): ReactNode {
  const { activation, providerName } = state
  const displaced = activation.displaced_model
  const count = activation.displaced_embedded_resources
  return (
    <>
      <span className="font-medium">{providerName}</span> is now{' '}
      <span className="font-medium">{teamName}</span>&apos;s active embedding
      provider
      {displaced ? (
        <>
          , displacing <span className="font-medium">{displaced}</span>
        </>
      ) : null}
      .{' '}
      {count > 0 ? (
        <>
          {count.toLocaleString()} resource{count === 1 ? '' : 's'} embedded
          with <span className="font-medium">{displaced}</span> will stop
          matching new queries until they are re-embedded.
        </>
      ) : (
        'Nothing is embedded with the previous model, so no existing vectors are affected.'
      )}{' '}
      {activation.reprocess_enqueued
        ? `A background re-embed is already running${
            activation.embeddings_wiped
              ? ', and the stale vectors were cleared first'
              : ''
          }. Semantic search falls back to keyword matching until it completes.`
        : 'Re-embedding regenerates them in the background; semantic search falls back to keyword matching until it completes.'}
    </>
  )
}

/**
 * The activation warning (#835). Driven off the SERVER's verdict from #831 —
 * never re-derived from `is_default`, which is exactly the reasoning that misses
 * the recency case: a copy always lands non-default, yet the active provider
 * resolves to "the default one, else the most recently updated one", so a copy
 * silently takes over whenever the destination has no default set.
 *
 * When the copy did not ask for a re-embed, the confirm IS the remedy.
 */
function CopyActivationDialog({
  state,
  teamName,
  reembedding,
  onConfirm,
  onDismiss,
}: Readonly<{
  state: CopyActivationState | null
  teamName: string
  reembedding: boolean
  onConfirm: () => Promise<void>
  onDismiss: () => void
}>) {
  if (!state) return null
  const needsReembed = !state.activation.reprocess_enqueued
  return (
    <ConfirmDialog
      open
      onOpenChange={open => {
        if (!open && !reembedding) onDismiss()
      }}
      title={
        needsReembed
          ? `Re-embed ${teamName}'s resources?`
          : `${teamName}'s search now uses the copied provider`
      }
      description={activationDescription(state, teamName)}
      confirmLabel={needsReembed ? 'Re-embed now' : 'Got it'}
      cancelLabel={needsReembed ? 'Not now' : 'Close'}
      variant={needsReembed ? 'destructive' : 'default'}
      loading={reembedding}
      onConfirm={onConfirm}
    />
  )
}

interface Props {
  /**
   * The DESTINATION team, as `TeamScopeLayout` resolved it from the URL — never
   * the ambient `useTeam().currentTeam`, which on a cold deep-link is still the
   * previously persisted team (#584).
   */
  team: Team
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Called after a successful copy so the page can refresh what it shows. */
  onCopied: () => Promise<void>
}

/**
 * The whole "copy an embedding provider in from another team" flow (#835):
 * source team → source provider → a pre-filled `EmbeddingProviderDialog` in copy
 * mode → the server's activation verdict.
 *
 * Owned by its own component rather than the page so `EmbeddingProviders` stays
 * within the eslint `max-lines-per-function` budget.
 */
export function CopyEmbeddingProviderFlow({
  team,
  open,
  onOpenChange,
  onCopied,
}: Readonly<Props>) {
  const { handleError } = useErrorHandler()
  const teamId = team.id

  const [sourceProviders, setSourceProviders] = useState<
    EmbeddingProviderResponse[]
  >([])
  const [sourceLoading, setSourceLoading] = useState(false)
  const [sourceFailed, setSourceFailed] = useState(false)
  const [selectedSource, setSelectedSource] =
    useState<EmbeddingProviderResponse | null>(null)
  // Set once the source team + provider are chosen; puts the provider dialog
  // into copy mode and carries what the copy request needs.
  const [copyTarget, setCopyTarget] = useState<
    (CopySource & { sourceTeamId: string }) | null
  >(null)
  const [submitting, setSubmitting] = useState(false)
  const [activationState, setActivationState] =
    useState<CopyActivationState | null>(null)
  const [reembedding, setReembedding] = useState(false)
  // Bumped on every source change so a slow response for a previously selected
  // team cannot overwrite the list of the current one.
  const sourceSeq = useRef(0)

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
      const result = await embeddingProviderService.getEmbeddingProviders(
        sourceTeam.id
      )
      if (seq !== sourceSeq.current) return
      setSourceProviders(result)
    } catch (error) {
      if (seq !== sourceSeq.current) return
      setSourceFailed(true)
      handleError(error, "Failed to load the other team's embedding providers")
    } finally {
      if (seq === sourceSeq.current) setSourceLoading(false)
    }
  }

  // The shared dialog confirms a source TEAM; the provider was picked in the
  // preview slot. Confirming hands off to the provider dialog in copy mode, so
  // the user reviews (and may override) the pre-filled values before the copy.
  const handleSourceConfirmed = (sourceTeam: Team) => {
    if (!selectedSource) return
    setCopyTarget({
      provider: selectedSource,
      sourceTeamName: sourceTeam.name,
      sourceTeamId: sourceTeam.id,
    })
    onOpenChange(false)
  }

  const handleCopySubmit = async (data: CopySubmitValues) => {
    if (!copyTarget) return
    try {
      setSubmitting(true)
      // Only the source identifiers and the (possibly edited) overrides go up:
      // the API key is deliberately absent, the server carries the source row's
      // ciphertext across without ever decrypting it.
      const result =
        await embeddingProviderService.copyEmbeddingProviderFromTeam(teamId, {
          source_team_id: copyTarget.sourceTeamId,
          source_provider_id: copyTarget.provider.id,
          ...data,
        })
      toast.success(`Provider copied from ${copyTarget.sourceTeamName}`)
      setCopyTarget(null)
      // The verdict is the server's, and it is the ONLY thing that decides
      // whether a warning is shown: `becomes_active` already accounts for the
      // recency rule a client-side `is_default` check would miss.
      if (result.activation.becomes_active) {
        setActivationState({
          activation: result.activation,
          providerId: result.provider.id,
          providerName: result.provider.name,
        })
      }
      await onCopied()
    } catch (error) {
      handleError(error, 'Failed to copy provider')
    } finally {
      setSubmitting(false)
    }
  }

  const handleReembed = async () => {
    const state = activationState
    if (!state) return
    if (!state.activation.reprocess_enqueued) {
      try {
        setReembedding(true)
        await embeddingProviderService.reprocessEmbeddingProvider(
          teamId,
          state.providerId
        )
        toast.success('Re-embedding started', {
          description:
            "This team's resources are being re-embedded in the background.",
        })
        await onCopied()
      } catch (error) {
        handleError(error, 'Failed to start re-embedding')
        return
      } finally {
        setReembedding(false)
      }
    }
    setActivationState(null)
  }

  return (
    <>
      <CopyFromTeamDialog
        open={open}
        onOpenChange={onOpenChange}
        team={team}
        title="Copy an embedding provider"
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
        canCopyFrom={canCopyEmbeddingProviderFrom}
        confirmLabel="Continue"
      />

      <EmbeddingProviderDialog
        teamId={teamId}
        teamName={team.name}
        open={!!copyTarget}
        onOpenChange={(dialogOpen: boolean) => {
          if (!dialogOpen) setCopyTarget(null)
        }}
        copySource={copyTarget ?? undefined}
        submitting={submitting}
        onCopySubmit={handleCopySubmit}
      />

      <CopyActivationDialog
        state={activationState}
        teamName={team.name}
        reembedding={reembedding}
        onConfirm={handleReembed}
        onDismiss={() => {
          setActivationState(null)
        }}
      />
    </>
  )
}
