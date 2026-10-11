import { type FormEvent, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import type { LockableKind } from '@/types/resourceLock'

/** The server's limit on a lock comment (`LockResourceRequest.comment` in the spec). */
export const LOCK_COMMENT_MAX_LENGTH = 500

interface LockResourceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  kind: LockableKind
  /** A lock request is in flight: the form is frozen and cannot be closed. */
  submitting: boolean
  /** Receives the trimmed, non-empty comment. */
  onSubmit: (comment: string) => void
}

/**
 * Asks for the reason a prompt, blueprint or artifact is being locked. The
 * comment is required: the submit button stays disabled until it holds
 * non-whitespace text, matching the server's trim-then-reject rule.
 */
export function LockResourceDialog({
  open,
  onOpenChange,
  kind,
  submitting,
  onSubmit,
}: Readonly<LockResourceDialogProps>) {
  const [comment, setComment] = useState('')
  const trimmed = comment.trim()

  // The draft is not cleared here: `useResourceLock` remounts the dialog on
  // every open, which also covers the closes this component never sees.
  const handleOpenChange = (next: boolean) => {
    if (submitting) return
    onOpenChange(next)
  }

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (trimmed === '' || submitting) return
    onSubmit(trimmed)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>Lock {kind}?</DialogTitle>
            <DialogDescription>
              A locked {kind} cannot be deleted, and its slug, status, project,
              labels and other settings cannot be changed until it is unlocked.
              Its content stays editable.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-1.5">
            <Label htmlFor="lock-comment">
              Reason <span className="text-destructive">*</span>
            </Label>
            <Textarea
              id="lock-comment"
              data-testid="lock-comment-input"
              value={comment}
              onChange={event => {
                setComment(event.target.value)
              }}
              placeholder="Why is this being locked?"
              rows={3}
              maxLength={LOCK_COMMENT_MAX_LENGTH}
              disabled={submitting}
              required
            />
            <p className="text-muted-foreground text-xs">
              {comment.length}/{LOCK_COMMENT_MAX_LENGTH} characters
            </p>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={submitting}
              onClick={() => {
                handleOpenChange(false)
              }}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              data-testid="confirm-lock-button"
              disabled={trimmed === '' || submitting}
            >
              {submitting ? 'Locking…' : 'Lock'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
