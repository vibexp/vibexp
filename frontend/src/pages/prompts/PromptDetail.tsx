import { AlertCircle, ArrowLeft, Pencil, Share2, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { type VersionHistoryMeta } from '@/components/metadata/MetadataPanel'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics, usePromptRenderer } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { PromptContentCard } from '@/pages/prompts/PromptContentCard'
import { PromptDetailSidebar } from '@/pages/prompts/PromptDetailSidebar'
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

export function PromptDetail() {
  const { slug } = useParams<{ slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
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
  const [tab, setTab] = useState<'rendered' | 'raw'>('rendered')

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

  const handleCopyRendered = async () => {
    try {
      const text = renderedBody !== '' ? renderedBody : (prompt?.body ?? '')
      await navigator.clipboard.writeText(text)
      showSuccess('Copied to clipboard', 'Copied')
    } catch {
      showError('Failed to copy')
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading prompt…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (!prompt) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Prompt not found"
          actions={
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/prompts')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
          }
        />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Prompt not found</AlertTitle>
          <AlertDescription>
            The prompt could not be loaded. It may have been deleted.
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  const versionHistory = buildPromptVersionHistory(prompt, versions)

  return (
    <div className="space-y-6">
      <PageHeader
        title={prompt.name}
        description={
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <StatusBadge
              tone={prompt.status === 'published' ? 'success' : 'warning'}
            >
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
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/prompts')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            <Button
              variant="outline"
              data-testid="edit-prompt-button"
              onClick={() => {
                // Editor still lives in v1 until Slice 5b
                void navigate(`/prompts/${prompt.slug}/edit`)
              }}
            >
              <Pencil className="mr-2 size-4" />
              Edit
            </Button>
            <Button
              variant="destructive"
              data-testid="delete-prompt-button"
              onClick={() => {
                setDeleteOpen(true)
              }}
            >
              <Trash2 className="mr-2 size-4" />
              Delete
            </Button>
          </>
        }
      />

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <PromptContentCard
            prompt={prompt}
            tab={tab}
            onTabChange={setTab}
            renderedBody={renderedBody}
            renderError={renderError}
            isRendering={isRendering}
            isLoadingPlaceholders={isLoadingPlaceholders}
            allPlaceholders={allPlaceholders}
            placeholderValues={placeholderValues}
            updatePlaceholderValue={updatePlaceholderValue}
            onCopy={() => {
              void handleCopyRendered()
            }}
          />
        </div>

        <PromptDetailSidebar
          prompt={prompt}
          teamId={currentTeam?.id}
          dependencies={dependencies}
          loadingDependencies={loadingDependencies}
          versionHistory={versionHistory}
        />
      </div>

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
    </div>
  )
}
