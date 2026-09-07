import {
  AlertCircle,
  ArrowLeft,
  HardDrive,
  Pencil,
  Tag as TagIcon,
  Trash2,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
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
import { Badge } from '@/components/ui/badge'
import {
  Panel,
  PanelBody,
  PanelHeader,
  PanelTitle,
} from '@/components/ui/panel'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceProject } from '@/hooks/useResourceProject'
import { deriveMemoryTitle } from '@/lib/memoryTitle'
import { buildProjectEditUrl } from '@/lib/resourceUrl'
import type { Memory, MemoryVersion } from '@/services/memoryService'
import { memoryService } from '@/services/memoryService'
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
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  // Supplemental — the Project metadata row needs the project's name, and the
  // memory payload carries only its id.
  const project = useResourceProject(currentTeam?.id, memory?.project_id)

  const backAction: ReadingAction = {
    id: 'back',
    label: 'Back',
    icon: ArrowLeft,
    onClick: () => {
      void navigate('/memories')
    },
  }
  const copyAction = useCopyAction(memory?.text ?? '')

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
  }, [id, currentTeam, isLoadingTeam, handleError, trackEvent])

  // Memories carry no title, so it is derived from the body — memoised because
  // that scans the whole (unbounded) text and this page re-renders on dialog
  // and view-mode state.
  const title = useMemo(
    () => deriveMemoryTitle(memory?.text ?? ''),
    [memory?.text]
  )

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
      <ResourceReadingPage title="Loading memory…">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ResourceReadingPage>
    )
  }

  if (error || !memory) {
    return (
      <ResourceReadingPage title="Memory not found" actions={[backAction]}>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Memory not found</AlertTitle>
          <AlertDescription>
            {error ?? 'The memory could not be found.'}
          </AlertDescription>
        </Alert>
      </ResourceReadingPage>
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

  const actions: ReadingAction[] = [
    backAction,
    copyAction,
    {
      id: 'edit',
      label: 'Edit',
      icon: Pencil,
      testId: 'edit-memory-button',
      onClick: () => {
        void navigate(`/memories/${memory.id}/edit`)
      },
    },
  ]
  if (canDeleteResource(memory.user_id)) {
    actions.push({
      id: 'delete',
      label: 'Delete',
      icon: Trash2,
      tone: 'destructive',
      testId: 'delete-memory-button',
      onClick: () => {
        setDeleteOpen(true)
      },
    })
  }

  const hasMetadata =
    tags.length > 0 || Object.keys(extras).length > 0 || project !== null

  return (
    <>
      <ResourceReadingPage
        title={title}
        status={{
          value: statusLabel('memory', memory.status),
          tone: statusTone('memory', memory.status),
        }}
        updatedAt={memory.updated_at}
        actions={actions}
        resource={
          currentTeam
            ? { kind: 'memory', id: memory.id, teamId: currentTeam.id }
            : undefined
        }
        // Memory is not a registered attachment owner_type server-side — the
        // universal attachments endpoint only accepts artifact/prompt/blueprint
        // (internal/server/server.go), so the panel would 404 on list and
        // upload. Raised on epic #899; flip this once the backend registers it.
        attachments={false}
        metadata={
          <div className="space-y-5">
            <ResourceMetadataSection
              descriptor={resourceRegistry.memory}
              resource={memory}
              versionHistory={versionHistory}
              project={project}
              projectHref={p => buildProjectEditUrl(currentTeam?.id, p.slug)}
            />

            {tags.length > 0 && (
              <Panel>
                <PanelHeader>
                  <PanelTitle>Tags</PanelTitle>
                </PanelHeader>
                <PanelBody className="pb-4">
                  <div className="flex flex-wrap gap-1.5">
                    {tags.map(tag => (
                      <Badge key={tag} variant="secondary" className="gap-1">
                        <TagIcon className="size-3" />
                        {tag}
                      </Badge>
                    ))}
                  </div>
                </PanelBody>
              </Panel>
            )}

            <AdditionalDataCard data={extras} />

            {!hasMetadata && (
              <div className="text-muted-foreground flex items-center gap-2 p-3 text-xs">
                <HardDrive className="size-4" />
                No metadata.
              </div>
            )}
          </div>
        }
      >
        <ResourceBody content={memory.text} />
      </ResourceReadingPage>

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
    </>
  )
}
