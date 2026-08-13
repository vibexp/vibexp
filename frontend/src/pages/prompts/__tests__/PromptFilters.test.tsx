import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { PromptFilters } from '../PromptFilters'

// Radix Select relies on layout APIs jsdom doesn't implement.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

function renderFilters(
  overrides: Partial<Parameters<typeof PromptFilters>[0]> = {}
) {
  const props = {
    searchInput: '',
    onSearchInputChange: vi.fn(),
    statusFilter: 'all' as const,
    onStatusChange: vi.fn(),
    sharedFilter: 'all' as const,
    onSharedChange: vi.fn(),
    freshness: undefined,
    onFreshnessChange: vi.fn(),
    ...overrides,
  }
  render(<PromptFilters {...props} />)
  return props
}

describe('PromptFilters', () => {
  it('reports search input changes', async () => {
    const props = renderFilters()

    const user = userEvent.setup()
    await user.type(screen.getByPlaceholderText('Search prompts…'), 'r')

    expect(props.onSearchInputChange).toHaveBeenCalledWith('r')
  })

  it('maps the status select to onStatusChange', async () => {
    const props = renderFilters()

    const user = userEvent.setup()
    const [statusTrigger] = screen.getAllByRole('combobox')
    await user.click(statusTrigger)
    await user.click(screen.getByRole('option', { name: 'Published' }))

    expect(props.onStatusChange).toHaveBeenCalledWith('published')
  })

  it('maps the shared select to onSharedChange', async () => {
    const props = renderFilters()

    const user = userEvent.setup()
    const [, sharedTrigger] = screen.getAllByRole('combobox')
    await user.click(sharedTrigger)
    await user.click(screen.getByRole('option', { name: 'Not shared' }))

    expect(props.onSharedChange).toHaveBeenCalledWith('not_shared')
  })
})
