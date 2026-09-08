import { AlertCircle, ArrowLeft, Pencil, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
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
import { deriveMemoryTitle } from '@/lib/memoryTitle'
import { buildProjectEditUrl } from '@/lib/resourceUrl'
import type { Memory } from '@/services/memoryService'
import { memoryService } from '@/services/memoryService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

export function MemoryView() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [memory, setMemory] = useState<Memory | null>(null)
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
      } catch (err) {
        const errorMessage = getErrorMessage(err, 'Failed to fetch memory')
        setError(errorMessage)
        handleError(err, 'Failed to load memory')
      } finally {
        setLoading(false)
      }
    }
    void fetchMemory()
  }, [id, currentTeam, isLoadingTeam, handleError, trackEvent])

  // Memories carry no title, so it is derived from the body — memoised because
  // that scans the whole (unbounded) text and this page re-renders on dialog
  // and view-mode state.
  const title = useMemo(
    () => deriveMemoryTitle(memory?.text ?? ''),
    [memory?.text]
  )

  // Version history powers the Metadata panel's footer link + count chip.
  // Best-effort and stale-guarded by the hook; gated on the same readiness
  // conditions as the detail fetch above.
  const { versionHistory } = useResourceVersions({
    loadVersions:
      !isLoadingTeam && currentTeam && id
        ? () => memoryService.getMemoryVersions(currentTeam.id, id)
        : null,
    to: memory
      ? `/memories/${encodeURIComponent(memory.id)}/versions`
      : undefined,
    editedAt: memory?.updated_at,
    deps: [isLoadingTeam, currentTeam?.id, id],
  })

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

            <ResourceTaxonomySection
              descriptor={resourceRegistry.memory}
              resource={memory}
            />
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
