import { Ban, Pencil, Trash2, UserCheck } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { PageHeader } from '@/components/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useAuth } from '@/contexts/useAuth'
import { formatDate } from '@/lib/time'
import { AdminDetailScaffold } from '@/pages/admin/AdminDetailScaffold'
import {
  actionBlockedReason,
  canActOn,
} from '@/pages/admin/users/adminUserGuards'
import { DeleteUserDialog } from '@/pages/admin/users/DeleteUserDialog'
import type { UserFormValues } from '@/pages/admin/users/UserFormDialog'
import { UserFormDialog } from '@/pages/admin/users/UserFormDialog'
import type {
  AdminUserDeleteBlockedResponse,
  AdminUserDetail as AdminUserDetailType,
} from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

/** Instance user detail — profile, memberships, and the admin actions (#459). */
export function AdminUserDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { user: actingAdmin } = useAuth()
  const [user, setUser] = useState<AdminUserDetailType | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [editOpen, setEditOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editError, setEditError] = useState<string | null>(null)

  const [statusDialogOpen, setStatusDialogOpen] = useState(false)
  const [statusWorking, setStatusWorking] = useState(false)

  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [refusal, setRefusal] = useState<AdminUserDeleteBlockedResponse | null>(
    null
  )

  const load = useCallback(() => {
    if (!id) return
    setLoading(true)
    setError(null)
    adminService
      .getUser(id)
      .then(result => {
        setUser(result)
      })
      .catch((err: unknown) => {
        setError(getErrorMessage(err, 'Failed to load user'))
      })
      .finally(() => {
        setLoading(false)
      })
  }, [id])

  useEffect(() => {
    load()
  }, [load])

  const suspended = user?.status === 'suspended'
  const actionable = canActOn(user, actingAdmin)
  const blockedReason = actionBlockedReason(user, actingAdmin)

  const handleEdit = (values: UserFormValues) => {
    if (!user) return
    setSaving(true)
    setEditError(null)
    adminService
      .updateUser(user.id, { name: values.name })
      .then(updated => {
        setUser(updated)
        setEditOpen(false)
        toast.success('User updated')
      })
      .catch((err: unknown) => {
        setEditError(getErrorMessage(err, 'Failed to update the user'))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  const handleStatusChange = () => {
    if (!user) return
    setStatusWorking(true)
    const action = suspended
      ? adminService.reactivateUser(user.id)
      : adminService.suspendUser(user.id)
    action
      .then(updated => {
        setUser(updated)
        setStatusDialogOpen(false)
        toast.success(suspended ? 'User reactivated' : 'User suspended')
      })
      .catch((err: unknown) => {
        // Also covers the server's 409 for a config-listed instance admin, which
        // the client cannot identify up front — the reason comes from the API
        // rather than from a guess made here.
        toast.error(
          getErrorMessage(
            err,
            suspended ? 'Failed to reactivate' : 'Failed to suspend'
          )
        )
      })
      .finally(() => {
        setStatusWorking(false)
      })
  }

  const handleDelete = () => {
    if (!user) return
    setDeleting(true)
    setRefusal(null)
    adminService
      .deleteUser(user.id)
      .then(result => {
        // Narrowed with `in` rather than on `result.deleted`: the two are
        // equivalent under tsc's project build but ts-jest's config does not
        // narrow the boolean discriminant, and the test suite would not compile.
        if (!('refusal' in result)) {
          toast.success('User deleted')
          void navigate('/admin/users')
          return
        }
        // A refusal keeps the dialog open and switches it to the blocked state.
        // Nothing was deleted, and the blocker list is the actionable part.
        setRefusal(result.refusal)
      })
      .catch((err: unknown) => {
        toast.error(getErrorMessage(err, 'Failed to delete the user'))
      })
      .finally(() => {
        setDeleting(false)
      })
  }

  return (
    <AdminDetailScaffold
      backTo="/admin/users"
      backLabel="Back to users"
      loading={loading}
      error={error}
      errorTitle="Failed to load user"
    >
      {user && (
        <>
          <PageHeader
            title={user.name || user.email}
            description={user.name ? user.email : undefined}
            actions={
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setEditError(null)
                    setEditOpen(true)
                  }}
                >
                  <Pencil className="mr-2 size-4" aria-hidden />
                  Edit
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!actionable}
                  title={blockedReason ?? undefined}
                  onClick={() => {
                    setStatusDialogOpen(true)
                  }}
                >
                  {suspended ? (
                    <UserCheck className="mr-2 size-4" aria-hidden />
                  ) : (
                    <Ban className="mr-2 size-4" aria-hidden />
                  )}
                  {suspended ? 'Reactivate' : 'Suspend'}
                </Button>
                <Button
                  variant="destructive"
                  size="sm"
                  disabled={!actionable}
                  title={blockedReason ?? undefined}
                  onClick={() => {
                    setDeleteOpen(true)
                  }}
                >
                  <Trash2 className="mr-2 size-4" aria-hidden />
                  Delete
                </Button>
              </div>
            }
          />

          {blockedReason && (
            <p className="text-muted-foreground text-sm">{blockedReason}</p>
          )}

          <Card>
            <CardContent className="grid grid-cols-1 gap-4 py-4 sm:grid-cols-4">
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
              <div>
                <p className="text-muted-foreground text-xs">Joined</p>
                <p className="text-sm">{formatDate(user.created_at)}</p>
              </div>
              <div>
                <p className="text-muted-foreground text-xs">Teams</p>
                <p className="text-sm tabular-nums">
                  {user.memberships.length}
                </p>
              </div>
            </CardContent>
          </Card>

          <div className="space-y-2">
            <h2 className="text-sm font-semibold">Team memberships</h2>
            {user.memberships.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                This user is not a member of any team.
              </p>
            ) : (
              <Card className="overflow-hidden">
                <Table>
                  <TableHeader>
                    <TableRow className="bg-muted/40 hover:bg-muted/40">
                      <TableHead className="h-9 text-xs font-medium">
                        Team
                      </TableHead>
                      <TableHead className="h-9 text-xs font-medium">
                        Role
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {user.memberships.map(m => (
                      <TableRow key={m.team_id}>
                        <TableCell className="py-3 text-sm">
                          {m.team_name}
                        </TableCell>
                        <TableCell className="py-3">
                          <Badge variant="outline">{m.role}</Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Card>
            )}
          </div>

          <UserFormDialog
            open={editOpen}
            onOpenChange={setEditOpen}
            mode="edit"
            initial={{ name: user.name, email: user.email }}
            submitting={saving}
            error={editError}
            onSubmit={handleEdit}
          />

          <ConfirmDialog
            open={statusDialogOpen}
            onOpenChange={setStatusDialogOpen}
            title={suspended ? 'Reactivate this user?' : 'Suspend this user?'}
            description={
              suspended
                ? `${user.email} will be able to sign in again, and their existing API keys and tokens start working immediately.`
                : `${user.email} will be signed out everywhere. Sessions, API keys and MCP tokens stop working immediately, not at expiry. This does not disable their account at your identity provider, and you can undo it at any time.`
            }
            confirmLabel={suspended ? 'Reactivate' : 'Suspend'}
            variant={suspended ? 'default' : 'destructive'}
            loading={statusWorking}
            onConfirm={handleStatusChange}
          />

          <DeleteUserDialog
            open={deleteOpen}
            onOpenChange={open => {
              setDeleteOpen(open)
              // Cleared on close, not on open: tying it to the dialog's lifecycle
              // means every way of dismissing it — the footer button, the X,
              // Escape, an outside click — leaves a clean state, rather than
              // relying on each opener to remember.
              if (!open) setRefusal(null)
            }}
            user={user}
            refusal={refusal}
            deleting={deleting}
            onConfirm={handleDelete}
            canSuspendInstead={!suspended}
            onSuspendInstead={() => {
              setDeleteOpen(false)
              setStatusDialogOpen(true)
            }}
          />
        </>
      )}
    </AdminDetailScaffold>
  )
}
