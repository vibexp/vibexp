import { useEffect, useState } from 'react'

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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { AdminUserSetup } from '@/services/adminService'

import { LinkReveal } from './LinkReveal'
import { setupOutcome } from './setupOutcome'
import { type TeamAssignment, TeamAssignments } from './TeamAssignments'

/** Everything the dialog can collect; `create` uses all of it, `edit` only the name. */
export interface UserFormValues {
  email: string
  name: string
  idp_provider: string
  /** Create only: the teams to add the new user to. */
  teams: TeamAssignment[]
}

/** What creating the user produced, shown in place of the form (#1443). */
export interface CreatedUser {
  email: string
  setup: AdminUserSetup
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
  /** The server's message per team id, on a refused create. */
  teamErrors?: Readonly<Record<string, string>>
  /** Set once the user exists: the dialog shows how they get in. */
  created?: CreatedUser | null
  /** "Open user" on the outcome panel. */
  onOpenCreated?: () => void
  onSubmit: (values: UserFormValues) => void
}

/** Resolves the submit button's caption; in-flight state wins over the mode. */
function submitLabel({
  submitting,
  creating,
}: Readonly<{ submitting: boolean; creating: boolean }>): string {
  if (submitting) return 'Saving…'
  return creating ? 'Create user' : 'Save changes'
}

/**
 * One dialog for both creating and editing a user, because the API only lets an
 * admin edit the display name — a separate edit dialog would be this one with a
 * few fields hidden.
 *
 * Create posts to #462's endpoint, which publishes `user.created`, so an
 * admin-created account gets its personal workspace and default project exactly
 * as a self-signup would; it also joins the chosen teams and gets a setup link
 * for its first password (#1436). The dialog then shows how the owner gets in:
 * the link was emailed, or it is shown here ONCE to hand over, or why there is
 * none. The link lives in this dialog's props only and is gone once it closes.
 */
export function UserFormDialog({
  open,
  onOpenChange,
  mode,
  initial,
  submitting,
  error,
  teamErrors,
  created,
  onOpenCreated,
  onSubmit,
}: Readonly<UserFormDialogProps>) {
  const [email, setEmail] = useState(initial?.email ?? '')
  const [name, setName] = useState(initial?.name ?? '')
  const [provider, setProvider] = useState(initial?.idp_provider ?? '')
  const [teams, setTeams] = useState<TeamAssignment[]>([])

  // Reseed whenever the dialog opens, so reopening after a cancel does not show
  // the previous attempt's text.
  useEffect(() => {
    if (open) {
      setEmail(initial?.email ?? '')
      setName(initial?.name ?? '')
      setProvider(initial?.idp_provider ?? '')
      setTeams([])
    }
  }, [open, initial?.email, initial?.name, initial?.idp_provider])

  const creating = mode === 'create'
  // Create needs an email (the name is optional); edit needs a name.
  const canSubmit =
    (creating ? email.trim() !== '' : name.trim() !== '') && !submitting

  const handleSubmit = () => {
    if (!canSubmit) return
    onSubmit({
      email: email.trim(),
      name: name.trim(),
      idp_provider: provider,
      teams,
    })
  }

  if (creating && created) {
    const outcome = setupOutcome(created.email, created.setup)
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-lg" data-testid="user-created">
          <DialogHeader>
            <DialogTitle>User created</DialogTitle>
            <DialogDescription>
              {created.email} has an account and a personal workspace.
            </DialogDescription>
          </DialogHeader>
          {outcome.link ? (
            <div className="space-y-2">
              <p className="text-sm">{outcome.text}</p>
              <LinkReveal link={outcome.link} />
            </div>
          ) : (
            <Alert
              variant={outcome.tone === 'warning' ? 'destructive' : 'default'}
              data-testid="user-created-setup"
            >
              <AlertTitle>{outcome.title}</AlertTitle>
              <AlertDescription>{outcome.text}</AlertDescription>
            </Alert>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                onOpenChange(false)
              }}
            >
              Done
            </Button>
            <Button type="button" onClick={onOpenCreated}>
              Open user
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{creating ? 'New user' : 'Edit user'}</DialogTitle>
          <DialogDescription>
            {creating
              ? 'Creates the account and its personal workspace. They get a link to set their password (or sign in through an identity provider with this email address).'
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
            <Label htmlFor="admin-user-name">
              {creating ? 'Name (optional)' : 'Name'}
            </Label>
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

          {creating && (
            <TeamAssignments
              value={teams}
              onChange={setTeams}
              errors={teamErrors}
              disabled={submitting}
            />
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
              {submitLabel({ submitting, creating })}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
