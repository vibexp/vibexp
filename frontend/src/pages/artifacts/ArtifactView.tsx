import { AlertCircle, ArrowLeft, Pencil, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { AdditionalDataCard } from '@/components/MetadataCard'
import {
  type ReadingAction,
  ResourceBody,
  useCopyAction,
} from '@/components/patterns/reading-page'
import {
  ResourceMetadataSection,
  resourceRegistry,
  statusLabel,
  statusTone,
} from '@/components/patterns/resource'
import { ResourceReadingPage } from '@/components/resource-detail/ResourceReadingPage'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceProject } from '@/hooks/useResourceProject'
import { useResourceVersions } from '@/hooks/useResourceVersions'
import { buildProjectEditUrl } from '@/lib/resourceUrl'
import type { Artifact } from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

/**
 * The artifact's detail-route base — shared by the edit action and the
 * version-history link, so the two can never drift apart.
 */
function artifactBase(artifact: Artifact) {
  return `/artifacts/${encodeURIComponent(artifact.project_id)}/${encodeURIComponent(artifact.slug)}`
}

export function ArtifactView() {
  const { project, slug } = useParams<{ project: string; slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [artifact, setArtifact] = useState<Artifact | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  // Supplemental — the Project metadata row needs the project's name, and the
  // artifact payload carries only its id.
  const projectRef = useResourceProject(currentTeam?.id, artifact?.project_id)

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
    to: artifact ? `${artifactBase(artifact)}/versions` : undefined,
    editedAt: artifact?.updated_at,
    deps: [isLoadingTeam, currentTeam?.id, project, slug],
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
  if (canDeleteResource(artifact.user_id)) {
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
          value: statusLabel('artifact', artifact.status),
          tone: statusTone('artifact', artifact.status),
        }}
        address={{ value: artifact.slug }}
        updatedAt={artifact.updated_at}
        summary={artifact.description}
        actions={actions}
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
              project={projectRef}
              projectHref={p => buildProjectEditUrl(currentTeam?.id, p.slug)}
            />
            <AdditionalDataCard data={artifact.metadata ?? {}} />
          </div>
        }
      >
        <ResourceBody content={artifact.content ?? ''} />
      </ResourceReadingPage>

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
