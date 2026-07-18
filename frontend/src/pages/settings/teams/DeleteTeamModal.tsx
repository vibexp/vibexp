import { AlertCircle, AlertTriangle } from 'lucide-react'
import { useState } from 'react'

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
import { toast } from '@/lib/toast'
import type { Team } from '@/services/teamService'
import { teamService } from '@/services/teamService'
import { ApiError } from '@/types/errors'

interface DeleteTeamModalProps {
  isOpen: boolean
  team: Team
  onClose: () => void
  onSuccess: () => void
}

const KNOWN_ERROR_CODES = [
  'TEAM_HAS_MEMBERS',
  'CANNOT_DELETE_PERSONAL_WORKSPACE',
]

export function DeleteTeamModal({
  isOpen,
  team,
  onClose,
  onSuccess,
}: Readonly<DeleteTeamModalProps>) {
  const [isDeleting, setIsDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [errorCode, setErrorCode] = useState<string | null>(null)

  const resetErrorState = () => {
    setError(null)
    setErrorCode(null)
  }

  const handleDelete = async () => {
    resetErrorState()

    try {
      setIsDeleting(true)
      await teamService.deleteTeam(team.id)
      toast.success('Team deleted successfully')
      onSuccess()
      onClose()
    } catch (err) {
      if (err instanceof ApiError) {
        setErrorCode(err.code)
        setError(err.getMessage())

        if (!KNOWN_ERROR_CODES.includes(err.code)) {
          toast.error(err.getMessage())
        }
      } else {
        const errorMessage =
          err instanceof Error ? err.message : 'Failed to delete team'
        setError(errorMessage)
        toast.error(errorMessage)
      }
    } finally {
      setIsDeleting(false)
    }
  }

  const handleClose = () => {
    resetErrorState()
    onClose()
  }

  const handleOpenChange = (open: boolean) => {
    if (!open && !isDeleting) handleClose()
  }

  if (errorCode === 'TEAM_HAS_MEMBERS') {
    return (
      <Dialog open={isOpen} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertCircle className="text-destructive size-5" />
              Cannot Delete Team
            </DialogTitle>
            <DialogDescription>{error}</DialogDescription>
          </DialogHeader>

          <Alert variant="destructive">
            <AlertDescription>
              Please remove all team members before deleting the team.
            </AlertDescription>
          </Alert>

          <DialogFooter>
            <Button variant="outline" onClick={handleClose}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="text-destructive size-5" />
            Delete Team &quot;{team.name}&quot;?
          </DialogTitle>
          <DialogDescription>
            This action cannot be undone. All team data will be permanently
            deleted.
          </DialogDescription>
        </DialogHeader>

        {(team.member_count ?? 0) > 1 && (
          <Alert variant="destructive">
            <AlertTitle>
              This team has {team.member_count ?? 0} members
            </AlertTitle>
            <AlertDescription>
              Remove all members before deleting.
            </AlertDescription>
          </Alert>
        )}

        {error && errorCode !== 'TEAM_HAS_MEMBERS' && (
          <Alert variant="destructive">
            <AlertCircle className="size-4" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={handleClose} disabled={isDeleting}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            data-testid="confirm-delete-team-button"
            onClick={() => {
              void handleDelete()
            }}
            disabled={isDeleting}
          >
            {isDeleting ? 'Deleting…' : 'Delete Team'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
