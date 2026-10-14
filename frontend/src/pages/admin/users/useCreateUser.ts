import { useState } from 'react'

import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

import { teamRowErrors } from './setupOutcome'
import type { CreatedUser, UserFormValues } from './UserFormDialog'

/**
 * The "New user" dialog's state on the users list (#1443): the request, its
 * inline errors (a duplicate email, a refused team row) and, once the user
 * exists, how they get in. The setup link lives in this state only and is
 * dropped when the dialog closes.
 */
export function useCreateUser(onCreated: () => void) {
  const [open, setOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [teamErrors, setTeamErrors] = useState<Record<number, string>>({})
  const [created, setCreated] = useState<(CreatedUser & { id: string }) | null>(
    null
  )

  const start = () => {
    setError(null)
    setTeamErrors({})
    setCreated(null)
    setOpen(true)
  }

  const onOpenChange = (next: boolean) => {
    setOpen(next)
    if (!next) setCreated(null)
  }

  const submit = (values: UserFormValues) => {
    setSubmitting(true)
    setError(null)
    setTeamErrors({})
    adminService
      .createUser({
        email: values.email,
        ...(values.name ? { name: values.name } : {}),
        ...(values.idp_provider ? { idp_provider: values.idp_provider } : {}),
        ...(values.teams.length > 0
          ? {
              teams: values.teams.map(({ team_id, role }) => ({
                team_id,
                role,
              })),
            }
          : {}),
      })
      .then(user => {
        setCreated({ id: user.id, email: user.email, setup: user.setup })
        onCreated()
      })
      .catch((err: unknown) => {
        // Inline rather than a toast: a duplicate email or a bad team is a
        // correction to make in the form, which stays open.
        const rows = teamRowErrors(err)
        setTeamErrors(rows)
        setError(
          Object.keys(rows).length > 0
            ? 'Some teams cannot be used. Nothing was created.'
            : getErrorMessage(err, 'Failed to create the user')
        )
      })
      .finally(() => {
        setSubmitting(false)
      })
  }

  return {
    open,
    start,
    onOpenChange,
    submitting,
    error,
    teamErrors,
    created,
    submit,
  }
}
