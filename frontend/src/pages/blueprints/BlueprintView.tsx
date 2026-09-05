import { AlertCircle, ArrowLeft, Pencil, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import {
  MetadataPanel,
  MetaRow,
  MetaSlugRow,
} from '@/components/metadata/MetadataPanel'
import { AdditionalDataCard } from '@/components/MetadataCard'
import {
  type ReadingAction,
  useCopyAction,
} from '@/components/patterns/reading-page'
import { ResourceReadingPage } from '@/components/resource-detail/ResourceReadingPage'
import { StatusBadge } from '@/components/StatusBadge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { formatDate } from '@/lib/time'
import type { Blueprint, BlueprintVersion } from '@/services/blueprintService'
import { blueprintService } from '@/services/blueprintService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

const TYPE_LABEL: Record<Blueprint['type'], string> = {
  general: 'General',
  'claude-code': 'Claude Code',
  claude: 'Claude',
  cursor: 'Cursor',
  codex: 'Codex',
}

export function BlueprintView() {
  const { project, slug } = useParams<{ project: string; slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [blueprint, setBlueprint] = useState<Blueprint | null>(null)
  const [versions, setVersions] = useState<BlueprintVersion[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const backAction: ReadingAction = {
    id: 'back',
    label: 'Back',
    icon: ArrowLeft,
    onClick: () => {
      void navigate('/blueprints')
    },
  }
  const copyAction = useCopyAction(blueprint?.content ?? '')

  useEffect(() => {
    // Guard against stale responses: if params/team change mid-flight, a slower
    // earlier request must not overwrite the newer blueprint's version state.
    let active = true
    const load = async () => {
      if (isLoadingTeam) return
      if (!project || !slug) {
        setError('Missing required context')
        setLoading(false)
        return
      }
      if (!currentTeam) {
        setError('No team available. Please select or create a team first.')
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        // clear stale error from loading-phase runs before issuing a real request
        setError(null)
        const decodedProject = decodeURIComponent(project)
        const decodedSlug = decodeURIComponent(slug)
        const a = await blueprintService.getBlueprint(
          currentTeam.id,
          decodedProject,
          decodedSlug
        )
        setBlueprint(a)
        trackEvent({
          event: ANALYTICS_EVENTS.BLUEPRINT_VIEWED,
          properties: {
            blueprint_id: a.slug,
            blueprint_type: a.type,
            blueprint_title: a.title,
            action_context: 'view',
          },
        })
        // Version history powers the Metadata panel's footer link + count chip.
        // Best-effort: a failure here must not break the blueprint view itself.
        try {
          const history = await blueprintService.getBlueprintVersions(
            currentTeam.id,
            decodedProject,
            decodedSlug
          )
          if (active) setVersions(history.versions)
        } catch {
          if (active) setVersions([])
        }
      } catch (err) {
        setError(getErrorMessage(err, 'Failed to fetch blueprint'))
        handleError(err, 'Failed to load blueprint')
      } finally {
        setLoading(false)
      }
    }
    void load()
    return () => {
      active = false
    }
  }, [project, slug, currentTeam, isLoadingTeam, handleError, trackEvent])

  const handleDelete = async () => {
    if (!blueprint || !currentTeam) return
    try {
      setDeleting(true)
      await blueprintService.deleteBlueprint(
        currentTeam.id,
        blueprint.project_id,
        blueprint.slug
      )
      showSuccess('Blueprint deleted successfully', 'Success')
      void navigate('/blueprints')
    } catch (err) {
      handleError(err, 'Failed to delete blueprint')
    } finally {
      setDeleting(false)
      setDeleteOpen(false)
    }
  }

  if (isLoadingTeam || loading) {
    return (
      <ResourceReadingPage title="Loading blueprint…">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ResourceReadingPage>
    )
  }

  if (error || !blueprint) {
    return (
      <ResourceReadingPage title="Blueprint not found" actions={[backAction]}>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Blueprint not found</AlertTitle>
          <AlertDescription>
            {error ?? 'The blueprint could not be found.'}
          </AlertDescription>
        </Alert>
      </ResourceReadingPage>
    )
  }

  const base = `/blueprints/${encodeURIComponent(blueprint.project_id)}/${encodeURIComponent(blueprint.slug)}`
  // Snapshots capture the *prior* content and version numbers are monotonic (never
  // reused, oldest pruned past the retention cap), so the live blueprint's version is
  // one past the highest retained snapshot number. `versions.length` is the number of
  // entries shown on the linked history page — the chip count.
  const latestVersionNumber = versions.reduce(
    (max, v) => Math.max(max, v.version_number),
    0
  )
  // Only surface the version-history affordance once there's history to show; a "0"
  // chip linking to an empty page would be misleading.
  const versionHistory =
    versions.length > 0
      ? {
          count: versions.length,
          currentVersion: latestVersionNumber + 1,
          editedAt: blueprint.updated_at,
          to: `${base}/versions`,
        }
      : undefined

  const actions: ReadingAction[] = [
    backAction,
    copyAction,
    {
      id: 'edit',
      label: 'Edit',
      icon: Pencil,
      testId: 'edit-blueprint-button',
      onClick: () => {
        void navigate(`${base}/edit`)
      },
    },
  ]
  if (canDeleteResource(blueprint.user_id)) {
    actions.push({
      id: 'delete',
      label: 'Delete',
      icon: Trash2,
      tone: 'destructive',
      testId: 'delete-blueprint-button',
      onClick: () => {
        setDeleteOpen(true)
      },
    })
  }

  return (
    <>
      <ResourceReadingPage
        title={blueprint.title}
        description={blueprint.description}
        actions={actions}
        resource={
          currentTeam
            ? { kind: 'blueprint', id: blueprint.id, teamId: currentTeam.id }
            : undefined
        }
        metadata={
          <div className="space-y-4">
            <MetadataPanel
              createdAt={blueprint.created_at}
              updatedAt={blueprint.updated_at}
              versionHistory={versionHistory}
            >
              <MetaRow label="Type">
                <Badge variant="secondary">{TYPE_LABEL[blueprint.type]}</Badge>
              </MetaRow>
              <MetaRow label="Status">
                <StatusBadge
                  tone={blueprint.status === 'active' ? 'success' : 'neutral'}
                >
                  <span className="size-1.5 rounded-full bg-current" />
                  {blueprint.status}
                </StatusBadge>
              </MetaRow>
              <MetaSlugRow value={blueprint.slug} />
              <MetaRow label="Path">
                <code
                  className="text-foreground/90 min-w-0 truncate font-mono text-xs"
                  title={blueprint.path}
                >
                  {blueprint.path}
                </code>
              </MetaRow>
              {blueprint.source && (
                <>
                  {blueprint.source.repo && (
                    <MetaRow label="Source">
                      <a
                        href={blueprint.source.repo}
                        target="_blank"
                        rel="noreferrer"
                        className="text-primary min-w-0 truncate text-sm hover:underline"
                        title={blueprint.source.repo}
                      >
                        {blueprint.source.repo.replace(/^https?:\/\//, '')}
                      </a>
                    </MetaRow>
                  )}
                  {blueprint.source.commit_sha && (
                    <MetaRow label="Commit">
                      <code className="text-muted-foreground font-mono text-xs">
                        {blueprint.source.commit_sha.slice(0, 7)}
                      </code>
                    </MetaRow>
                  )}
                  {blueprint.source.imported_at && (
                    <MetaRow label="Imported">
                      <span className="text-muted-foreground text-sm">
                        {formatDate(blueprint.source.imported_at)}
                      </span>
                    </MetaRow>
                  )}
                </>
              )}
            </MetadataPanel>
            <AdditionalDataCard data={blueprint.metadata ?? {}} />
          </div>
        }
      >
        <MarkdownRenderer content={blueprint.content} syntaxTheme="auto" />
      </ResourceReadingPage>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete blueprint?"
        description="This will permanently delete the blueprint. This action cannot be undone."
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </>
  )
}
