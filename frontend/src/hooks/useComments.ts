import { useCallback, useEffect, useRef, useState } from 'react'

import type { Comment, CommentResourceType } from '@/services/commentService'
import { commentService } from '@/services/commentService'
import type { TeamMember } from '@/services/teamService'
import { teamService } from '@/services/teamService'

/**
 * Comments are fetched five-at-a-time: the sidebar widget shows the first page
 * and the "all comments" popup appends further pages via `loadMore`.
 */
const COMMENTS_PAGE_SIZE = 5

export interface UseCommentsResult {
  comments: Comment[]
  /** user_id → team member, for resolving author name/avatar (feed-reply pattern). */
  members: Map<string, TeamMember>
  totalCount: number
  /** Initial page load in flight. */
  loading: boolean
  /** A `loadMore` append is in flight. */
  loadingMore: boolean
  /** True while the initial load failed and nothing is shown. */
  error: boolean
  /** More pages remain to append. */
  hasMore: boolean
  reload: () => void
  loadMore: () => void
  addComment: (content: string) => Promise<void>
  editComment: (commentId: string, content: string) => Promise<void>
  removeComment: (commentId: string) => Promise<void>
}

/**
 * Self-contained data layer for a resource's comments: paginated read with
 * append-on-loadMore (deduped by id, terminates on the page count derived from
 * `totalCount`) and optimistic add/edit/delete that keep `comments`/`totalCount`
 * in sync so the widget and popup — which share one instance — always agree.
 */
export function useComments(
  teamId: string,
  resourceType: CommentResourceType,
  resourceId: string
): UseCommentsResult {
  const [comments, setComments] = useState<Comment[]>([])
  const [members, setMembers] = useState<Map<string, TeamMember>>(new Map())
  const [totalCount, setTotalCount] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState(false)

  // Bumped on every (re)load. The resourceId prop can change in place (navigating
  // between two detail pages reuses this component), so a slower earlier request
  // for a now-stale resource must not overwrite the current one.
  const seqRef = useRef(0)
  const loadingMoreRef = useRef(false)

  const loadFirst = useCallback(() => {
    const seq = ++seqRef.current
    setLoading(true)
    setError(false)
    const run = async () => {
      const [listResult, membersResult] = await Promise.allSettled([
        commentService.list(
          teamId,
          resourceType,
          resourceId,
          1,
          COMMENTS_PAGE_SIZE
        ),
        teamService.getTeamMembers(teamId),
      ])
      if (seq !== seqRef.current) return // superseded by a newer load
      if (listResult.status === 'fulfilled') {
        setComments(listResult.value.comments)
        setTotalCount(listResult.value.total_count)
        setPage(1)
      } else {
        setComments([])
        setTotalCount(0)
        setPage(0)
        setError(true)
      }
      if (membersResult.status === 'fulfilled') {
        setMembers(new Map(membersResult.value.map(m => [m.user_id, m])))
      }
      setLoading(false)
    }
    void run()
  }, [teamId, resourceType, resourceId])

  useEffect(() => {
    loadFirst()
  }, [loadFirst])

  const hasMore = page < Math.ceil(totalCount / COMMENTS_PAGE_SIZE)

  const loadMore = useCallback(() => {
    if (loadingMoreRef.current) return
    if (page >= Math.ceil(totalCount / COMMENTS_PAGE_SIZE)) return
    loadingMoreRef.current = true
    setLoadingMore(true)
    const seq = seqRef.current
    const run = async () => {
      try {
        const next = page + 1
        const res = await commentService.list(
          teamId,
          resourceType,
          resourceId,
          next,
          COMMENTS_PAGE_SIZE
        )
        if (seq !== seqRef.current) return // resource changed mid-flight
        setComments(prev => {
          const seen = new Set(prev.map(c => c.id))
          return [...prev, ...res.comments.filter(c => !seen.has(c.id))]
        })
        setTotalCount(res.total_count)
        setPage(next)
      } finally {
        loadingMoreRef.current = false
        setLoadingMore(false)
      }
    }
    void run()
  }, [teamId, resourceType, resourceId, page, totalCount])

  const addComment = useCallback(
    async (content: string) => {
      const created = await commentService.create(teamId, {
        resource_type: resourceType,
        resource_id: resourceId,
        content,
      })
      setComments(prev => [created, ...prev])
      setTotalCount(t => t + 1)
    },
    [teamId, resourceType, resourceId]
  )

  const editComment = useCallback(
    async (commentId: string, content: string) => {
      const updated = await commentService.update(teamId, commentId, {
        content,
      })
      setComments(prev => prev.map(c => (c.id === commentId ? updated : c)))
    },
    [teamId]
  )

  const removeComment = useCallback(
    async (commentId: string) => {
      await commentService.remove(teamId, commentId)
      setComments(prev => prev.filter(c => c.id !== commentId))
      setTotalCount(t => Math.max(0, t - 1))
    },
    [teamId]
  )

  return {
    comments,
    members,
    totalCount,
    loading,
    loadingMore,
    error,
    hasMore,
    reload: loadFirst,
    loadMore,
    addComment,
    editComment,
    removeComment,
  }
}
