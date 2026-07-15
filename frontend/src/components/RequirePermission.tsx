import type { ReactNode } from 'react'

import type { TeamPermission } from '@/hooks/usePermissions'
import { usePermissions } from '@/hooks/usePermissions'

interface RequirePermissionProps {
  /** The permission the current team must grant for `children` to render. */
  permission: TeamPermission
  children: ReactNode
  /** Rendered instead when the permission is absent. Defaults to nothing. */
  fallback?: ReactNode
}

/**
 * Renders `children` only if the current team grants `permission` (#225).
 *
 * Hiding rather than disabling matches the existing convention in the teams UI
 * (`TeamMembersList` suppresses actions outright), and a disabled button
 * invites "why can't I click this?" without answering it.
 *
 * Use this for whole UI regions. For a single conditional inside a component
 * that already calls the hook — or where the decision needs the resource's
 * owner, as with own-vs-any delete — prefer `usePermissions()` directly.
 */
export function RequirePermission({
  permission,
  children,
  fallback = null,
}: RequirePermissionProps) {
  const { can } = usePermissions()

  return <>{can(permission) ? children : fallback}</>
}
