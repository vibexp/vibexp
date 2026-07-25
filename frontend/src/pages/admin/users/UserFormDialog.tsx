import { useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/** Everything the dialog can collect; `create` uses all of it, `edit` only the name. */
export interface UserFormValues {
  email: string
  name: string
  idp_provider: string
}

export interface UserFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  /** Seeds the fields in edit mode. */
  initial?: Partial<UserFormValues>
  submitting: boolean
  /** Server-side failure to show inline, e.g. a 409 for a duplicate email. */
  error?: string | null
  onSubmit: (values: UserFormValues) => void
}

/**
 * One dialog for both creating and editing a user, because the API only lets an
 * admin edit the display name — a separate edit dialog would be this one with two
 * fields hidden.
 *
 * Create posts to #462's endpoint, which publishes `user.created`, so an
 * admin-created account gets its personal workspace and default project exactly
 * as a self-signup would. No password is involved: VibeXP has no password
 * provider, and the account's owner still signs in through an identity provider.
 */
export function UserFormDialog({
  open,
  onOpenChange,
  mode,
  initial,
  submitting,
  error,
  onSubmit,
}: Readonly<UserFormDialogProps>) {
  const [email, setEmail] = useState(initial?.email ?? '')
  const [name, setName] = useState(initial?.name ?? '')
  const [provider, setProvider] = useState(initial?.idp_provider ?? '')

  // Reseed whenever the dialog opens, so reopening after a cancel does not show
  // the previous attempt's text.
  useEffect(() => {
    if (open) {
      setEmail(initial?.email ?? '')
      setName(initial?.name ?? '')
      setProvider(initial?.idp_provider ?? '')
    }
  }, [open, initial?.email, initial?.name, initial?.idp_provider])

  const creating = mode === 'create'
  // The API requires a name on both operations, and an email on create.
  const canSubmit =
    name.trim() !== '' && (!creating || email.trim() !== '') && !submitting

  const handleSubmit = () => {
    if (!canSubmit) return
    onSubmit({
      email: email.trim(),
      name: name.trim(),
      idp_provider: provider,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{creating ? 'New user' : 'Edit user'}</DialogTitle>
          <DialogDescription>
            {creating
              ? 'Creates the account and its personal workspace. They sign in through your identity provider — no password is set.'
              : 'Only the display name can be changed here.'}
          </DialogDescription>
        </DialogHeader>

        <form
          className="space-y-4"
          onSubmit={event => {
            event.preventDefault()
            handleSubmit()
          }}
        >
          {creating && (
            <div className="space-y-1.5">
              <Label htmlFor="admin-user-email">Email</Label>
              <Input
                id="admin-user-email"
                type="email"
                value={email}
                onChange={event => {
                  setEmail(event.target.value)
                }}
                placeholder="new.user@example.com"
                autoComplete="off"
              />
            </div>
          )}

          <div className="space-y-1.5">
            <Label htmlFor="admin-user-name">Name</Label>
            <Input
              id="admin-user-name"
              value={name}
              onChange={event => {
                setName(event.target.value)
              }}
              placeholder="New User"
              autoComplete="off"
            />
          </div>

          {creating && (
            <div className="space-y-1.5">
              <Label htmlFor="admin-user-provider">
                Expected identity provider (optional)
              </Label>
              {/* Free text, not a select: the API documents this as an
                  informational label an operator may set to anything, and a fixed
                  list could not express a provider this instance actually uses.
                  The filter bar's provider control is a select because it narrows
                  to known values; this one records one. */}
              <Input
                id="admin-user-provider"
                value={provider}
                onChange={event => {
                  setProvider(event.target.value)
                }}
                placeholder="google, oidc, …"
                autoComplete="off"
              />
              <p className="text-muted-foreground text-xs">
                A label for your records. It does not pre-link an account — the
                identity is established on first sign-in.
              </p>
            </div>
          )}

          {error && <p className="text-destructive text-sm">{error}</p>}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                onOpenChange(false)
              }}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={!canSubmit}>
              {submitting
                ? 'Saving…'
                : creating
                  ? 'Create user'
                  : 'Save changes'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
