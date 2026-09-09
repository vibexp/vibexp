import {
  AlertCircle,
  Archive,
  ArchiveRestore,
  ArrowLeft,
  FolderOpen,
  MessageSquare,
  Rss,
  Trash2,
} from 'lucide-react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MetadataPanel, MetaRow } from '@/components/metadata/MetadataPanel'
import {
  type ReadingAction,
  type ReadingSection,
  ResourceBody,
} from '@/components/patterns/reading-page'
import { RelativeTime } from '@/components/RelativeTime'
import { ResourceReadingPage } from '@/components/resource-detail/ResourceReadingPage'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { FeedActorAvatar, resolveFeedActor } from '@/pages/feeds/feedActor'
import { FeedItemReplies } from '@/pages/feeds/FeedItemReplies'
import type { Feed, FeedItem } from '@/services/feedService'
import { feedService } from '@/services/feedService'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'
import type { TeamMember } from '@/services/teamService'
import { teamService } from '@/services/teamService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

export function FeedItemView() {
  const { itemId } = useParams<{ itemId: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: teamLoading } = useTeam()
  const { canDeleteFeedContent } = usePermissions()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [item, setItem] = useState<FeedItem | null>(null)
  const [feed, setFeed] = useState<Feed | null>(null)
  const [project, setProject] = useState<Project | null>(null)
  const [author, setAuthor] = useState<TeamMember | undefined>(undefined)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [archiving, setArchiving] = useState(false)

  useEffect(() => {
    const ctrl = new AbortController()
    const isCancelled = () => ctrl.signal.aborted
    const load = async () => {
      if (teamLoading) return
      if (!itemId) {
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
        const feedItem = await feedService.getFeedItem(currentTeam.id, itemId)
        if (isCancelled()) return
        setItem(feedItem)
        trackEvent({
          event: ANALYTICS_EVENTS.FEED_ITEM_VIEWED,
          properties: {
            feed_item_id: feedItem.id,
            feed_id: feedItem.feed_id,
            action_context: 'view',
          },
        })
        const [feedData, projectsData, membersData] = await Promise.allSettled([
          feedService.getFeed(currentTeam.id, feedItem.feed_id),
          feedItem.project_id
            ? projectService.getProjects(currentTeam.id, { limit: 100 })
            : Promise.resolve(null),
          teamService.getTeamMembers(currentTeam.id),
        ])
        if (isCancelled()) return
        if (feedData.status === 'fulfilled') setFeed(feedData.value)
        if (projectsData.status === 'fulfilled' && projectsData.value) {
          const found = projectsData.value.projects.find(
            p => p.id === feedItem.project_id
          )
          setProject(found ?? null)
        }
        if (membersData.status === 'fulfilled') {
          const match = membersData.value.find(
            m => m.user_id === feedItem.posted_by_user_id
          )
          setAuthor(match)
        } else {
          // Non-fatal: header falls back to "Unknown user"
          console.error('Failed to load team members:', membersData.reason)
        }
      } catch (err) {
        if (isCancelled()) return
        setError(getErrorMessage(err, 'Failed to fetch feed item'))
        handleError(err, 'Failed to load feed item')
      } finally {
        if (!isCancelled()) setLoading(false)
      }
    }
    void load()
    return () => {
      ctrl.abort()
    }
  }, [itemId, currentTeam, teamLoading, handleError, trackEvent])

  const handleArchive = async () => {
    if (!item || !currentTeam) return
    try {
      setArchiving(true)
      await feedService.archiveFeedItem(currentTeam.id, item.id)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_ITEM_ARCHIVED,
        properties: { feed_item_id: item.id, feed_id: item.feed_id },
      })
      showSuccess('Feed item archived', 'Success')
      setItem(prev =>
        prev ? { ...prev, archived_at: new Date().toISOString() } : prev
      )
    } catch (err) {
      handleError(err, 'Failed to archive feed item')
    } finally {
      setArchiving(false)
    }
  }

  const handleUnarchive = async () => {
    if (!item || !currentTeam) return
    try {
      setArchiving(true)
      await feedService.unarchiveFeedItem(currentTeam.id, item.id)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_ITEM_UNARCHIVED,
        properties: { feed_item_id: item.id, feed_id: item.feed_id },
      })
      showSuccess('Feed item unarchived', 'Success')
      setItem(prev => (prev ? { ...prev, archived_at: null } : prev))
    } catch (err) {
      handleError(err, 'Failed to unarchive feed item')
    } finally {
      setArchiving(false)
    }
  }

  const handleDelete = async () => {
    if (!item || !currentTeam) return
    try {
      setDeleting(true)
      await feedService.deleteFeedItem(currentTeam.id, item.id)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_ITEM_DELETED,
        properties: { feed_item_id: item.id, feed_id: item.feed_id },
      })
      showSuccess('Feed item deleted', 'Success')
      void navigate('/feeds')
    } catch (err) {
      handleError(err, 'Failed to delete feed item')
    } finally {
      setDeleting(false)
      setDeleteOpen(false)
    }
  }

  // Declared before the early returns so the not-found branch can offer it too.
  // `feed` is what decides the target: without it there is no feed to go back
  // to, only the feed list.
  const backAction: ReadingAction = useMemo(
    () => ({
      id: 'back',
      label: 'Back',
      icon: ArrowLeft,
      onClick: () => {
        void navigate(feed ? `/feeds/${encodeURIComponent(feed.id)}` : '/feeds')
      },
    }),
    [navigate, feed]
  )

  if (teamLoading || loading) {
    return (
      <ResourceReadingPage title="Loading feed item…">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ResourceReadingPage>
    )
  }

  if (error ?? !item) {
    return (
      <ResourceReadingPage title="Feed item not found" actions={[backAction]}>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Feed item not found</AlertTitle>
          <AlertDescription>
            {error ?? 'The feed item could not be found.'}
          </AlertDescription>
        </Alert>
      </ResourceReadingPage>
    )
  }

  const isArchived = !!item.archived_at
  const actor = resolveFeedActor(item, author)

  const actions: ReadingAction[] = [
    backAction,
    isArchived
      ? {
          id: 'archive',
          label: archiving ? 'Restoring…' : 'Unarchive',
          icon: ArchiveRestore,
          disabled: archiving,
          onClick: () => {
            void handleUnarchive()
          },
        }
      : {
          id: 'archive',
          label: archiving ? 'Archiving…' : 'Archive',
          icon: Archive,
          disabled: archiving,
          onClick: () => {
            void handleArchive()
          },
        },
  ]
  if (canDeleteFeedContent(item.posted_by_user_id)) {
    actions.push({
      id: 'delete',
      // Outlined, like every other reading action — not the solid red button
      // this page used before #890.
      tone: 'destructive',
      label: 'Delete',
      icon: Trash2,
      onClick: () => {
        setDeleteOpen(true)
      },
    })
  }

  // Feed and project are resolved from separately-fetched objects rather than
  // read off the item payload, so the rows are composed here instead of through
  // a descriptor (a feed item has no status, slug or versions to describe).
  const metadataRows: ReactNode[] = []
  if (feed) {
    metadataRows.push(
      <MetaRow key="feed" label="Feed">
        <Link
          to={`/feeds/${encodeURIComponent(feed.id)}`}
          className="flex items-center gap-1 hover:underline"
        >
          <Rss className="size-3" />
          {feed.name}
        </Link>
      </MetaRow>
    )
  }
  if (project) {
    metadataRows.push(
      <MetaRow key="project" label="Project">
        <span className="flex items-center gap-1">
          <FolderOpen className="size-3" />
          {project.name}
        </span>
      </MetaRow>
    )
  }

  // Replies are their own API and their own thread, so they arrive as an extra
  // section rather than through the shared `CommentsPanel`. A feed item has no
  // team-scoped resource id either, so this page passes no `resource` and every
  // standard panel (attachments, activity, comments, relations) drops out on
  // its own.
  const extraSections: ReadingSection[] = []
  if (currentTeam) {
    extraSections.push({
      id: 'replies',
      label: 'Replies',
      icon: MessageSquare,
      content: <FeedItemReplies teamId={currentTeam.id} itemId={item.id} />,
    })
  }

  return (
    <>
      <ResourceReadingPage
        title={item.title}
        // Not `updatedAt`: the shared header prefixes that with "Updated", and
        // a feed item is posted, never edited.
        headerExtra={
          <>
            <span className="inline-flex items-center gap-1.5">
              <FeedActorAvatar actor={actor} size="sm" />
              <span className="text-foreground font-semibold">
                {actor.displayName}
              </span>
            </span>
            {actor.isAi && (
              <Badge variant="outline" className="text-xs">
                AI
              </Badge>
            )}
            {isArchived && (
              <Badge variant="secondary" className="text-xs">
                Archived
              </Badge>
            )}
            <span className="text-muted-foreground inline-flex items-center gap-1">
              Posted <RelativeTime value={item.posted_at} />
            </span>
          </>
        }
        actions={actions}
        metadata={
          metadataRows.length > 0 ? (
            <MetadataPanel>{metadataRows}</MetadataPanel>
          ) : undefined
        }
        extraSections={extraSections}
      >
        <ResourceBody content={item.content} />
      </ResourceReadingPage>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete feed item?"
        description="This will permanently delete this feed item. This action cannot be undone."
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </>
  )
}
