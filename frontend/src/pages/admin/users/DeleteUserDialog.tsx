import { AlertTriangle, Users } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type {
  AdminUserDeleteBlockedResponse,
  AdminUserDetail,
} from '@/services/adminService'

export interface DeleteUserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: AdminUserDetail
  /**
   * The server's refusal, when the delete came back 409. Its presence switches
   * the dialog from "confirm" to "blocked" — the 409 body is documented data, not
   * an error string, so it is rendered rather than toasted.
   */
  refusal: AdminUserDeleteBlockedResponse | null
  deleting: boolean
  onConfirm: () => void
  /** Offered as the reversible alternative from the confirm state. */
  onSuspendInstead?: () => void
  canSuspendInstead: boolean
}

/**
 * Confirmation for the only destructive action in the admin portal.
 *
 * Two states:
 * - **confirm** — names the user, spells out exactly what is removed, and offers
 *   suspension as the reversible alternative. The destructive button is not the
 *   default focus, so Enter cannot delete an account.
 * - **blocked** — the server refused (409) and *nothing was deleted*. The teams
 *   that blocked it are listed with their member counts, and there is no
 *   destructive button at all: the way forward is transferring ownership, which
 *   this dialog cannot do for you.
 */
export function DeleteUserDialog({
  open,
  onOpenChange,
  user,
  refusal,
  deleting,
  onConfirm,
  onSuspendInstead,
  canSuspendInstead,
}: Readonly<DeleteUserDialogProps>) {
  const label = user.name || user.email
  const blocked = refusal !== null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {blocked ? 'Cannot delete this user' : `Delete ${label}?`}
          </DialogTitle>
          <DialogDescription>
            {blocked
              ? 'Nothing was deleted. The account and every team below still exist.'
              : 'This permanently removes the account. It cannot be undone.'}
          </DialogDescription>
        </DialogHeader>

        {blocked ? (
          <div className="space-y-3">
            <Alert>
              <AlertTriangle className="size-4" />
              <AlertTitle>Ownership must be transferred first</AlertTitle>
              <AlertDescription>{refusal.message}</AlertDescription>
            </Alert>
            {refusal.blockers.length > 0 && (
              <div className="space-y-1.5">
                <p className="text-sm font-medium">
                  Shared teams owned by {label}
                </p>
                <ul className="divide-y rounded-md border">
                  {refusal.blockers.map(blocker => (
                    <li
                      key={blocker.team_id}
                      className="flex items-center justify-between px-3 py-2 text-sm"
                    >
                      <span>{blocker.team_name}</span>
                      <span className="text-muted-foreground flex items-center gap-1 text-xs tabular-nums">
                        <Users className="size-3.5" aria-hidden />
                        {blocker.member_count} members
                      </span>
                    </li>
                  ))}
                </ul>
                <p className="text-muted-foreground text-xs">
                  Transfer each team to another owner, then delete the account.
                </p>
              </div>
            )}
          </div>
        ) : (
          <div className="space-y-3 text-sm">
            <div>
              <p className="font-medium">This will remove:</p>
              <ul className="text-muted-foreground mt-1 list-inside list-disc space-y-1">
                <li>
                  the account <span className="font-mono">{user.email}</span>{' '}
                  and its sessions, API keys and tokens
                </li>
                <li>
                  their personal workspace and everything in it — prompts,
                  memories, artifacts and blueprints
                </li>
                <li>
                  their membership of{' '}
                  {user.memberships.length === 1
                    ? '1 team'
                    : `${String(user.memberships.length)} teams`}{' '}
                  (the teams themselves stay)
                </li>
              </ul>
            </div>
            {canSuspendInstead && (
              <Alert>
                <AlertTitle>Consider suspending instead</AlertTitle>
                <AlertDescription>
                  Suspending blocks every way this account can sign in —
                  sessions, API keys and tokens stop working immediately — and
                  can be undone at any time.
                </AlertDescription>
              </Alert>
            )}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              onOpenChange(false)
            }}
            disabled={deleting}
          >
            {/* "Done" rather than "Close": DialogContent already renders an X
                with the accessible name "Close", and two controls sharing a name
                is a needless ambiguity for screen-reader users. */}
            {blocked ? 'Done' : 'Cancel'}
          </Button>
          {!blocked && canSuspendInstead && (
            <Button
              variant="secondary"
              onClick={onSuspendInstead}
              disabled={deleting}
            >
              Suspend instead
            </Button>
          )}
          {!blocked && (
            <Button
              variant="destructive"
              onClick={onConfirm}
              disabled={deleting}
            >
              {deleting ? 'Deleting…' : 'Delete permanently'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
