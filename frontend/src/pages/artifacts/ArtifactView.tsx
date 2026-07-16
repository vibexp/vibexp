import { AlertCircle, ArrowLeft, Pencil, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { AccessActivityPanel } from '@/components/access-activity/AccessActivityPanel'
import { ResourceAttachments } from '@/components/attachments/ResourceAttachments'
import { CommentsPanel } from '@/components/comments/CommentsPanel'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CopyButton } from '@/components/CopyButton'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import {
  MetadataPanel,
  MetaRow,
  MetaSlugRow,
} from '@/components/metadata/MetadataPanel'
import { AdditionalDataCard } from '@/components/MetadataCard'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import {
  ARTIFACT_STATUS_LABEL,
  artifactStatusTone,
} from '@/pages/artifacts/artifactStatus'
import type { Artifact, ArtifactVersion } from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

const TYPE_LABEL: Record<Artifact['type'], string> = {
  general: 'General',
  work_reports: 'Work reports',
  static_contexts: 'Static contexts',
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
  const [versions, setVersions] = useState<ArtifactVersion[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

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
      <div className="space-y-6">
        <PageHeader title="Loading artifact…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (error || !artifact) {
    return (
      <div className="space-y-6">
        <PageHeader title="Artifact not found" />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Artifact not found</AlertTitle>
          <AlertDescription>
            {error ?? 'The artifact could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => {
            void navigate('/artifacts')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back
        </Button>
      </div>
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

  return (
    <div className="space-y-6">
      <PageHeader
        title={artifact.title}
        description={artifact.description}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/artifacts')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            <CopyButton
              value={artifact.content ?? ''}
              label="Copy content"
              size="default"
              variant="outline"
            />
            <Button
              variant="outline"
              data-testid="edit-artifact-button"
              onClick={() => {
                void navigate(`${base}/edit`)
              }}
            >
              <Pencil className="mr-2 size-4" />
              Edit
            </Button>
            {canDeleteResource(artifact.user_id) && (
              <Button
                variant="destructive"
                data-testid="delete-artifact-button"
                onClick={() => {
                  setDeleteOpen(true)
                }}
              >
                <Trash2 className="mr-2 size-4" />
                Delete
              </Button>
            )}
          </>
        }
      />

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <Card>
            <CardContent className="pt-6">
              <MarkdownRenderer
                content={artifact.content ?? ''}
                syntaxTheme="auto"
              />
            </CardContent>
          </Card>
        </div>

        <div className="space-y-4">
          <MetadataPanel
            createdAt={artifact.created_at}
            updatedAt={artifact.updated_at}
            versionHistory={versionHistory}
          >
            <MetaRow label="Type">
              <Badge variant="secondary">{TYPE_LABEL[artifact.type]}</Badge>
            </MetaRow>
            <MetaRow label="Status">
              <StatusBadge tone={artifactStatusTone(artifact.status)}>
                {ARTIFACT_STATUS_LABEL[artifact.status]}
              </StatusBadge>
            </MetaRow>
            <MetaSlugRow value={artifact.slug} />
          </MetadataPanel>

          <AdditionalDataCard data={artifact.metadata ?? {}} />

          {currentTeam && (
            <ResourceAttachments
              teamId={currentTeam.id}
              ownerType="artifact"
              ownerId={artifact.id}
            />
          )}

          {currentTeam && (
            <AccessActivityPanel
              teamId={currentTeam.id}
              resourceType="artifact"
              resourceId={artifact.id}
            />
          )}

          {currentTeam && (
            <CommentsPanel
              teamId={currentTeam.id}
              resourceType="artifact"
              resourceId={artifact.id}
            />
          )}
        </div>
      </div>

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
    </div>
  )
}
