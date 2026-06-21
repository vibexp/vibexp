import { render, screen, within } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

import { SearchModal } from '../SearchModal'

// Radix sets pointer-events:none on the body while a dialog is open; disable
// userEvent's pointer-events guard so clicks inside the portal still register.
function setup() {
  const user = userEvent.setup({ pointerEventsCheck: 0 })
  render(
    <MemoryRouter>
      <SearchModal />
    </MemoryRouter>
  )
  return user
}

async function openModal(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Search' }))
  return screen.getByRole('dialog')
}

describe('SearchModal', () => {
  beforeEach(() => {
    mockNavigate.mockClear()
  })

  it('opens the dialog with a query textarea and a submit button', async () => {
    const user = setup()
    const dialog = await openModal(user)

    expect(
      within(dialog).getByRole('textbox', { name: 'Search query' })
    ).toBeInTheDocument()
    expect(
      within(dialog).getByRole('button', { name: /search/i })
    ).toBeInTheDocument()
  })

  it('disables the Search button until a non-empty query is entered', async () => {
    const user = setup()
    const dialog = await openModal(user)

    const submit = within(dialog).getByRole('button', { name: /search/i })
    expect(submit).toBeDisabled()

    await user.type(
      within(dialog).getByRole('textbox', { name: 'Search query' }),
      '  '
    )
    expect(submit).toBeDisabled() // whitespace only

    await user.type(
      within(dialog).getByRole('textbox', { name: 'Search query' }),
      'retry config'
    )
    expect(submit).toBeEnabled()
  })

  it('navigates to the encoded results URL when the Search button is clicked', async () => {
    const user = setup()
    const dialog = await openModal(user)

    await user.type(
      within(dialog).getByRole('textbox', { name: 'Search query' }),
      'a & b'
    )
    await user.click(within(dialog).getByRole('button', { name: /search/i }))

    expect(mockNavigate).toHaveBeenCalledWith('/search?q=a%20%26%20b')
  })

  it('submits on Enter but inserts a newline on Shift+Enter', async () => {
    const user = setup()
    const dialog = await openModal(user)
    const textarea = within(dialog).getByRole('textbox', {
      name: 'Search query',
    })

    // Shift+Enter must NOT submit
    await user.type(textarea, 'line one{Shift>}{Enter}{/Shift}line two')
    expect(mockNavigate).not.toHaveBeenCalled()

    // Plain Enter submits the accumulated value
    await user.type(textarea, '{Enter}')
    expect(mockNavigate).toHaveBeenCalledTimes(1)
    expect(mockNavigate).toHaveBeenCalledWith(
      expect.stringContaining('/search?q=')
    )
  })

  it('does not navigate when the query is empty or whitespace', async () => {
    const user = setup()
    const dialog = await openModal(user)
    const textarea = within(dialog).getByRole('textbox', {
      name: 'Search query',
    })

    await user.type(textarea, '   {Enter}')
    expect(mockNavigate).not.toHaveBeenCalled()
  })
})
