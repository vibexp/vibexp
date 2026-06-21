import { AlertCircle } from 'lucide-react'
import { useState } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/lib/toast'
import { teamService } from '@/services/teamService'
import type { Team } from '@/types'

interface EditTeamModalProps {
  isOpen: boolean
  team: Team
  onClose: () => void
  onSuccess: () => void
}

export function EditTeamModal({
  isOpen,
  team,
  onClose,
  onSuccess,
}: EditTeamModalProps) {
  const [name, setName] = useState(team.name)
  const [description, setDescription] = useState(team.description)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async () => {
    setError(null)

    if (!name.trim()) {
      setError('Team name is required')
      return
    }

    if (name.length > 100) {
      setError('Team name cannot exceed 100 characters')
      return
    }

    if (description.length > 500) {
      setError('Description cannot exceed 500 characters')
      return
    }

    try {
      setIsSubmitting(true)
      await teamService.updateTeam(team.id, {
        name: name.trim(),
        description: description.trim(),
      })
      toast.success('Team updated successfully')
      onSuccess()
      onClose()
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : 'Failed to update team'
      toast.error(errorMessage)
      setError(errorMessage)
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleClose = () => {
    setName(team.name)
    setDescription(team.description)
    setError(null)
    onClose()
  }

  return (
    <Dialog
      open={isOpen}
      onOpenChange={open => {
        if (!open && !isSubmitting) handleClose()
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit Team</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="team-name">
              Team Name <span className="text-destructive">*</span>
            </Label>
            <Input
              id="team-name"
              type="text"
              value={name}
              onChange={e => {
                setName(e.target.value)
              }}
              placeholder="Enter team name"
              maxLength={100}
              disabled={isSubmitting}
            />
            <p className="text-muted-foreground text-xs">
              {name.length}/100 characters
            </p>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="description">Description (Optional)</Label>
            <Textarea
              id="description"
              value={description}
              onChange={e => {
                setDescription(e.target.value)
              }}
              placeholder="Enter team description"
              rows={4}
              maxLength={500}
              disabled={isSubmitting}
            />
            <p className="text-muted-foreground text-xs">
              {description.length}/500 characters
            </p>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
        </div>

        <DialogFooter className="gap-2 sm:gap-2">
          <Button
            variant="outline"
            onClick={handleClose}
            disabled={isSubmitting}
          >
            Cancel
          </Button>
          <Button
            onClick={() => {
              void handleSubmit()
            }}
            disabled={isSubmitting}
          >
            {isSubmitting ? 'Updating…' : 'Update Team'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
