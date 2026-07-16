import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CommentComposer } from '../CommentComposer'

describe('CommentComposer', () => {
  it('disables submit until there is non-whitespace content', async () => {
    const user = userEvent.setup()
    render(<CommentComposer onSubmit={jest.fn()} submitLabel="Comment" />)

    const submit = screen.getByRole('button', { name: 'Comment' })
    expect(submit).toBeDisabled()

    await user.type(screen.getByRole('textbox'), '   ')
    expect(submit).toBeDisabled()

    await user.type(screen.getByRole('textbox'), 'hello')
    expect(submit).toBeEnabled()
  })

  it('submits the trimmed content and calls onSuccess', async () => {
    const user = userEvent.setup()
    const onSubmit = jest.fn().mockResolvedValue(undefined)
    const onSuccess = jest.fn()
    render(
      <CommentComposer
        onSubmit={onSubmit}
        onSuccess={onSuccess}
        submitLabel="Comment"
      />
    )

    await user.type(screen.getByRole('textbox'), '  hi there  ')
    await user.click(screen.getByRole('button', { name: 'Comment' }))

    expect(onSubmit).toHaveBeenCalledWith('hi there')
    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled()
    })
    // Draft is cleared so a still-mounted composer (popup add box) doesn't keep it.
    expect(screen.getByRole('textbox')).toHaveValue('')
  })

  it('keeps the draft and shows an inline error when submit fails', async () => {
    const user = userEvent.setup()
    const onSubmit = jest.fn().mockRejectedValue(new Error('Server said no'))
    const onSuccess = jest.fn()
    render(
      <CommentComposer
        onSubmit={onSubmit}
        onSuccess={onSuccess}
        submitLabel="Save"
        initialValue="my draft"
      />
    )

    await user.click(screen.getByRole('button', { name: 'Save' }))

    // Editor stays open with the draft intact and an inline error.
    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('Server said no')
    })
    expect(screen.getByRole('textbox')).toHaveValue('my draft')
    expect(onSuccess).not.toHaveBeenCalled()
  })
})
