import { AlertCircle, ArrowLeft, Pencil, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import {
  type ReadingAction,
  ResourceBody,
  useBodyViewMode,
  useCopyAction,
} from '@/components/patterns/reading-page'
import {
  resourceRegistry,
  useStatusOptions,
} from '@/components/patterns/resource'
import { ResourceReadingPage } from '@/components/resource-detail/ResourceReadingPage'
import { useResourceLock } from '@/components/resource-lock/useResourceLock'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics, usePromptRenderer } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceVersions } from '@/hooks/useResourceVersions'
import {
  buildProjectEditUrl,
  buildResourceVersionsUrl,
} from '@/lib/resourceUrl'
import {
  PromptMetadata,
  promptUsedBySection,
} from '@/pages/prompts/PromptDetailSidebar'
import type {
  Prompt,
  PromptDependenciesResponse,
} from '@/services/promptService'
import { promptService } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

import { SharedBadge } from './SharedBadge'

/**
 * The server is asked to render with NO values: it expands the @references and
 * leaves every {{placeholder}} in place. The values are then substituted on
 * screen by the markdown renderer, which is what keeps an unfilled placeholder
 * visible as a chip (an empty value sent to the server would erase it) and
 * makes typing a value update the body without a round trip.
 */
const NO_PLACEHOLDER_VALUES: Record<string, string> = {}

/**
 * The prompt's rendered-mode-only affordances — the placeholder inputs whose
 * values are substituted into the body, and the render error when one comes
 * back.
 * Slotted into `ResourceBody`'s `renderedExtra`, which is what keeps that
 * shared component domain-free (#901).
 */
function PromptRenderedExtra({
  allPlaceholders,
  placeholderValues,
  updatePlaceholderValue,
  renderError,
}: Readonly<{
  allPlaceholders: string[]
  placeholderValues: Record<string, string>
  updatePlaceholderValue: (placeholder: string, value: string) => void
  renderError: string | null
}>) {
  return (
    <>
      {allPlaceholders.length > 0 && (
        <div className="bg-muted/40 space-y-2 rounded-md border p-3">
          <div className="text-xs font-medium">Placeholders</div>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {allPlaceholders.map(ph => (
              <div key={ph} className="space-y-1">
                <label className="text-muted-foreground text-xs font-medium">
                  {ph}
                </label>
                <Input
                  value={placeholderValues[ph] ?? ''}
                  onChange={e => {
                    updatePlaceholderValue(ph, e.target.value)
                  }}
                  placeholder={`Enter ${ph}`}
                />
              </div>
            ))}
          </div>
        </div>
      )}
      {renderError && (
        <Alert variant="destructive">
          <AlertTitle>Render error</AlertTitle>
          <AlertDescription>{renderError}</AlertDescription>
        </Alert>
      )}
    </>
  )
}

export function PromptDetail() {
  const { slug } = useParams<{ slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const statusOptions = useStatusOptions(resourceRegistry.prompt)
  const { canDeleteResource } = usePermissions()
  const { showSuccess, showError } = useAlerts()
  const { trackEvent } = useAnalytics()
  const { handleError } = useErrorHandler()

  const [prompt, setPrompt] = useState<Prompt | null>(null)
  const [loading, setLoading] = useState(true)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [dependencies, setDependencies] =
    useState<PromptDependenciesResponse | null>(null)
  const [loadingDependencies, setLoadingDependencies] = useState(false)
  // Owned here rather than by `ResourceBody` because the render and
  // placeholder effects below key off it; persisted under the same shared
  // key so the choice follows the reader across resources (#901).
  const [tab, setTab] = useBodyViewMode()

  const {
    renderedBody,
    renderError,
    isRendering,
    allPlaceholders,
    placeholderValues,
    isLoadingPlaceholders,
    renderPrompt,
    fetchPlaceholders,
    updatePlaceholderValue,
  } = usePromptRenderer()

  const loadPrompt = useCallback(async () => {
    if (!slug) return
    if (!currentTeam) {
      showError('No team selected')
      void navigate('/prompts')
      return
    }
    try {
      setLoading(true)
      const p = await promptService.getPrompt(currentTeam.id, slug)
      setPrompt(p)
      trackEvent({
        event: ANALYTICS_EVENTS.PROMPT_PREVIEW_VIEWED,
        properties: {
          prompt_id: p.slug,
          prompt_title: p.name,
          prompt_type: p.status,
          action_context: 'view',
        },
      })
      try {
        setLoadingDependencies(true)
        const deps = await promptService.getPromptDependencies(
          currentTeam.id,
          slug
        )
        setDependencies(deps)
      } catch {
        // non-critical
      } finally {
        setLoadingDependencies(false)
      }
    } catch (error) {
      handleError(error, 'Failed to load prompt')
      void navigate('/prompts')
    } finally {
      setLoading(false)
    }
  }, [slug, currentTeam, showError, navigate, trackEvent, handleError])

  useEffect(() => {
    if (!slug || isLoadingTeam) return
    void loadPrompt()
  }, [slug, isLoadingTeam, loadPrompt])

  // Version history powers the Metadata panel's footer link + count chip.
  // Best-effort, and stale-guarded by the hook — `loadPrompt` fetched it
  // unguarded, so a slow response for a previously viewed slug could land
  // afterwards and render that prompt's version count (#905).
  const { versionHistory } = useResourceVersions({
    loadVersions:
      !isLoadingTeam && currentTeam && slug
        ? () => promptService.getPromptVersions(currentTeam.id, slug)
        : null,
    // Built as the edit page builds it (#1180), so the two links cannot drift.
    to:
      (prompt &&
        buildResourceVersionsUrl({
          type: 'prompt',
          id: prompt.id,
          slug: prompt.slug,
        })) ??
      undefined,
    editedAt: prompt?.updated_at,
    deps: [isLoadingTeam, currentTeam?.id, slug],
  })

  const loadedRef = useRef<string | null>(null)

  useEffect(() => {
    if (!prompt?.slug || tab !== 'rendered' || !currentTeam) return
    if (loadedRef.current !== prompt.slug) {
      loadedRef.current = prompt.slug
      void fetchPlaceholders(prompt.slug, currentTeam.id).then(() => {
        setTimeout(() => {
          void renderPrompt(prompt.slug, currentTeam.id, NO_PLACEHOLDER_VALUES)
        }, 100)
      })
    } else if (!isLoadingPlaceholders) {
      void renderPrompt(prompt.slug, currentTeam.id, NO_PLACEHOLDER_VALUES)
    }
  }, [
    prompt?.slug,
    tab,
    currentTeam,
    fetchPlaceholders,
    renderPrompt,
    isLoadingPlaceholders,
  ])

  // Lock / Unlock, the lock dialog and the lock history (epic #1285).
  const resourceLock = useResourceLock<Prompt>({
    kind: 'prompt',
    resource: prompt,
    ops:
      currentTeam && prompt
        ? {
            lock: comment =>
              promptService.lockPrompt(currentTeam.id, prompt.slug, comment),
            unlock: () =>
              promptService.unlockPrompt(currentTeam.id, prompt.slug),
            reload: () => promptService.getPrompt(currentTeam.id, prompt.slug),
            loadHistory: () =>
              promptService.getPromptLockHistory(currentTeam.id, prompt.slug),
          }
        : null,
    onChanged: setPrompt,
  })

  const handleDelete = async () => {
    if (!prompt || !currentTeam) return
    try {
      setDeleting(true)
      await promptService.deletePrompt(currentTeam.id, prompt.slug)
      showSuccess('Prompt deleted successfully', 'Success')
      void navigate('/prompts')
    } catch (error) {
      handleError(error, 'Failed to delete prompt')
    } finally {
      setDeleting(false)
      setDeleteOpen(false)
    }
  }

  // Copy the SOURCE body, not the placeholder-rendered output — the raw view
  // shows the same text, and it is what a reader pastes into a tool.
  const copyAction = useCopyAction(prompt?.body ?? '')

  const backAction: ReadingAction = {
    id: 'back',
    label: 'Back',
    icon: ArrowLeft,
    onClick: () => {
      void navigate('/prompts')
    },
  }

  if (loading) {
    return (
      <ResourceReadingPage title="Loading prompt…">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ResourceReadingPage>
    )
  }

  if (!prompt) {
    return (
      <ResourceReadingPage title="Prompt not found" actions={[backAction]}>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Prompt not found</AlertTitle>
          <AlertDescription>
            The prompt could not be loaded. It may have been deleted.
          </AlertDescription>
        </Alert>
      </ResourceReadingPage>
    )
  }

  const actions: ReadingAction[] = [
    backAction,
    copyAction,
    {
      id: 'edit',
      label: 'Edit',
      icon: Pencil,
      testId: 'edit-prompt-button',
      onClick: () => {
        // Editor still lives in v1 until Slice 5b
        void navigate(`/prompts/${prompt.slug}/edit`)
      },
    },
  ]
  actions.push(...resourceLock.actions)
  // A locked prompt cannot be deleted; Delete returns after unlock.
  if (canDeleteResource(prompt.user_id) && !prompt.lock) {
    actions.push({
      id: 'delete',
      label: 'Delete',
      icon: Trash2,
      tone: 'destructive',
      testId: 'delete-prompt-button',
      onClick: () => {
        setDeleteOpen(true)
      },
    })
  }

  return (
    <>
      <ResourceReadingPage
        title={prompt.name}
        status={{
          value: statusOptions.labelOf(prompt.status),
          tone: statusOptions.toneOf(prompt.status),
        }}
        address={{ label: 'Slug', value: prompt.slug }}
        updatedAt={prompt.updated_at}
        summary={prompt.description}
        headerExtra={prompt.is_shared && <SharedBadge />}
        actions={actions}
        resource={
          currentTeam
            ? { kind: 'prompt', id: prompt.id, teamId: currentTeam.id }
            : undefined
        }
        metadata={
          <PromptMetadata
            prompt={prompt}
            versionHistory={versionHistory}
            project={prompt.project}
            projectHref={p => buildProjectEditUrl(currentTeam?.id, p.slug)}
          />
        }
        extraSections={[
          ...promptUsedBySection(dependencies, loadingDependencies),
          ...resourceLock.historySection,
        ]}
      >
        <ResourceBody
          content={renderedBody !== '' ? renderedBody : prompt.body}
          rawContent={prompt.body}
          placeholders={placeholderValues}
          mode={tab}
          onModeChange={setTab}
          isLoading={isRendering || isLoadingPlaceholders}
          renderedExtra={
            <PromptRenderedExtra
              allPlaceholders={allPlaceholders}
              placeholderValues={placeholderValues}
              updatePlaceholderValue={updatePlaceholderValue}
              renderError={renderError}
            />
          }
        />
      </ResourceReadingPage>

      {resourceLock.dialog}

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete prompt?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">{prompt.name}</span>. This action
            cannot be undone.
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </>
  )
}
