import {
  AlertCircle,
  Archive,
  ArchiveRestore,
  ArrowLeft,
  Trash2,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { formatRelativeTime } from '@/lib/time'
import { FeedActorAvatar, resolveFeedActor } from '@/pages/feeds/feedActor'
import { FeedItemReplies } from '@/pages/feeds/FeedItemReplies'
import { feedService } from '@/services/feedService'
import { projectService } from '@/services/projectService'
import { teamService } from '@/services/teamService'
import type { Project } from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import type { Feed, FeedItem } from '@/types/feed'
import type { TeamMember } from '@/types/team'
import { getErrorMessage } from '@/utils/errorHandling'

export function FeedItemView() {
  const { itemId } = useParams<{ itemId: string }>()
  const navigate = useNavigate()
  const { currentTeam, isLoading: teamLoading } = useTeam()
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

  if (teamLoading || loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading feed item…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (error ?? !item) {
    return (
      <div className="space-y-6">
        <PageHeader title="Feed item not found" />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Feed item not found</AlertTitle>
          <AlertDescription>
            {error ?? 'The feed item could not be found.'}
          </AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => {
            void navigate('/feeds')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to feeds
        </Button>
      </div>
    )
  }

  const isArchived = !!item.archived_at
  const actor = resolveFeedActor(item, author)

  return (
    <div className="space-y-6">
      <PageHeader
        title={item.title}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate(
                  feed ? `/feeds/${encodeURIComponent(item.feed_id)}` : '/feeds'
                )
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            {isArchived ? (
              <Button
                variant="outline"
                onClick={() => {
                  void handleUnarchive()
                }}
                disabled={archiving}
              >
                <ArchiveRestore className="mr-2 size-4" />
                {archiving ? 'Restoring…' : 'Unarchive'}
              </Button>
            ) : (
              <Button
                variant="outline"
                onClick={() => {
                  void handleArchive()
                }}
                disabled={archiving}
              >
                <Archive className="mr-2 size-4" />
                {archiving ? 'Archiving…' : 'Archive'}
              </Button>
            )}
            <Button
              variant="destructive"
              onClick={() => {
                setDeleteOpen(true)
              }}
            >
              <Trash2 className="mr-2 size-4" />
              Delete
            </Button>
          </>
        }
      />

      {/* Single full-width column layout */}
      <div className="space-y-6">
        <Card>
          <CardHeader>
            <div className="flex items-start gap-3">
              <FeedActorAvatar actor={actor} size="md" />
              <div className="min-w-0 flex-1">
                <CardTitle>{item.title}</CardTitle>
                {/* Inline metadata strip — mirrors FeedItemCard social-feed pattern */}
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground mt-2">
                  <span className="font-semibold text-foreground">
                    {actor.displayName}
                  </span>
                  {actor.isAi && (
                    <Badge variant="outline" className="text-xs">
                      AI
                    </Badge>
                  )}
                  <span>{formatRelativeTime(item.posted_at)}</span>
                  {feed && (
                    <button
                      type="button"
                      className="hover:text-primary underline-offset-2 hover:underline"
                      onClick={() => {
                        void navigate(
                          `/feeds/${encodeURIComponent(item.feed_id)}`
                        )
                      }}
                    >
                      {feed.name}
                    </button>
                  )}
                  {project && (
                    <span className="inline-flex items-center gap-1">
                      <span>📁</span>
                      <span>{project.name}</span>
                    </span>
                  )}
                  {isArchived && (
                    <Badge variant="secondary" className="text-xs">
                      Archived
                    </Badge>
                  )}
                </div>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="max-w-3xl">
              <MarkdownRenderer content={item.content} syntaxTheme="auto" />
            </div>
          </CardContent>
        </Card>

        {currentTeam && (
          <FeedItemReplies teamId={currentTeam.id} itemId={item.id} />
        )}
      </div>

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
    </div>
  )
}
