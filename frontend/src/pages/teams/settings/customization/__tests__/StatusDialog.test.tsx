import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { StatusDefinition } from '@/services/statusService'

import { StatusDialog, type StatusFormErrorTarget } from '../StatusDialog'

type OnSubmit = React.ComponentProps<typeof StatusDialog>['onSubmit']

const custom: StatusDefinition = {
  id: 'status-1',
  resource_type: 'prompt',
  slug: 'in-review',
  name: 'In review',
  is_system: false,
  is_live: true,
  usage_count: 2,
}

function renderCreate(onSubmit: OnSubmit = vi.fn()) {
  render(
    <StatusDialog
      mode="create"
      open
      onOpenChange={vi.fn()}
      resourceLabel="prompt"
      submitting={false}
      onSubmit={onSubmit}
    />
  )
  return onSubmit
}

function renderRename(onSubmit: OnSubmit = vi.fn()) {
  render(
    <StatusDialog
      mode="rename"
      open
      onOpenChange={vi.fn()}
      resourceLabel="prompt"
      status={custom}
      submitting={false}
      onSubmit={onSubmit}
    />
  )
  return onSubmit
}

const nameInput = () => screen.getByTestId('status-name-input')
const slugInput = () => screen.getByTestId('status-slug-input')
const submit = () => screen.getByTestId('submit-status-button')

describe('StatusDialog — create', () => {
  it('names the resource type in the title', () => {
    renderCreate()
    expect(
      screen.getByRole('heading', { name: 'Create prompt status' })
    ).toBeInTheDocument()
  })

  it('derives the slug from the name as it is typed', async () => {
    const user = userEvent.setup()
    renderCreate()

    await user.type(nameInput(), 'In Review!')

    expect(slugInput()).toHaveValue('in-review')
  })

  it('stops deriving once the slug has been edited by hand', async () => {
    const user = userEvent.setup()
    renderCreate()

    await user.type(nameInput(), 'In review')
    await user.clear(slugInput())
    await user.type(slugInput(), 'review')
    await user.type(nameInput(), ' again')

    expect(slugInput()).toHaveValue('review')
  })

  it('submits the trimmed name and the slug', async () => {
    const user = userEvent.setup()
    const onSubmit = renderCreate()

    await user.type(nameInput(), '  In review  ')
    await user.click(submit())

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        { name: 'In review', slug: 'in-review' },
        expect.any(Function)
      )
    })
  })

  it('keeps submit disabled until both fields have a value', async () => {
    const user = userEvent.setup()
    renderCreate()

    expect(submit()).toBeDisabled()
    await user.type(nameInput(), 'In review')
    expect(submit()).toBeEnabled()
    await user.clear(slugInput())
    expect(submit()).toBeDisabled()
  })

  it.each([
    ['In_Review', 'Lowercase letters, numbers, and dashes only'],
    ['-in-review', 'Lowercase letters, numbers, and dashes only'],
    ['in--review', 'Lowercase letters, numbers, and dashes only'],
    ['a'.repeat(51), 'Slug must be at most 50 characters'],
  ])('rejects the slug %j before any request', async (slug, message) => {
    const user = userEvent.setup()
    const onSubmit = renderCreate()

    await user.type(nameInput(), 'In review')
    await user.clear(slugInput())
    await user.type(slugInput(), slug)
    await user.click(submit())

    expect(await screen.findByText(message)).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('accepts a 50-character name and rejects a 51-character one', async () => {
    const user = userEvent.setup()
    const onSubmit = renderCreate()

    await user.type(nameInput(), 'n'.repeat(51))
    await user.click(submit())
    expect(
      await screen.findByText('Name must be at most 50 characters')
    ).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()

    await user.clear(nameInput())
    await user.type(nameInput(), 'n'.repeat(50))
    await user.click(submit())
    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
  })

  // The server counts code points; String.length would count each of these
  // emoji twice and refuse a name the server accepts.
  it('counts the name length in characters, as the server does', async () => {
    const user = userEvent.setup()
    const onSubmit = renderCreate()

    await user.click(nameInput())
    await user.paste('😀'.repeat(50))
    await user.clear(slugInput())
    await user.type(slugInput(), 'smile')
    await user.click(submit())

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
  })

  it.each<[StatusFormErrorTarget, string]>([
    ['slug', 'slug is a built-in status and cannot be used'],
    ['slug', 'A status with this slug already exists for this resource type'],
  ])('shows a server %s error on the slug field', async (target, message) => {
    const user = userEvent.setup()
    renderCreate((_values, setError) => {
      setError(target, message)
    })

    await user.type(nameInput(), 'Draft two')
    await user.click(submit())

    expect(await screen.findByText(message)).toBeInTheDocument()
    expect(slugInput()).toHaveAttribute('aria-invalid', 'true')
    expect(screen.queryByTestId('status-form-error')).not.toBeInTheDocument()
  })

  it('shows a server form error above the fields', async () => {
    const user = userEvent.setup()
    const message =
      'a team can have at most 20 custom statuses per resource type'
    renderCreate((_values, setError) => {
      setError('root', message)
    })

    await user.type(nameInput(), 'One too many')
    await user.click(submit())

    expect(await screen.findByTestId('status-form-error')).toHaveTextContent(
      message
    )
  })
})

describe('StatusDialog — rename', () => {
  it('prefills the name and shows the slug read-only', () => {
    renderRename()

    expect(
      screen.getByRole('heading', { name: 'Rename status' })
    ).toBeInTheDocument()
    expect(nameInput()).toHaveValue('In review')
    expect(slugInput()).toHaveValue('in-review')
    expect(slugInput()).toHaveAttribute('readonly')
  })

  it('leaves the slug alone when the name changes', async () => {
    const user = userEvent.setup()
    const onSubmit = renderRename()

    await user.clear(nameInput())
    await user.type(nameInput(), 'Under review')
    expect(slugInput()).toHaveValue('in-review')

    await user.click(submit())
    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        { name: 'Under review', slug: 'in-review' },
        expect.any(Function)
      )
    })
  })

  it('has nothing to save until the name actually differs', async () => {
    const user = userEvent.setup()
    renderRename()

    expect(submit()).toBeDisabled()
    await user.type(nameInput(), ' ')
    expect(submit()).toBeDisabled()
    await user.type(nameInput(), 'ed')
    expect(submit()).toBeEnabled()
  })

  it('requires a name', async () => {
    const user = userEvent.setup()
    renderRename()

    await user.clear(nameInput())
    expect(submit()).toBeDisabled()
  })
})

describe('StatusDialog — reopening', () => {
  it('starts clean each time it opens', async () => {
    const user = userEvent.setup()
    const props = {
      mode: 'create' as const,
      onOpenChange: vi.fn(),
      resourceLabel: 'prompt',
      submitting: false,
      onSubmit: vi.fn(),
    }
    const { rerender } = render(<StatusDialog {...props} open />)

    await user.type(nameInput(), 'In review')
    await user.clear(slugInput())
    await user.type(slugInput(), 'custom')

    rerender(<StatusDialog {...props} open={false} />)
    rerender(<StatusDialog {...props} open />)

    expect(nameInput()).toHaveValue('')
    expect(slugInput()).toHaveValue('')
    // Derivation is back on: the manual edit belonged to the last session.
    await user.type(nameInput(), 'Blocked')
    expect(slugInput()).toHaveValue('blocked')
  })
})
