import { useCallback, useMemo } from 'react'

import { useTeam } from '@/contexts/TeamContext'
import { useAuth } from '@/contexts/useAuth'
import type { Team } from '@/services/teamService'

/**
 * A permission the server may grant in a team.
 *
 * Derived from the generated client, so this union IS the backend's matrix
 * (epic #220) — a typo or a permission the server does not publish is a
 * compile error, not a silently-false check.
 */
export type TeamPermission = Team['permissions'][number]

export interface UsePermissionsResult {
  /** Whether the current team grants `permission` to the current user. */
  can: (permission: TeamPermission) => boolean
  /**
   * Whether the user may delete a resource (prompt/memory/artifact/blueprint/
   * agent) created by `resourceOwnerId`: their own with `resource.delete.own`,
   * anyone's with `resource.delete.any`.
   */
  canDeleteResource: (resourceOwnerId: string | undefined) => boolean
  /**
   * Whether the user may delete feed content (a post or a whole feed) created
   * by `createdByUserId`: their own with `resource.delete.own`, someone else's
   * with `feed.delete.any` (moderation).
   *
   * This mirrors the backend's rule for both `DeleteFeedItem` and `DeleteFeed`,
   * which pass the same (`resource.delete.own`, `feed.delete.any`) pair to
   * CanActOnResource — note the "own" half is the generic resource permission;
   * there is no `feed.delete.own`.
   */
  canDeleteFeedContent: (createdByUserId: string | undefined) => boolean
}

/**
 * Reads a team's permissions, as computed by the server.
 *
 * The role matrix lives on the backend and is published verbatim on every team
 * payload (#224). This hook is the only place the SPA reads it, and it never
 * re-derives permissions from `role` — that would duplicate the matrix in two
 * places and guarantee drift the moment the server's changes.
 *
 * By default it reads the *current* team, which is what team-scoped pages
 * (projects, resources, feeds) operate on. Pass `team` explicitly when the page
 * acts on a team it fetched itself rather than the ambient one — the team
 * settings page can show any team you belong to, and that team's permissions,
 * not the current team's, decide what you may do to it. Passing `null` (e.g.
 * while loading) permits nothing rather than silently falling back.
 *
 * Gating with this hook is a convenience, not a security boundary: the backend
 * authorizes every write regardless. A 403 that slips through (e.g. the role
 * changed since the team was fetched) is handled by useErrorHandler.
 *
 * Fails closed: with no team, nothing is permitted.
 */
export function usePermissions(team?: Team | null): UsePermissionsResult {
  const { currentTeam } = useTeam()
  const { user } = useAuth()
  const userId = user?.id

  const source = team === undefined ? currentTeam : team
  const permissions = source?.permissions

  const granted = useMemo(
    () => new Set<TeamPermission>(permissions ?? []),
    [permissions]
  )

  const can = useCallback(
    (permission: TeamPermission) => granted.has(permission),
    [granted]
  )

  const canDeleteResource = useCallback(
    (resourceOwnerId: string | undefined) => {
      if (granted.has('resource.delete.any')) return true
      if (!resourceOwnerId || !userId) return false
      return resourceOwnerId === userId && granted.has('resource.delete.own')
    },
    [granted, userId]
  )

  const canDeleteFeedContent = useCallback(
    (createdByUserId: string | undefined) => {
      const isAuthor =
        !!createdByUserId && !!userId && createdByUserId === userId
      return granted.has(isAuthor ? 'resource.delete.own' : 'feed.delete.any')
    },
    [granted, userId]
  )

  return { can, canDeleteResource, canDeleteFeedContent }
}
