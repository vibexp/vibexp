import { useCallback, useEffect, useRef, useState } from 'react'

import { useErrorHandler } from '@/hooks/useErrorHandler'
import type { FeedItemReply } from '@/services/feedService'
import { feedService } from '@/services/feedService'
import type { TeamMember } from '@/services/teamService'
import { teamService } from '@/services/teamService'

/**
 * Replies are fetched ten-at-a-time: the details-column panel shows the first
 * page and the "all replies" popup appends further pages via `loadMore`. The
 * page size is always sent explicitly rather than relying on the server's 20.
 */
export const REPLIES_PAGE_SIZE = 10

/** Appends a fetched page to the list, dropping replies already present (dedup by id). */
function appendPageDeduped(
  prev: FeedItemReply[],
  incoming: FeedItemReply[]
): FeedItemReply[] {
  const seen = new Set(prev.map(r => r.id))
  return [...prev, ...incoming.filter(r => !seen.has(r.id))]
}

export interface UseFeedRepliesResult {
  replies: FeedItemReply[]
  /** user_id → team member, for resolving the reply author's name/avatar. */
  members: Map<string, TeamMember>
  totalCount: number
  /** Initial page load in flight. */
  loading: boolean
  /** A `loadMore` append is in flight. */
  loadingMore: boolean
  /** More pages remain to append. */
  hasMore: boolean
  loadMore: () => void
  addReply: (content: string) => Promise<void>
}

/**
 * Self-contained data layer for a feed item's replies — the feed-reply
 * counterpart of `useComments`: paginated read with append-on-loadMore (deduped
 * by id, terminating on the page count derived from `totalCount`) and an
 * optimistic add that keeps `replies`/`totalCount` in sync, so the panel and the
 * popup — which share one instance — always agree.
 */
export function useFeedReplies(
  teamId: string,
  itemId: string
): UseFeedRepliesResult {
  const { handleError } = useErrorHandler()

  const [replies, setReplies] = useState<FeedItemReply[]>([])
  const [members, setMembers] = useState<Map<string, TeamMember>>(new Map())
  const [totalCount, setTotalCount] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)

  // Bumped on every (re)load. The itemId prop can change in place (navigating
  // between two feed items reuses this component), so a slower earlier request
  // for a now-stale item must not overwrite the current one.
  const seqRef = useRef(0)
  const loadingMoreRef = useRef(false)

  useEffect(() => {
    const seq = ++seqRef.current
    setLoading(true)
    const run = async () => {
      const [repliesResult, membersResult] = await Promise.allSettled([
        feedService.listReplies(teamId, itemId, 1, REPLIES_PAGE_SIZE),
        teamService.getTeamMembers(teamId),
      ])
      if (seq !== seqRef.current) return // superseded by a newer load
      if (repliesResult.status === 'fulfilled') {
        setReplies(repliesResult.value.replies)
        setTotalCount(repliesResult.value.total_count)
        setPage(1)
      } else {
        setReplies([])
        setTotalCount(0)
        setPage(0)
      }
      if (membersResult.status === 'fulfilled') {
        setMembers(new Map(membersResult.value.map(m => [m.user_id, m])))
      }
      setLoading(false)
    }
    void run()
  }, [teamId, itemId])

  const hasMore = page < Math.ceil(totalCount / REPLIES_PAGE_SIZE)

  const loadMore = useCallback(() => {
    if (loadingMoreRef.current) return
    if (page >= Math.ceil(totalCount / REPLIES_PAGE_SIZE)) return
    loadingMoreRef.current = true
    setLoadingMore(true)
    const seq = seqRef.current
    const run = async () => {
      try {
        const next = page + 1
        const res = await feedService.listReplies(
          teamId,
          itemId,
          next,
          REPLIES_PAGE_SIZE
        )
        if (seq !== seqRef.current) return // item changed mid-flight
        setReplies(prev => appendPageDeduped(prev, res.replies))
        setTotalCount(res.total_count)
        setPage(next)
      } catch (err) {
        handleError(err, 'Failed to load more replies')
      } finally {
        loadingMoreRef.current = false
        setLoadingMore(false)
      }
    }
    void run()
  }, [teamId, itemId, page, totalCount, handleError])

  const addReply = useCallback(
    async (content: string) => {
      const seq = seqRef.current
      const created = await feedService.createReply(teamId, itemId, {
        content,
      })
      if (seq !== seqRef.current) return // item changed while posting
      setReplies(prev => [created, ...prev])
      setTotalCount(t => t + 1)
    },
    [teamId, itemId]
  )

  return {
    replies,
    members,
    totalCount,
    loading,
    loadingMore,
    hasMore,
    loadMore,
    addReply,
  }
}
