import {
  AlertCircle,
  ArrowLeft,
  FolderOpen,
  HardDrive,
  Pencil,
  Tag as TagIcon,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'

import { AccessActivityPanel } from '@/components/access-activity/AccessActivityPanel'
import { CommentsPanel } from '@/components/comments/CommentsPanel'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CopyButton } from '@/components/CopyButton'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { MetadataPanel, MetaRow } from '@/components/metadata/MetadataPanel'
import { AdditionalDataCard } from '@/components/MetadataCard'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import {
  MEMORY_STATUS_LABEL,
  memoryStatusTone,
} from '@/pages/memories/memoryStatus'
import type { Memory, MemoryVersion } from '@/services/memoryService'
import { memoryService } from '@/services/memoryService'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

function extractTags(meta?: Record<string, unknown>): string[] {
  const tags = meta?.tags
  if (!Array.isArray(tags)) return []
  return tags.filter((t): t is string => typeof t === 'string')
}

function extractExtras(meta?: Record<string, unknown>) {
  if (!meta) return {}
  const { tags: _tags, ...rest } = meta
  void _tags
  return rest
}

export function MemoryView() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [memory, setMemory] = useState<Memory | null>(null)
  const [versions, setVersions] = useState<MemoryVersion[]>([])
  const [project, setProject] = useState<Project | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const fetchProject = useCallback(
    async (teamId: string, projectId: string) => {
      try {
        const res = await projectService.getProjects(teamId, { limit: 100 })
        const found = res.projects.find(p => p.id === projectId) ?? null
        setProject(found)
      } catch {
        // project metadata is supplemental — don't surface this as a page error
      }
    },
    []
  )

  useEffect(() => {
    // Guard against stale responses: if id/team change mid-flight, a slower earlier
    // request must not overwrite the newer memory's version state.
    let active = true
    const fetchMemory = async () => {
      if (isLoadingTeam) return
      if (!id) {
        setError('Memory ID is required')
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
        const response = await memoryService.getMemory(currentTeam.id, id)
        setMemory(response)
        trackEvent({
          event: ANALYTICS_EVENTS.MEMORY_VIEWED,
          properties: {
            memory_id: response.id,
            memory_type:
              typeof response.metadata?.type === 'string'
                ? response.metadata.type
                : 'unknown',
            action_context: 'view',
          },
        })
        if (response.project_id) {
          void fetchProject(currentTeam.id, response.project_id)
        }
        // Version history powers the Metadata panel's footer link + count chip.
        // Best-effort: a failure here must not break the memory view itself.
        try {
          const history = await memoryService.getMemoryVersions(
            currentTeam.id,
            id
          )
          if (active) setVersions(history.versions)
        } catch {
          if (active) setVersions([])
        }
      } catch (err) {
        const errorMessage = getErrorMessage(err, 'Failed to fetch memory')
        setError(errorMessage)
        handleError(err, 'Failed to load memory')
      } finally {
        setLoading(false)
      }
    }
    void fetchMemory()
    return () => {
      active = false
    }
  }, [id, currentTeam, isLoadingTeam, handleError, trackEvent, fetchProject])

  const handleDelete = async () => {
    if (!memory || !currentTeam) return
    try {
      setDeleting(true)
      await memoryService.deleteMemory(currentTeam.id, memory.id)
      showSuccess('Memory deleted successfully', 'Success')
      void navigate('/memories')
    } catch (err) {
      handleError(err, 'Failed to delete memory')
    } finally {
      setDeleting(false)
      setDeleteOpen(false)
    }
  }

  if (isLoadingTeam || loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading memory…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (error || !memory) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Memory not found"
          actions={
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/memories')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
          }
        />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Memory not found</AlertTitle>
          <AlertDescription>
            {error ?? 'The memory could not be found.'}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  const tags = extractTags(memory.metadata)
  const extras = extractExtras(memory.metadata)

  // Snapshots capture the *prior* text and version numbers are monotonic (never
  // reused, oldest pruned past the retention cap), so the live memory's version is
  // one past the highest retained snapshot number. `versions.length` is the number
  // of entries shown on the linked history page — the chip count.
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
          editedAt: memory.updated_at,
          to: `/memories/${encodeURIComponent(memory.id)}/versions`,
        }
      : undefined

  return (
    <div className="space-y-6">
      <PageHeader
        title={`Memory #${memory.id}`}
        description="View memory details."
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/memories')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            <CopyButton
              value={memory.text}
              label="Copy content"
              size="default"
              variant="outline"
            />
            <Button
              variant="outline"
              onClick={() => {
                void navigate(`/memories/${memory.id}/edit`)
              }}
            >
              <Pencil className="mr-2 size-4" />
              Edit
            </Button>
            {canDeleteResource(memory.user_id) && (
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
              <MarkdownRenderer content={memory.text} syntaxTheme="auto" />
            </CardContent>
          </Card>
        </div>

        <div className="space-y-4">
          <MetadataPanel
            createdAt={memory.created_at}
            updatedAt={memory.updated_at}
            versionHistory={versionHistory}
          >
            <MetaRow label="Status">
              <StatusBadge tone={memoryStatusTone(memory.status)}>
                {MEMORY_STATUS_LABEL[memory.status]}
              </StatusBadge>
            </MetaRow>
            {project && (
              <MetaRow label="Project">
                <Link
                  to={`/settings/projects/edit/${project.slug}`}
                  className="flex items-center gap-1 hover:underline"
                >
                  <FolderOpen className="size-3" />
                  {project.name}
                </Link>
              </MetaRow>
            )}
          </MetadataPanel>

          {tags.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Tags</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap gap-1.5">
                  {tags.map(tag => (
                    <Badge key={tag} variant="secondary" className="gap-1">
                      <TagIcon className="size-3" />
                      {tag}
                    </Badge>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}

          <AdditionalDataCard data={extras} />

          {tags.length === 0 &&
            Object.keys(extras).length === 0 &&
            !project && (
              <div className="text-muted-foreground flex items-center gap-2 p-3 text-xs">
                <HardDrive className="size-4" />
                No metadata.
              </div>
            )}

          {currentTeam && (
            <AccessActivityPanel
              teamId={currentTeam.id}
              resourceType="memory"
              resourceId={memory.id}
            />
          )}

          {currentTeam && (
            <CommentsPanel
              teamId={currentTeam.id}
              resourceType="memory"
              resourceId={memory.id}
            />
          )}
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete memory?"
        description="This will permanently delete the memory. This action cannot be undone."
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </div>
  )
}
