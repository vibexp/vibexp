import { useState } from 'react'

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
import { useTeam } from '@/contexts/TeamContext'
import { toast } from '@/lib/toast'
import { teamService } from '@/services/teamService'
import type { CreateTeamRequest } from '@/types/team'
import { getErrorMessage } from '@/utils/errorHandling'

interface CreateTeamModalProps {
  isOpen: boolean
  onClose: () => void
  onSuccess: () => void
}

export function CreateTeamModal({
  isOpen,
  onClose,
  onSuccess,
}: CreateTeamModalProps) {
  const { refreshTeams } = useTeam()
  const [formData, setFormData] = useState<CreateTeamRequest>({
    name: '',
    description: '',
  })
  const [errors, setErrors] = useState<Partial<CreateTeamRequest>>({})
  const [isSubmitting, setIsSubmitting] = useState(false)

  const validateForm = (): boolean => {
    const newErrors: Partial<CreateTeamRequest> = {}

    if (!formData.name.trim()) {
      newErrors.name = 'Team name is required'
    } else if (formData.name.length > 100) {
      newErrors.name = 'Team name must not exceed 100 characters'
    }

    if (formData.description && formData.description.length > 500) {
      newErrors.description = 'Description must not exceed 500 characters'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async () => {
    if (!validateForm()) {
      return
    }

    try {
      setIsSubmitting(true)
      await teamService.createTeam(formData)
      toast.success('Team created successfully')
      setFormData({ name: '', description: '' })
      setErrors({})
      await refreshTeams()
      onSuccess()
      onClose()
    } catch (error) {
      const errorMessage = getErrorMessage(error, 'Failed to create team')
      toast.error(errorMessage)
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleClose = () => {
    setFormData({ name: '', description: '' })
    setErrors({})
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
          <DialogTitle>Create New Team</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="team-name">
              Team Name <span className="text-destructive">*</span>
            </Label>
            <Input
              id="team-name"
              data-testid="team-name-input"
              type="text"
              value={formData.name}
              onChange={e => {
                setFormData({ ...formData, name: e.target.value })
              }}
              placeholder="Enter team name"
              maxLength={100}
              disabled={isSubmitting}
              aria-invalid={!!errors.name}
            />
            {errors.name && (
              <p className="text-destructive text-sm">{errors.name}</p>
            )}
            <p className="text-muted-foreground text-xs">
              {formData.name.length}/100 characters
            </p>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="team-description">Description (Optional)</Label>
            <Textarea
              id="team-description"
              value={formData.description}
              onChange={e => {
                setFormData({ ...formData, description: e.target.value })
              }}
              placeholder="Enter team description"
              rows={4}
              maxLength={500}
              disabled={isSubmitting}
              aria-invalid={!!errors.description}
            />
            {errors.description && (
              <p className="text-destructive text-sm">{errors.description}</p>
            )}
            <p className="text-muted-foreground text-xs">
              {formData.description?.length ?? 0}/500 characters
            </p>
          </div>
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
            data-testid="submit-create-team-button"
            onClick={() => {
              void handleSubmit()
            }}
            disabled={isSubmitting}
          >
            {isSubmitting ? 'Creating…' : 'Create Team'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
