import { ArrowLeft, RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { FeedTabs, FeedToolbar } from '@/pages/feeds/FeedChrome'
import { FeedItemList } from '@/pages/feeds/FeedItemList'
import { FeedPostComposer } from '@/pages/feeds/FeedPostComposer'
import { feedService } from '@/services/feedService'
import { projectService } from '@/services/projectService'
import { teamService } from '@/services/teamService'
import type { Project } from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import type { Feed, FeedItem, FeedItemFilters } from '@/types/feed'
import type { TeamMember } from '@/types/team'
import { getErrorMessage } from '@/utils/errorHandling'

function buildDeleteDescription(title?: string) {
  return (
    <>
      This will permanently delete{' '}
      <span className="font-medium">{title ?? 'this feed item'}</span>. This
      action cannot be undone.
    </>
  )
}

function useFeedSidebarData(teamId: string | undefined) {
  const [projects, setProjects] = useState<Project[]>([])
  const [members, setMembers] = useState<Map<string, TeamMember>>(new Map())

  useEffect(() => {
    const run = async () => {
      if (!teamId) return
      try {
        setProjects(
          (await projectService.getProjects(teamId, { limit: 100 })).projects
        )
      } catch (e) {
        console.error('Failed to load projects:', e)
      }
    }
    void run()
  }, [teamId])

  useEffect(() => {
    const run = async () => {
      if (!teamId) return
      try {
        const list = await teamService.getTeamMembers(teamId)
        setMembers(new Map(list.map(m => [m.user_id, m])))
      } catch (e) {
        console.error('Failed to load team members:', e)
      }
    }
    void run()
  }, [teamId])

  return { projects, members }
}

interface ItemsState {
  items: FeedItem[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  totalActive?: number
  totalArchived?: number
}

interface FeedHeaderActionsProps {
  isLoading: boolean
  onAllFeeds: () => void
  onRefresh: () => void
  onEdit: () => void
}

function FeedHeaderActions({
  isLoading,
  onAllFeeds,
  onRefresh,
  onEdit,
}: FeedHeaderActionsProps) {
  return (
    <>
      <Button variant="outline" onClick={onAllFeeds}>
        <ArrowLeft className="mr-2 size-4" />
        All feeds
      </Button>
      <Button
        variant="outline"
        size="icon"
        aria-label="Refresh"
        onClick={onRefresh}
        disabled={isLoading}
      >
        <RefreshCw className={`size-4 ${isLoading ? 'animate-spin' : ''}`} />
      </Button>
      <Button variant="outline" onClick={onEdit}>
        Edit feed
      </Button>
    </>
  )
}

export function FeedView() {
  const { feedId } = useParams<{ feedId: string }>()
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { currentProject, isLoading: isProjectLoading } = useProject()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [feed, setFeed] = useState<Feed | null>(null)
  const [feedLoading, setFeedLoading] = useState(true)
  const { projects, members } = useFeedSidebarData(currentTeam?.id)
  const [assistants, setAssistants] = useState<string[]>([])
  const [activeCount, setActiveCount] = useState<number | undefined>()
  const [archivedCount, setArchivedCount] = useState<number | undefined>()
  const [tab, setTab] = useState<'active' | 'archived'>('active')
  const [itemsState, setItemsState] = useState<ItemsState>({
    items: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
  })
  const [searchInput, setSearchInput] = useState('')
  const [filters, setFilters] = useState<FeedItemFilters>(() => ({
    page: 1,
    limit: 20,
    archived: 'false',
    project_id: currentProject?.id,
  }))
  const [itemToDelete, setItemToDelete] = useState<FeedItem | null>(null)
  const [deletingItem, setDeletingItem] = useState(false)

  useEffect(() => {
    const run = async () => {
      if (!feedId || !currentTeam) return
      try {
        setFeedLoading(true)
        setFeed(await feedService.getFeed(currentTeam.id, feedId))
      } catch (e) {
        handleError(e, 'Failed to load feed')
      } finally {
        setFeedLoading(false)
      }
    }
    void run()
  }, [feedId, currentTeam, handleError])

  const fetchItems = useCallback(
    async (f: FeedItemFilters) => {
      // Wait for a persisted project selection to restore, so the first fetch
      // is already scoped instead of flashing unfiltered results.
      if (!currentTeam || !feedId || isProjectLoading) return
      setItemsState(prev => ({ ...prev, loading: true, error: null }))
      try {
        const res = await feedService.getFeedItemsForFeed(
          currentTeam.id,
          feedId,
          f
        )
        setItemsState(prev => ({
          ...prev,
          items: res.items,
          loading: false,
          error: null,
          totalPages: res.total_pages,
          currentPage: f.page ?? 1,
        }))
        // Update count for current tab from this response
        if (f.archived === 'true') setArchivedCount(res.total_count)
        else setActiveCount(res.total_count)
        const names = Array.from(
          new Set(res.items.map(i => i.ai_assistant_name))
        ).filter(Boolean)
        if (names.length > 0)
          setAssistants(prev => Array.from(new Set([...prev, ...names])))
      } catch (e: unknown) {
        setItemsState(prev => ({
          ...prev,
          loading: false,
          error: getErrorMessage(e, 'Failed to fetch feed items'),
        }))
        handleError(e, 'Failed to load feed items')
      }
    },
    [currentTeam, feedId, isProjectLoading, handleError]
  )

  useEffect(() => {
    void fetchItems(filters)
  }, [fetchItems, filters])

  // Lightweight count fetch for the OTHER tab so its badge can render.
  // The current tab's count is set by `fetchItems` from `total_count`.
  // We don't depend on items.length — that would re-fire after every
  // main fetch and double-load. apiClient does not currently accept an
  // AbortSignal, so the request still completes; we only suppress the
  // resulting setState if the effect has been re-fired or unmounted.
  useEffect(() => {
    if (!currentTeam || !feedId) return
    const otherArchived = tab === 'archived' ? 'false' : 'true'
    const ctrl = new AbortController()
    void (async () => {
      try {
        const res = await feedService.getFeedItemsForFeed(
          currentTeam.id,
          feedId,
          { page: 1, limit: 1, archived: otherArchived }
        )
        if (ctrl.signal.aborted) return
        if (otherArchived === 'true') setArchivedCount(res.total_count)
        else setActiveCount(res.total_count)
      } catch {
        // Non-fatal — tab badge just stays undefined
      }
    })()
    return () => {
      ctrl.abort()
    }
  }, [currentTeam, feedId, tab])

  // Debounced search input → filter
  useEffect(() => {
    const t = setTimeout(() => {
      setFilters(prev =>
        prev.search === searchInput
          ? prev
          : {
              ...prev,
              search: searchInput !== '' ? searchInput : undefined,
              page: 1,
            }
      )
    }, 400)
    return () => {
      clearTimeout(t)
    }
  }, [searchInput])

  // Keep the list scoped to the globally selected project (header selector).
  const currentProjectId = currentProject?.id
  useEffect(() => {
    setFilters(prev =>
      prev.project_id === currentProjectId
        ? prev
        : { ...prev, project_id: currentProjectId, page: 1 }
    )
  }, [currentProjectId])

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.FEED_PAGE_VIEW,
      properties: { feed_id: feedId, action_context: 'view' },
    })
  }, [trackEvent, feedId])

  const handleTabChange = (newTab: 'active' | 'archived') => {
    setTab(newTab)
    setFilters(prev => ({
      ...prev,
      archived: newTab === 'archived' ? 'true' : 'false',
      page: 1,
    }))
  }

  const handleArchiveItem = async (item: FeedItem) => {
    if (!currentTeam) return
    try {
      await feedService.archiveFeedItem(currentTeam.id, item.id)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_ITEM_ARCHIVED,
        properties: { feed_item_id: item.id, feed_id: feedId },
      })
      showSuccess('Feed item archived', 'Success')
      void fetchItems(filters)
    } catch (e) {
      handleError(e, 'Failed to archive feed item')
    }
  }

  const handleUnarchiveItem = async (item: FeedItem) => {
    if (!currentTeam) return
    try {
      await feedService.unarchiveFeedItem(currentTeam.id, item.id)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_ITEM_UNARCHIVED,
        properties: { feed_item_id: item.id, feed_id: feedId },
      })
      showSuccess('Feed item unarchived', 'Success')
      void fetchItems(filters)
    } catch (e) {
      handleError(e, 'Failed to unarchive feed item')
    }
  }

  const handleDeleteItem = async () => {
    if (!itemToDelete || !currentTeam) return
    try {
      setDeletingItem(true)
      await feedService.deleteFeedItem(currentTeam.id, itemToDelete.id)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_ITEM_DELETED,
        properties: { feed_item_id: itemToDelete.id, feed_id: feedId },
      })
      showSuccess('Feed item deleted', 'Success')
      void fetchItems(filters)
    } catch (e) {
      handleError(e, 'Failed to delete feed item')
    } finally {
      setDeletingItem(false)
      setItemToDelete(null)
    }
  }

  const getProjectName = (pid: string | null | undefined) =>
    pid !== null && pid !== undefined
      ? projects.find(p => p.id === pid)?.name
      : undefined
  const getMember = (uid: string | null | undefined) =>
    uid !== null && uid !== undefined ? members.get(uid) : undefined
  const hasFilters =
    !!filters.search || !!filters.project_id || !!filters.ai_assistant_name

  const description = feedLoading ? undefined : (feed?.description ?? undefined)

  return (
    <div className="space-y-6">
      <PageHeader
        title={feedLoading ? 'Loading feed…' : (feed?.name ?? 'Feed')}
        description={description}
        actions={
          <FeedHeaderActions
            isLoading={itemsState.loading}
            onAllFeeds={() => {
              void navigate('/feeds')
            }}
            onRefresh={() => {
              void fetchItems(filters)
            }}
            onEdit={() => {
              void navigate(`/feeds/${feedId ?? ''}/edit`)
            }}
          />
        }
      />
      <div className="space-y-5">
        {feedId && tab === 'active' && (
          <FeedPostComposer
            feedId={feedId}
            projects={projects}
            onPosted={() => {
              void fetchItems(filters)
            }}
          />
        )}

        {/* Tabs row */}
        <FeedTabs
          tab={tab}
          onChange={handleTabChange}
          activeCount={activeCount}
          archivedCount={archivedCount}
        />

        <FeedToolbar
          searchInput={searchInput}
          onSearchChange={setSearchInput}
          assistants={assistants}
          assistantName={filters.ai_assistant_name}
          onAssistantChange={v => {
            setFilters(prev => ({ ...prev, ai_assistant_name: v, page: 1 }))
          }}
        />

        <FeedItemList
          items={itemsState.items}
          loading={itemsState.loading}
          error={itemsState.error}
          totalPages={itemsState.totalPages}
          currentPage={itemsState.currentPage}
          tab={tab}
          hasFilters={hasFilters}
          projectName={getProjectName}
          member={getMember}
          onArchive={handleArchiveItem}
          onUnarchive={handleUnarchiveItem}
          onDelete={setItemToDelete}
          onPagePrev={() => {
            setFilters(prev => ({
              ...prev,
              page: (prev.page ?? 1) - 1,
            }))
          }}
          onPageNext={() => {
            setFilters(prev => ({
              ...prev,
              page: (prev.page ?? 1) + 1,
            }))
          }}
        />
      </div>
      <ConfirmDialog
        open={!!itemToDelete}
        onOpenChange={open => {
          if (!open) setItemToDelete(null)
        }}
        title="Delete feed item?"
        description={buildDeleteDescription(itemToDelete?.title)}
        confirmLabel="Delete"
        variant="destructive"
        loading={deletingItem}
        onConfirm={handleDeleteItem}
      />
    </div>
  )
}
