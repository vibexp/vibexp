import { AlertCircle, ArrowLeft, Pencil, Share2, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { type VersionHistoryMeta } from '@/components/metadata/MetadataPanel'
import {
  type ReadingAction,
  ResourceBody,
  useBodyViewMode,
  useCopyAction,
} from '@/components/patterns/reading-page'
import { statusTone } from '@/components/patterns/resource'
import { ResourceReadingPage } from '@/components/resource-detail/ResourceReadingPage'
import { StatusBadge } from '@/components/StatusBadge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics, usePromptRenderer } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceProject } from '@/hooks/useResourceProject'
import { buildProjectEditUrl } from '@/lib/resourceUrl'
import {
  PromptMetadata,
  promptUsedBySection,
} from '@/pages/prompts/PromptDetailSidebar'
import type {
  Prompt,
  PromptDependenciesResponse,
  PromptVersion,
} from '@/services/promptService'
import { promptService } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

function formatDate(value: string) {
  return new Date(value).toLocaleString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function getRelativeTime(value: string): string {
  const date = new Date(value)
  const now = new Date()
  const diff = Math.floor((now.getTime() - date.getTime()) / 1000)
  if (diff < 60) return `${String(diff)}s ago`
  if (diff < 3600) return `${String(Math.floor(diff / 60))}m ago`
  if (diff < 86400) return `${String(Math.floor(diff / 3600))}h ago`
  if (diff < 2592000) return `${String(Math.floor(diff / 86400))}d ago`
  return formatDate(value)
}

// Build the Metadata panel's version-history affordance. Snapshots capture the *prior*
// body and version numbers are monotonic, so the live prompt's content version is one
// past the highest retained snapshot number; `versions.length` is the count shown on the
// linked history page. Returns undefined when there are no snapshots (nothing to link to).
function buildPromptVersionHistory(
  prompt: Prompt,
  versions: PromptVersion[]
): VersionHistoryMeta | undefined {
  if (versions.length === 0) return undefined
  const latestVersionNumber = versions.reduce(
    (max, v) => Math.max(max, v.version_number),
    0
  )
  return {
    count: versions.length,
    currentVersion: latestVersionNumber + 1,
    editedAt: prompt.updated_at,
    to: `/prompts/${encodeURIComponent(prompt.slug)}/versions`,
  }
}

/**
 * The prompt's rendered-mode-only affordances — the placeholder inputs whose
 * values drive the server render, and the render error when one comes back.
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
  const [versions, setVersions] = useState<PromptVersion[]>([])
  // Supplemental — the Project metadata row needs the project's name, and the
  // prompt payload carries only its id.
  const project = useResourceProject(currentTeam?.id, prompt?.project_id)
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
      // Version history powers the Metadata panel's footer link + count chip.
      // Best-effort: a failure here must not break the prompt view itself.
      try {
        const history = await promptService.getPromptVersions(
          currentTeam.id,
          slug
        )
        setVersions(history.versions)
      } catch {
        setVersions([])
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

  const loadedRef = useRef<string | null>(null)
  const prevValuesRef = useRef<Record<string, string>>({})

  useEffect(() => {
    if (!prompt?.slug || tab !== 'rendered' || !currentTeam) return
    if (loadedRef.current !== prompt.slug) {
      loadedRef.current = prompt.slug
      void fetchPlaceholders(prompt.slug, currentTeam.id).then(() => {
        setTimeout(() => {
          void renderPrompt(prompt.slug, currentTeam.id)
        }, 100)
      })
    } else if (!isLoadingPlaceholders) {
      void renderPrompt(prompt.slug, currentTeam.id)
    }
  }, [
    prompt?.slug,
    tab,
    currentTeam,
    fetchPlaceholders,
    renderPrompt,
    isLoadingPlaceholders,
  ])

  useEffect(() => {
    const prev = prevValuesRef.current
    const changed = Object.keys(placeholderValues).some(
      key => placeholderValues[key] !== prev[key]
    )
    if (
      !changed ||
      !prompt?.slug ||
      !currentTeam ||
      tab !== 'rendered' ||
      isLoadingPlaceholders ||
      loadedRef.current !== prompt.slug ||
      allPlaceholders.length === 0
    ) {
      prevValuesRef.current = placeholderValues
      return
    }
    const t = setTimeout(() => {
      void renderPrompt(prompt.slug, currentTeam.id)
      prevValuesRef.current = placeholderValues
    }, 500)
    return () => {
      clearTimeout(t)
    }
  }, [
    placeholderValues,
    prompt?.slug,
    tab,
    currentTeam,
    isLoadingPlaceholders,
    allPlaceholders.length,
    renderPrompt,
  ])

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

  const versionHistory = buildPromptVersionHistory(prompt, versions)

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
  if (canDeleteResource(prompt.user_id)) {
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
        description={
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <StatusBadge tone={statusTone('prompt', prompt.status)}>
              {prompt.status}
            </StatusBadge>
            {prompt.is_shared && (
              <Badge variant="secondary" className="gap-1">
                <Share2 className="size-3" />
                Shared
              </Badge>
            )}
            <span className="text-muted-foreground">
              ID: <span className="font-mono">{prompt.slug}</span> · Updated{' '}
              {getRelativeTime(prompt.updated_at)}
            </span>
          </div>
        }
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
            project={project}
            projectHref={p => buildProjectEditUrl(currentTeam?.id, p.slug)}
          />
        }
        extraSections={promptUsedBySection(dependencies, loadingDependencies)}
      >
        <ResourceBody
          content={renderedBody !== '' ? renderedBody : prompt.body}
          rawContent={prompt.body}
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
