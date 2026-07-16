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
      <div className="space-y-6">
        <PageHeader title="Loading blueprint…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (error || !blueprint) {
    return (
      <div className="space-y-6">
        <PageHeader title="Blueprint not found" />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Blueprint not found</AlertTitle>
          <AlertDescription>
            {error ?? 'The blueprint could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => {
            void navigate('/blueprints')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back
        </Button>
      </div>
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

  return (
    <div className="space-y-6">
      <PageHeader
        title={blueprint.title}
        description={blueprint.description}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/blueprints')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            <CopyButton
              value={blueprint.content}
              label="Copy content"
              size="default"
              variant="outline"
            />
            <Button
              variant="outline"
              onClick={() => {
                void navigate(`${base}/edit`)
              }}
            >
              <Pencil className="mr-2 size-4" />
              Edit
            </Button>
            {canDeleteResource(blueprint.user_id) && (
              <Button
                variant="destructive"
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
                content={blueprint.content}
                syntaxTheme="auto"
              />
            </CardContent>
          </Card>
        </div>

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
          </MetadataPanel>

          <AdditionalDataCard data={blueprint.metadata ?? {}} />

          {currentTeam && (
            <ResourceAttachments
              teamId={currentTeam.id}
              ownerType="blueprint"
              ownerId={blueprint.id}
            />
          )}

          {currentTeam && (
            <AccessActivityPanel
              teamId={currentTeam.id}
              resourceType="blueprint"
              resourceId={blueprint.id}
            />
          )}

          {currentTeam && (
            <CommentsPanel
              teamId={currentTeam.id}
              resourceType="blueprint"
              resourceId={blueprint.id}
            />
          )}
        </div>
      </div>

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
    </div>
  )
}
