import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { formatDate } from '@/lib/time'
import type { AdminUserDetail } from '@/services/adminService'

import { SignInMethods } from './SignInMethods'

/** The facts row at the top of a user page: status, provider, sign-in methods (#1443), joined and teams. */
export function UserSummaryCard({ user }: Readonly<{ user: AdminUserDetail }>) {
  const suspended = user.status === 'suspended'
  return (
    <Card>
      <CardContent className="grid grid-cols-1 gap-4 py-4 sm:grid-cols-5">
        <div>
          <p className="text-muted-foreground text-xs">Status</p>
          <div className="text-sm">
            {suspended ? (
              <Badge variant="destructive" className="font-normal">
                Suspended
              </Badge>
            ) : (
              <Badge variant="secondary" className="font-normal">
                Active
              </Badge>
            )}
          </div>
        </div>
        <div>
          <p className="text-muted-foreground text-xs">Provider</p>
          <p className="text-sm">{user.idp_provider ?? '—'}</p>
        </div>
        <div data-testid="user-sign-in-methods">
          <p className="text-muted-foreground text-xs">Sign-in methods</p>
          <SignInMethods user={user} />
        </div>
        <div>
          <p className="text-muted-foreground text-xs">Joined</p>
          <p className="text-sm">{formatDate(user.created_at)}</p>
        </div>
        <div>
          <p className="text-muted-foreground text-xs">Teams</p>
          <p className="text-sm tabular-nums">{user.memberships.length}</p>
        </div>
      </CardContent>
    </Card>
  )
}
