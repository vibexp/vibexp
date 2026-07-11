import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { SearchFilters } from '../SearchFilters'

/**
 * SearchFilters renders the platform-wide search query box plus its type/project
 * filters. These tests focus on the "search tips" affordance added for issue #183,
 * which surfaces the advanced keyword-search syntax (quoted phrases, OR,
 * -exclusion) that websearch_to_tsquery already supports.
 */
describe('SearchFilters search tips', () => {
  const baseProps = {
    queryInput: '',
    onQueryInputChange: jest.fn(),
    onSubmit: jest.fn(),
    onTypeChange: jest.fn(),
    onProjectChange: jest.fn(),
  }

  it('renders a search-tips trigger next to the query box', () => {
    render(<SearchFilters {...baseProps} />)
    expect(
      screen.getByRole('button', { name: 'Search tips' })
    ).toBeInTheDocument()
  })

  it('reveals the advanced-syntax operators when opened', async () => {
    const user = userEvent.setup()
    render(<SearchFilters {...baseProps} />)

    await user.click(screen.getByRole('button', { name: 'Search tips' }))

    // The three websearch_to_tsquery operators the strict keyword pass supports.
    expect(await screen.findByText('Search tips')).toBeInTheDocument()
    expect(screen.getByText(/match an exact phrase/)).toBeInTheDocument()
    expect(screen.getByText(/match\s+either\s+term/)).toBeInTheDocument()
    expect(screen.getByText(/exclude a\s+term/)).toBeInTheDocument()
  })
})
