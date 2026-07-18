import { AlertCircle, Crown } from 'lucide-react'
import { useState } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'
import type { Team, TeamMember } from '@/services/teamService'
import { teamService } from '@/services/teamService'
import { ApiError } from '@/types/errors'

interface TransferOwnershipModalProps {
  isOpen: boolean
  team: Team
  members: TeamMember[]
  onClose: () => void
  onSuccess: () => Promise<void> | void
}

function transferErrorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.getMessage()
  if (err instanceof Error) return err.message
  return 'Failed to transfer ownership'
}

function transferButtonLabel(
  isTransferring: boolean,
  selected: TeamMember | undefined
): string {
  if (isTransferring) return 'Transferring…'
  if (selected) return `Make ${selected.name} owner`
  return 'Select a member'
}

/**
 * Hands team ownership to another member (#225).
 *
 * The caller is demoted to admin in the same transaction, so this is
 * irreversible from their side — hence a target that must be picked explicitly
 * and a confirmation that names them.
 *
 * The target list is rendered as buttons rather than a Select: a Radix Select
 * inside a Radix Dialog deadlocks the focus scope under jsdom, which makes the
 * picker untestable (and takes the test worker down with it).
 */
export function TransferOwnershipModal({
  isOpen,
  team,
  members,
  onClose,
  onSuccess,
}: Readonly<TransferOwnershipModalProps>) {
  const [selectedUserId, setSelectedUserId] = useState<string | null>(null)
  const [isTransferring, setIsTransferring] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Only existing members can receive ownership: the current owner is already
  // there, and a pending invitee has no membership for the backend to promote.
  const candidates = members.filter(
    member => member.role !== 'owner' && member.invitation_status !== 'pending'
  )
  const selected = candidates.find(member => member.user_id === selectedUserId)

  const reset = () => {
    setSelectedUserId(null)
    setError(null)
  }

  const handleClose = () => {
    if (isTransferring) return
    reset()
    onClose()
  }

  const handleTransfer = async () => {
    if (!selected) return
    setError(null)

    try {
      setIsTransferring(true)
      await teamService.transferOwnership(team.id, selected.user_id)
      toast.success(`${selected.name} is now the owner of ${team.name}`)
      // The caller is an admin now, so their own permissions changed — the
      // parent refreshes the team before this dialog unmounts.
      await onSuccess()
      reset()
      onClose()
    } catch (err) {
      const message = transferErrorMessage(err)
      setError(message)
      toast.error(message)
    } finally {
      setIsTransferring(false)
    }
  }

  return (
    <Dialog
      open={isOpen}
      onOpenChange={open => {
        if (!open) handleClose()
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Crown className="size-5" />
            Transfer ownership of &quot;{team.name}&quot;
          </DialogTitle>
          <DialogDescription>
            The new owner gains full control of this team. You stay in the team
            as an admin and cannot undo this yourself.
          </DialogDescription>
        </DialogHeader>

        {candidates.length === 0 ? (
          <Alert>
            <AlertDescription>
              There is no one to transfer to yet. Invite someone and wait for
              them to accept first.
            </AlertDescription>
          </Alert>
        ) : (
          <div
            className="flex flex-col gap-2"
            role="radiogroup"
            aria-label="New owner"
          >
            {candidates.map(member => {
              const isSelected = member.user_id === selectedUserId
              return (
                <button
                  key={member.user_id}
                  type="button"
                  role="radio"
                  aria-checked={isSelected}
                  disabled={isTransferring}
                  onClick={() => {
                    setSelectedUserId(member.user_id)
                  }}
                  className={cn(
                    'flex flex-col items-start rounded-md border px-3 py-2 text-left text-sm transition-colors',
                    isSelected
                      ? 'border-primary bg-primary/5'
                      : 'hover:bg-muted/50'
                  )}
                >
                  <span className="font-medium">{member.name}</span>
                  <span className="text-muted-foreground text-xs">
                    {member.email}
                  </span>
                </button>
              )
            })}
          </div>
        )}

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="size-4" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <DialogFooter className="gap-2 sm:gap-2">
          <Button
            variant="outline"
            onClick={handleClose}
            disabled={isTransferring}
          >
            Cancel
          </Button>
          <Button
            data-testid="confirm-transfer-ownership-button"
            onClick={() => {
              void handleTransfer()
            }}
            disabled={!selected || isTransferring}
          >
            {transferButtonLabel(isTransferring, selected)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
