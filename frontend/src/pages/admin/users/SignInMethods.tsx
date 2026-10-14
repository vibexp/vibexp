import { Badge } from '@/components/ui/badge'
import type { AdminUserDetail } from '@/services/adminService'

/**
 * How an account can sign in (#1443): its identity providers by slug and its
 * password, a disabled password marked as such, and "Pending setup" when it
 * has no way in yet.
 */
export function SignInMethods({
  user,
}: Readonly<{
  user: Pick<
    AdminUserDetail,
    'sign_in_methods' | 'password_disabled' | 'pending_setup'
  >
}>) {
  if (user.pending_setup) {
    return (
      <Badge variant="outline" className="font-normal">
        Pending setup
      </Badge>
    )
  }
  if (user.sign_in_methods.length === 0) {
    return <p className="text-sm">—</p>
  }
  return (
    <div className="flex flex-wrap gap-1">
      {user.sign_in_methods.map(method =>
        method === 'password' ? (
          <Badge
            key={method}
            variant={user.password_disabled ? 'destructive' : 'secondary'}
            className="font-normal"
          >
            {user.password_disabled ? 'Password (disabled)' : 'Password'}
          </Badge>
        ) : (
          <Badge key={method} variant="secondary" className="font-normal">
            {method}
          </Badge>
        )
      )}
    </div>
  )
}
