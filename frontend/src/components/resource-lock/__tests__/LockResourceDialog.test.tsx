import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import {
  LOCK_COMMENT_MAX_LENGTH,
  LockResourceDialog,
} from '../LockResourceDialog'

function renderDialog(
  props: Partial<Parameters<typeof LockResourceDialog>[0]> = {}
) {
  const onSubmit = vi.fn()
  const onOpenChange = vi.fn()
  const view = render(
    <LockResourceDialog
      open
      kind="prompt"
      submitting={false}
      onOpenChange={onOpenChange}
      onSubmit={onSubmit}
      {...props}
    />
  )
  return { onSubmit, onOpenChange, ...view }
}

describe('LockResourceDialog', () => {
  it('keeps submit disabled until the comment has non-whitespace text', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderDialog()
    const submit = screen.getByTestId('confirm-lock-button')
    const input = screen.getByLabelText(/reason/i)

    expect(submit).toBeDisabled()

    await user.type(input, '   ')
    expect(submit).toBeDisabled()
    // Enter in a textarea is a newline, and the form itself refuses an empty
    // comment: nothing is submitted either way.
    await user.click(submit)
    expect(onSubmit).not.toHaveBeenCalled()

    await user.type(input, 'in use')
    expect(submit).toBeEnabled()
  })

  it('submits the trimmed comment', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderDialog()

    await user.type(
      screen.getByLabelText(/reason/i),
      '  Used by the nightly release agent  '
    )
    await user.click(screen.getByTestId('confirm-lock-button'))

    expect(onSubmit).toHaveBeenCalledTimes(1)
    expect(onSubmit).toHaveBeenCalledWith('Used by the nightly release agent')
  })

  it('caps the comment at the server limit of 500 characters', async () => {
    const user = userEvent.setup()
    renderDialog()
    const input = screen.getByLabelText<HTMLTextAreaElement>(/reason/i)

    expect(LOCK_COMMENT_MAX_LENGTH).toBe(500)
    expect(input).toHaveAttribute('maxlength', '500')

    await user.click(input)
    await user.paste('x'.repeat(520))
    expect(input.value).toHaveLength(500)
    expect(screen.getByText('500/500 characters')).toBeInTheDocument()
  })

  it('names the kind being locked', () => {
    renderDialog({ kind: 'artifact' })
    expect(
      screen.getByRole('heading', { name: 'Lock artifact?' })
    ).toBeInTheDocument()
  })

  it('freezes the form and refuses to close while a lock request is in flight', async () => {
    const user = userEvent.setup()
    const { onOpenChange, onSubmit } = renderDialog({ submitting: true })

    expect(screen.getByLabelText(/reason/i)).toBeDisabled()
    expect(screen.getByTestId('confirm-lock-button')).toBeDisabled()
    expect(screen.getByTestId('confirm-lock-button')).toHaveTextContent(
      'Locking…'
    )
    await user.keyboard('{Escape}')
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('starts empty again after being cancelled', async () => {
    const user = userEvent.setup()
    const { onOpenChange, rerender } = renderDialog()

    await user.type(screen.getByLabelText(/reason/i), 'draft')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)

    rerender(
      <LockResourceDialog
        open
        kind="prompt"
        submitting={false}
        onOpenChange={onOpenChange}
        onSubmit={vi.fn()}
      />
    )
    expect(screen.getByLabelText(/reason/i)).toHaveValue('')
  })
})
