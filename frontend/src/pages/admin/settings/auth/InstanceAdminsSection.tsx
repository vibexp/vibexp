import { UserPlus } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { formatDateTime } from '@/lib/time'
import { toast } from '@/lib/toast'
import { ownerEmailParam } from '@/pages/admin/filters/advancedFilterParams'
import {
  type AdminInstanceAdmin,
  type AdminInstanceAdminList,
  authSettingsService,
} from '@/services/authSettingsService'

import { FieldFrame } from '../InstanceSettingsFields'
import { fieldDescribedBy } from '../instanceSettingsForm'
import { messageOf } from './authSettingsForm'

/**
 * Instance admins (#1239): the root admins set in the server configuration,
 * read-only, and the admins granted in the database.
 *
 * Only a root admin may grant or revoke, and the server refuses anyone else
 * (403), so the controls are rendered only when `canManage` is set — from the
 * server's `is_root_instance_admin` flag, never from local role logic.
 * `onChanged` runs after each stored change.
 */
export function InstanceAdminsSection({
  canManage,
  onChanged,
}: Readonly<{ canManage: boolean; onChanged?: () => void }>) {
  const [list, setList] = useState<AdminInstanceAdminList | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [email, setEmail] = useState('')
  const [emailError, setEmailError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [revoking, setRevoking] = useState<AdminInstanceAdmin | null>(null)

  const load = useCallback(async (): Promise<void> => {
    try {
      setLoading(true)
      setLoadError(null)
      setList(await authSettingsService.listAdmins())
    } catch (err) {
      setLoadError(messageOf(err, 'Failed to load the instance admins'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  if (loading && !list) {
    return (
      <div className="flex justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
    )
  }

  if (!list) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>
          {loadError ?? 'Failed to load the instance admins'}
        </AlertDescription>
      </Alert>
    )
  }

  const grant = async () => {
    const address = ownerEmailParam(email)
    if (address === undefined) {
      setEmailError('Enter the full email address of an existing user.')
      return
    }
    try {
      setBusy(true)
      setActionError(null)
      const granted = await authSettingsService.grantAdmin({ email: address })
      setEmail('')
      toast.success(`${granted.email} is now an instance admin`)
    } catch (err) {
      setActionError(messageOf(err, 'Failed to grant instance admin'))
      return
    } finally {
      setBusy(false)
    }
    onChanged?.()
    await load()
  }

  const revoke = async (admin: AdminInstanceAdmin) => {
    try {
      setBusy(true)
      setActionError(null)
      await authSettingsService.revokeAdmin(admin.user_id)
      toast.success(`${admin.email} is no longer an instance admin`)
    } catch (err) {
      setActionError(messageOf(err, 'Failed to revoke instance admin'))
      return
    } finally {
      setBusy(false)
      setRevoking(null)
    }
    onChanged?.()
    await load()
  }

  return (
    <Card data-testid="auth-admins-section">
      <CardHeader>
        <CardTitle>Instance admins</CardTitle>
        <CardDescription>
          Who can open this admin portal. Root admins are set in the server
          configuration and cannot be changed here; only a root admin can grant
          or revoke the others.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {loadError && (
          <Alert variant="destructive" data-testid="auth-admins-load-error">
            <AlertTitle>Could not refresh the instance admins</AlertTitle>
            <AlertDescription>
              {loadError} The list below may be out of date.
            </AlertDescription>
          </Alert>
        )}

        <ul className="divide-y">
          {list.root_admins.map(root => (
            <li
              key={`root:${root}`}
              className="flex flex-wrap items-center gap-2 py-2 text-sm"
              data-testid="auth-root-admin"
            >
              <span className="font-medium">{root}</span>
              <Badge variant="secondary">config</Badge>
              <span className="text-muted-foreground">Root admin</span>
            </li>
          ))}
          {list.admins.map(admin => (
            <li
              key={admin.user_id}
              className="flex flex-wrap items-center gap-2 py-2 text-sm"
              data-testid="auth-db-admin"
            >
              <span className="font-medium">{admin.name ?? admin.email}</span>
              {admin.name && (
                <span className="text-muted-foreground">{admin.email}</span>
              )}
              <span className="text-muted-foreground">
                granted {formatDateTime(admin.granted_at)}
              </span>
              {canManage && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="ml-auto"
                  disabled={busy}
                  aria-label={`Remove ${admin.email}`}
                  onClick={() => {
                    setRevoking(admin)
                  }}
                >
                  Remove
                </Button>
              )}
            </li>
          ))}
        </ul>
        {list.admins.length === 0 && (
          <p
            className="text-muted-foreground text-sm"
            data-testid="auth-admins-empty"
          >
            No instance admin has been granted beyond the root admins.
          </p>
        )}

        {actionError && (
          <Alert variant="destructive" data-testid="auth-admins-error">
            <AlertTitle>Not saved</AlertTitle>
            <AlertDescription>{actionError}</AlertDescription>
          </Alert>
        )}

        {canManage ? (
          <form
            aria-label="Grant instance admin"
            className="flex flex-wrap items-end gap-2"
            noValidate
            onSubmit={event => {
              event.preventDefault()
              void grant()
            }}
          >
            <div className="min-w-64 flex-1">
              <FieldFrame
                id="grant-admin-email"
                label="Grant instance admin to"
                hint="The email address of an existing, active user."
                error={emailError ?? undefined}
              >
                <Input
                  id="grant-admin-email"
                  type="email"
                  autoComplete="off"
                  placeholder="ada@example.com"
                  value={email}
                  disabled={busy}
                  aria-invalid={emailError ? true : undefined}
                  aria-describedby={fieldDescribedBy(
                    'grant-admin-email',
                    true,
                    emailError
                  )}
                  onChange={event => {
                    setEmail(event.target.value)
                    setEmailError(null)
                  }}
                />
              </FieldFrame>
            </div>
            <Button
              type="submit"
              size="sm"
              disabled={busy || email.trim() === ''}
            >
              <UserPlus className="mr-2 size-4" />
              Grant
            </Button>
          </form>
        ) : (
          <p
            className="text-muted-foreground text-sm"
            data-testid="auth-admins-readonly"
          >
            Only a root admin can grant or revoke instance admin.
          </p>
        )}
      </CardContent>

      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={open => {
          if (!open) setRevoking(null)
        }}
        title={`Remove ${revoking?.email ?? 'this admin'} as an instance admin?`}
        description="They keep their account and their teams, and lose access to this admin portal on their next request."
        confirmLabel="Remove admin"
        variant="destructive"
        loading={busy}
        onConfirm={async () => {
          if (revoking) await revoke(revoking)
        }}
      />
    </Card>
  )
}
