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
} from '@/components/patterns/resource'
import { ResourceReadingPage } from '@/components/resource-detail/ResourceReadingPage'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceProject } from '@/hooks/useResourceProject'
import type { Artifact, ArtifactVersion } from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

export function ArtifactView() {
  const { project, slug } = useParams<{ project: string; slug: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [artifact, setArtifact] = useState<Artifact | null>(null)
  const [versions, setVersions] = useState<ArtifactVersion[]>([])
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
    // Guard against stale responses: if params/team change mid-flight, a slower
    // earlier request must not overwrite the newer artifact's state.
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
        const a = await artifactService.getArtifact(
          currentTeam.id,
          decodedProject,
          decodedSlug
        )
        setArtifact(a)
        // Track the view immediately — analytics must not wait on the
        // best-effort version-history fetch below.
        trackEvent({
          event: ANALYTICS_EVENTS.ARTIFACT_VIEWED,
          properties: {
            artifact_id: a.slug,
            artifact_type: a.type,
            artifact_title: a.title,
            action_context: 'view',
          },
        })
        // Version history powers the Metadata panel's footer link + count chip.
        // Best-effort: a failure here must not break the artifact view itself.
        // Guarded so a stale response (params/team changed mid-flight) can't
        // overwrite a newer artifact's version state.
        try {
          const history = await artifactService.getArtifactVersions(
            currentTeam.id,
            decodedProject,
            decodedSlug
          )
          if (active) setVersions(history.versions)
        } catch {
          if (active) setVersions([])
        }
      } catch (err) {
        setError(getErrorMessage(err, 'Failed to fetch artifact'))
        handleError(err, 'Failed to load artifact')
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

  const base = `/artifacts/${encodeURIComponent(artifact.project_id)}/${encodeURIComponent(artifact.slug)}`
  // Snapshots capture the *prior* content and version numbers are monotonic
  // (never reused, oldest pruned past the retention cap), so the live artifact's
  // version is one past the highest retained snapshot number. `versions.length`
  // is the number of entries shown on the linked history page — the chip count.
  const latestVersionNumber = versions.reduce(
    (max, v) => Math.max(max, v.version_number),
    0
  )
  // Only surface the version-history affordance once there's history to show;
  // a "0" chip linking to an empty page would be misleading.
  const versionHistory =
    versions.length > 0
      ? {
          count: versions.length,
          currentVersion: latestVersionNumber + 1,
          editedAt: artifact.updated_at,
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
        description={artifact.description}
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
              projectHref={p =>
                `/teams/${currentTeam?.id ?? ''}/projects/${encodeURIComponent(p.slug)}/edit`
              }
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
