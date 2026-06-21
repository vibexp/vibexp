import { useCallback, useEffect, useState } from 'react'

import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { feedService } from '@/services/feedService'
import { projectService } from '@/services/projectService'
import { teamService } from '@/services/teamService'
import type { Project } from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import type { Feed, FeedItem, FeedItemFilters } from '@/types/feed'
import type { TeamMember } from '@/types/team'
import { getErrorMessage } from '@/utils/errorHandling'

const DEFAULT_PER_PAGE = 20

interface ItemsState {
  items: FeedItem[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
}

export function useFeeds() {
  const { currentTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [tab, setTab] = useState<'active' | 'archived'>('active')
  const [feeds, setFeeds] = useState<Feed[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [members, setMembers] = useState<Map<string, TeamMember>>(new Map())
  const [assistants, setAssistants] = useState<string[]>([])
  const [activeCount, setActiveCount] = useState<number | undefined>()
  const [archivedCount, setArchivedCount] = useState<number | undefined>()
  const [itemsState, setItemsState] = useState<ItemsState>({
    items: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
  })
  const [searchInput, setSearchInput] = useState('')
  const [filters, setFilters] = useState<FeedItemFilters>({
    page: 1,
    limit: DEFAULT_PER_PAGE,
    archived: 'false',
  })
  const [feedToDelete, setFeedToDelete] = useState<Feed | null>(null)
  const [deletingFeed, setDeletingFeed] = useState(false)
  const [itemToDelete, setItemToDelete] = useState<FeedItem | null>(null)
  const [deletingItem, setDeletingItem] = useState(false)

  const fetchItems = useCallback(
    async (currentFilters: FeedItemFilters) => {
      if (!currentTeam) return
      setItemsState(prev => ({ ...prev, loading: true, error: null }))
      try {
        const response = await feedService.getFeedItems(
          currentTeam.id,
          currentFilters
        )
        setItemsState({
          items: response.items,
          loading: false,
          error: null,
          totalPages: response.total_pages,
          currentPage: currentFilters.page ?? 1,
        })
        if (currentFilters.archived === 'true') {
          setArchivedCount(response.total_count)
        } else {
          setActiveCount(response.total_count)
        }
        const names = Array.from(
          new Set(response.items.map(i => i.ai_assistant_name))
        ).filter(Boolean)
        if (names.length > 0) {
          setAssistants(prev => Array.from(new Set([...prev, ...names])))
        }
      } catch (error: unknown) {
        setItemsState(prev => ({
          ...prev,
          loading: false,
          error: getErrorMessage(error, 'Failed to fetch feed items'),
        }))
        handleError(error, 'Failed to load feed items')
      }
    },
    [currentTeam, handleError]
  )

  useEffect(() => {
    void fetchItems(filters)
  }, [fetchItems, filters])

  // Lightweight count fetch for the OTHER tab so its badge can render.
  // The current tab's count is set by `fetchItems` from `total_count`.
  // We use AbortController only to flag stale renders — apiClient does
  // not currently accept an AbortSignal so the HTTP request still
  // completes; we just suppress the resulting setState if the effect
  // has been re-fired or unmounted.
  useEffect(() => {
    if (!currentTeam) return
    const otherArchived = tab === 'archived' ? 'false' : 'true'
    const ctrl = new AbortController()
    void (async () => {
      try {
        const res = await feedService.getFeedItems(currentTeam.id, {
          page: 1,
          limit: 1,
          archived: otherArchived,
        })
        if (ctrl.signal.aborted) return
        if (otherArchived === 'true') setArchivedCount(res.total_count)
        else setActiveCount(res.total_count)
      } catch {
        // Non-fatal — tab badge stays undefined
      }
    })()
    return () => {
      ctrl.abort()
    }
  }, [currentTeam, tab])

  useEffect(() => {
    const run = async () => {
      if (!currentTeam) return
      try {
        const res = await feedService.getFeeds(currentTeam.id, {
          limit: 100,
        })
        setFeeds(res.feeds)
      } catch (e) {
        console.error('Failed to load feeds:', e)
      }
    }
    void run()
  }, [currentTeam])

  useEffect(() => {
    const run = async () => {
      if (!currentTeam) return
      try {
        const res = await projectService.getProjects(currentTeam.id, {
          limit: 100,
        })
        setProjects(res.projects)
      } catch (e) {
        console.error('Failed to load projects:', e)
      }
    }
    void run()
  }, [currentTeam])

  useEffect(() => {
    const run = async () => {
      if (!currentTeam) return
      try {
        const res = await teamService.getTeamMembers(currentTeam.id)
        setMembers(new Map(res.map(m => [m.user_id, m])))
      } catch (e) {
        // Non-fatal: feed will fall back to "Unknown user" / AI assistant name
        console.error('Failed to load team members:', e)
      }
    }
    void run()
  }, [currentTeam])

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
    }, 500)
    return () => {
      clearTimeout(t)
    }
  }, [searchInput])

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.FEED_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const handleTabChange = (value: string) => {
    const newTab = value as 'active' | 'archived'
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
        properties: { feed_item_id: item.id },
      })
      showSuccess('Feed item archived', 'Success')
      void fetchItems(filters)
    } catch (error) {
      handleError(error, 'Failed to archive feed item')
    }
  }

  const handleUnarchiveItem = async (item: FeedItem) => {
    if (!currentTeam) return
    try {
      await feedService.unarchiveFeedItem(currentTeam.id, item.id)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_ITEM_UNARCHIVED,
        properties: { feed_item_id: item.id },
      })
      showSuccess('Feed item unarchived', 'Success')
      void fetchItems(filters)
    } catch (error) {
      handleError(error, 'Failed to unarchive feed item')
    }
  }

  const handleDeleteItem = async () => {
    if (!itemToDelete || !currentTeam) return
    try {
      setDeletingItem(true)
      await feedService.deleteFeedItem(currentTeam.id, itemToDelete.id)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_ITEM_DELETED,
        properties: { feed_item_id: itemToDelete.id },
      })
      showSuccess('Feed item deleted', 'Success')
      void fetchItems(filters)
    } catch (error) {
      handleError(error, 'Failed to delete feed item')
    } finally {
      setDeletingItem(false)
      setItemToDelete(null)
    }
  }

  const handleDeleteFeed = async () => {
    if (!feedToDelete || !currentTeam) return
    try {
      setDeletingFeed(true)
      await feedService.deleteFeed(currentTeam.id, feedToDelete.id)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_DELETED,
        properties: { feed_id: feedToDelete.id },
      })
      showSuccess('Feed deleted', 'Success')
      setFeeds(prev => prev.filter(f => f.id !== feedToDelete.id))
      void fetchItems(filters)
    } catch (error) {
      handleError(error, 'Failed to delete feed')
    } finally {
      setDeletingFeed(false)
      setFeedToDelete(null)
    }
  }

  const getFeedName = (feedId: string) => feeds.find(f => f.id === feedId)?.name
  const getProjectName = (pid: string | null | undefined) =>
    pid !== null && pid !== undefined
      ? projects.find(p => p.id === pid)?.name
      : undefined
  const getMember = (userId: string | null | undefined) =>
    userId !== null && userId !== undefined ? members.get(userId) : undefined
  const hasFilters =
    !!filters.search || !!filters.feed_id || !!filters.project_id

  return {
    tab,
    feeds,
    projects,
    members,
    getMember,
    assistants,
    itemsState,
    searchInput,
    setSearchInput,
    filters,
    setFilters,
    feedToDelete,
    setFeedToDelete,
    deletingFeed,
    itemToDelete,
    setItemToDelete,
    deletingItem,
    fetchItems,
    handleTabChange,
    handleArchiveItem,
    handleUnarchiveItem,
    handleDeleteItem,
    handleDeleteFeed,
    getFeedName,
    getProjectName,
    hasFilters,
    activeCount,
    archivedCount,
  }
}
