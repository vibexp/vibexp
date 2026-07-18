import type { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'

import { useAuth } from '@/contexts/useAuth'

/**
 * Route guard for the instance-admin portal.
 *
 * Renders its children only for a signed-in instance admin. It **fails closed**:
 * while `/auth/me` is still resolving (`isLoading`) it renders a neutral loading
 * state rather than the children, so admin content never flashes before the
 * `is_instance_admin` flag is known. Non-admins (and logged-out users, whose
 * `user` is null) are redirected to `/`.
 *
 * UI gating is convenience only — every `/api/v1/admin/*` call is authorized
 * server-side regardless (epic #309 / authz matrix).
 */
export function RequireInstanceAdmin({
  children,
}: Readonly<{ children: ReactNode }>) {
  const { user, isLoading } = useAuth()

  if (isLoading) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center">
        <div className="text-muted-foreground text-sm">Loading…</div>
      </div>
    )
  }

  if (!user?.is_instance_admin) {
    return <Navigate to="/" replace />
  }

  return <>{children}</>
}
