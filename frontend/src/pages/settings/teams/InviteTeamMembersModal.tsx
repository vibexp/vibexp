import { AlertCircle, Mail, Send } from 'lucide-react'
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
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

interface InviteTeamMembersModalProps {
  isOpen: boolean
  teamName: string
  onClose: () => void
  onSubmit: (emails: string[]) => Promise<void>
}

function parseEmails(text: string): string[] {
  return text
    .split(/[,;\n]/)
    .map(email => email.trim())
    .filter(email => email.length > 0)
}

function validateEmails(emails: string[]): {
  valid: boolean
  errors: string[]
} {
  const errors: string[] = []
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

  emails.forEach((email, index) => {
    if (!emailRegex.test(email)) {
      errors.push(`Invalid email at position ${String(index + 1)}: ${email}`)
    }
  })

  return { valid: errors.length === 0, errors }
}

export function InviteTeamMembersModal({
  isOpen,
  teamName,
  onClose,
  onSubmit,
}: Readonly<InviteTeamMembersModalProps>) {
  const [emailsText, setEmailsText] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.SubmitEvent) => {
    e.preventDefault()
    setError(null)

    if (!emailsText.trim()) {
      setError('Please enter at least one email address')
      return
    }

    const emails = parseEmails(emailsText)
    if (emails.length === 0) {
      setError('Please enter at least one valid email address')
      return
    }

    const validation = validateEmails(emails)
    if (!validation.valid) {
      setError(validation.errors.join('\n'))
      return
    }

    setIsSubmitting(true)
    try {
      await onSubmit(emails)
      setEmailsText('')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to send invitations'
      )
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleClose = () => {
    setEmailsText('')
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
          <DialogTitle>Invite Team Members</DialogTitle>
          <DialogDescription>
            Invite new members to join{' '}
            <span className="text-foreground font-semibold">{teamName}</span>.
          </DialogDescription>
        </DialogHeader>

        <form
          onSubmit={e => {
            void handleSubmit(e)
          }}
          className="space-y-4"
        >
          <div className="space-y-1.5">
            <Label htmlFor="emails">Email Addresses</Label>
            <Textarea
              id="emails"
              value={emailsText}
              onChange={e => {
                setEmailsText(e.target.value)
              }}
              placeholder={
                'Enter email addresses separated by commas, semicolons, or new lines\nExample:\nuser1@example.com\nuser2@example.com, user3@example.com'
              }
              rows={6}
              disabled={isSubmitting}
            />
            <p className="text-muted-foreground flex items-center gap-1.5 text-xs">
              <Mail className="size-3" />
              You can enter multiple email addresses separated by commas,
              semicolons, or new lines.
            </p>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription className="whitespace-pre-line">
                {error}
              </AlertDescription>
            </Alert>
          )}

          <DialogFooter className="gap-2 sm:gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={handleClose}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              <Send className="mr-2 size-4" />
              {isSubmitting ? 'Sending…' : 'Send Invitations'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
