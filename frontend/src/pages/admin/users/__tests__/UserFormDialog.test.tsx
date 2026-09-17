/**
 * Pins the submit button's caption across the three states it has to
 * distinguish (#1031). The label used to be a nested ternary inline in the JSX;
 * it now resolves in a helper, and these cases are what keeps the two spellings
 * behaviourally identical — including that an in-flight submit outranks the
 * mode.
 */
import { render, screen } from '@testing-library/react'

import { UserFormDialog } from '../UserFormDialog'

const renderDialog = (
  props: Partial<React.ComponentProps<typeof UserFormDialog>> = {}
) =>
  render(
    <UserFormDialog
      open
      onOpenChange={vi.fn()}
      mode="create"
      submitting={false}
      onSubmit={vi.fn()}
      {...props}
    />
  )

describe('UserFormDialog', () => {
  it.each([
    { mode: 'create', submitting: false, label: 'Create user' },
    { mode: 'edit', submitting: false, label: 'Save changes' },
    { mode: 'create', submitting: true, label: 'Saving…' },
    { mode: 'edit', submitting: true, label: 'Saving…' },
  ] as const)(
    'labels the submit button "$label" in $mode mode when submitting is $submitting',
    ({ mode, submitting, label }) => {
      renderDialog({ mode, submitting })

      expect(screen.getByRole('button', { name: label })).toBeInTheDocument()
    }
  )
})
