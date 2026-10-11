import { AlertCircle, ArrowLeft, Pencil, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import {
  type ReadingAction,
  ResourceBody,
  useCopyAction,
} from '@/components/patterns/reading-page'
import {
  ResourceMetadataSection,
  resourceRegistry,
  ResourceTaxonomySection,
  useStatusOptions,
} from '@/components/patterns/resource'
import { ResourceReadingPage } from '@/components/resource-detail/ResourceReadingPage'
import { useResourceLock } from '@/components/resource-lock/useResourceLock'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceVersions } from '@/hooks/useResourceVersions'
import {
  buildProjectEditUrl,
  buildResourceVersionsUrl,
} from '@/lib/resourceUrl'
import type { Artifact } from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

/** The artifact's detail-route base, for the edit action. */
function artifactBase(artifact: Artifact) {
  return `/artifacts/${encodeURIComponent(artifact.project_id)}/${encodeURIComponent(artifact.slug)}`
}

export function ArtifactView() {
  const { project, slug } = useParams<{ project: string; slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const statusOptions = useStatusOptions(resourceRegistry.artifact)
  const { canDeleteResource } = usePermissions()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [artifact, setArtifact] = useState<Artifact | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const backAction: ReadingAction = {
    id: 'back',
    label: 'Back',
    icon: ArrowLeft,
    onClick: () => {
      void navigate('/artifacts')
    },
  }
  const copyAction = useCopyAction(artifact?.content ?? '')

  useEffect(() => {
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
        const a = await artifactService.getArtifact(
          currentTeam.id,
          decodedProject,
          decodedSlug
        )
        setArtifact(a)
        // Track the view immediately — analytics must not wait on the
        // best-effort version-history fetch in `useResourceVersions`.
        trackEvent({
          event: ANALYTICS_EVENTS.ARTIFACT_VIEWED,
          properties: {
            artifact_id: a.slug,
            artifact_type: a.type,
            artifact_title: a.title,
            action_context: 'view',
          },
        })
      } catch (err) {
        setError(getErrorMessage(err, 'Failed to fetch artifact'))
        handleError(err, 'Failed to load artifact')
      } finally {
        setLoading(false)
      }
    }
    void load()
  }, [project, slug, currentTeam, isLoadingTeam, handleError, trackEvent])

  // Version history powers the Metadata panel's footer link + count chip.
  // Best-effort and stale-guarded by the hook; gated on the same readiness
  // conditions as the detail fetch above.
  const { versionHistory } = useResourceVersions({
    loadVersions:
      !isLoadingTeam && currentTeam && project && slug
        ? () =>
            artifactService.getArtifactVersions(
              currentTeam.id,
              decodeURIComponent(project),
              decodeURIComponent(slug)
            )
        : null,
    // Built as the edit page builds it (#1180), so the two links cannot drift.
    to:
      (artifact &&
        buildResourceVersionsUrl({
          type: 'artifact',
          id: artifact.id,
          slug: artifact.slug,
          projectId: artifact.project_id,
        })) ??
      undefined,
    editedAt: artifact?.updated_at,
    deps: [isLoadingTeam, currentTeam?.id, project, slug],
  })

  // Lock / Unlock, the lock dialog and the lock history (epic #1285).
  const resourceLock = useResourceLock<Artifact>({
    kind: 'artifact',
    resource: artifact,
    ops:
      currentTeam && artifact
        ? {
            lock: comment =>
              artifactService.lockArtifact(
                currentTeam.id,
                artifact.project_id,
                artifact.slug,
                comment
              ),
            unlock: () =>
              artifactService.unlockArtifact(
                currentTeam.id,
                artifact.project_id,
                artifact.slug
              ),
            reload: () =>
              artifactService.getArtifact(
                currentTeam.id,
                artifact.project_id,
                artifact.slug
              ),
            loadHistory: () =>
              artifactService.getArtifactLockHistory(
                currentTeam.id,
                artifact.project_id,
                artifact.slug
              ),
          }
        : null,
    onChanged: setArtifact,
  })

  const handleDelete = async () => {
    if (!artifact || !currentTeam) return
    try {
      setDeleting(true)
      await artifactService.deleteArtifact(
        currentTeam.id,
        artifact.project_id,
        artifact.slug
      )
      showSuccess('Artifact deleted successfully', 'Success')
      void navigate('/artifacts')
    } catch (err) {
      handleError(err, 'Failed to delete artifact')
    } finally {
      setDeleting(false)
      setDeleteOpen(false)
    }
  }

  if (isLoadingTeam || loading) {
    return (
      <ResourceReadingPage title="Loading artifact…">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ResourceReadingPage>
    )
  }

  if (error || !artifact) {
    return (
      <ResourceReadingPage title="Artifact not found" actions={[backAction]}>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Artifact not found</AlertTitle>
          <AlertDescription>
            {error ?? 'The artifact could not be found.'}
          </AlertDescription>
        </Alert>
      </ResourceReadingPage>
    )
  }

  const base = artifactBase(artifact)

  const actions: ReadingAction[] = [
    backAction,
    copyAction,
    {
      id: 'edit',
      label: 'Edit',
      icon: Pencil,
      testId: 'edit-artifact-button',
      onClick: () => {
        void navigate(`${base}/edit`)
      },
    },
  ]
  actions.push(...resourceLock.actions)
  // A locked artifact cannot be deleted; Delete returns after unlock.
  if (canDeleteResource(artifact.user_id) && !artifact.lock) {
    actions.push({
      id: 'delete',
      label: 'Delete',
      icon: Trash2,
      tone: 'destructive',
      testId: 'delete-artifact-button',
      onClick: () => {
        setDeleteOpen(true)
      },
    })
  }

  return (
    <>
      <ResourceReadingPage
        title={artifact.title}
        status={{
          value: statusOptions.labelOf(artifact.status),
          tone: statusOptions.toneOf(artifact.status),
        }}
        address={{ value: artifact.slug }}
        updatedAt={artifact.updated_at}
        summary={artifact.description}
        actions={actions}
        extraSections={resourceLock.historySection}
        resource={
          currentTeam
            ? { kind: 'artifact', id: artifact.id, teamId: currentTeam.id }
            : undefined
        }
        metadata={
          <div className="space-y-5">
            <ResourceMetadataSection
              descriptor={resourceRegistry.artifact}
              resource={artifact}
              versionHistory={versionHistory}
              statusLabel={statusOptions.labelOf}
              lock={artifact.lock}
              project={artifact.project}
              projectHref={p => buildProjectEditUrl(currentTeam?.id, p.slug)}
            />
            <ResourceTaxonomySection
              descriptor={resourceRegistry.artifact}
              resource={artifact}
            />
          </div>
        }
      >
        <ResourceBody content={artifact.content ?? ''} />
      </ResourceReadingPage>

      {resourceLock.dialog}

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete artifact?"
        description="This will permanently delete the artifact. This action cannot be undone."
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </>
  )
}
