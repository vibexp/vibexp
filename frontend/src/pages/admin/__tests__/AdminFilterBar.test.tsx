/**
 * AdminFilterBar's "Advanced filters" toggle and badge (#1132).
 */
import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import type { ComponentProps } from 'react'

import { AdminFilterBar } from '../AdminFilterBar'

function renderBar(
  overrides: Partial<ComponentProps<typeof AdminFilterBar>> = {}
) {
  return render(
    <AdminFilterBar
      searchInput=""
      onSearchInputChange={vi.fn()}
      searchPlaceholder="Search"
      searchLabel="Search users"
      created={{}}
      onCreatedChange={vi.fn()}
      hasActiveFilters={false}
      {...overrides}
    />
  )
}

const toggle = () => screen.getByRole('button', { name: /Advanced filters/ })

describe('AdminFilterBar — advanced filters', () => {
  it('renders no toggle without advanced content', () => {
    renderBar()
    expect(
      screen.queryByRole('button', { name: /Advanced filters/ })
    ).not.toBeInTheDocument()
  })

  it('starts collapsed and toggles the panel', async () => {
    renderBar({ advanced: <p>panel body</p> })
    expect(toggle()).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('panel body')).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('advanced-filters-count')
    ).not.toBeInTheDocument()

    await userEvent.click(toggle())
    expect(toggle()).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('panel body')).toBeVisible()

    await userEvent.click(toggle())
    expect(toggle()).toHaveAttribute('aria-expanded', 'false')
  })

  it('opens on mount when an advanced filter is active, with a count badge', () => {
    renderBar({ advanced: <p>panel body</p>, advancedActiveCount: 2 })
    expect(toggle()).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('panel body')).toBeVisible()
    expect(screen.getByTestId('advanced-filters-count')).toHaveTextContent(
      '2 active'
    )
    expect(toggle()).toHaveAccessibleName(/^Advanced filters\s*2\s*active$/)
  })

  it('stays open when the active filters are cleared', () => {
    const { rerender } = renderBar({
      advanced: <p>panel body</p>,
      advancedActiveCount: 1,
    })
    rerender(
      <AdminFilterBar
        searchInput=""
        onSearchInputChange={vi.fn()}
        searchPlaceholder="Search"
        searchLabel="Search users"
        created={{}}
        onCreatedChange={vi.fn()}
        hasActiveFilters={false}
        advanced={<p>panel body</p>}
        advancedActiveCount={0}
      />
    )
    expect(toggle()).toHaveAttribute('aria-expanded', 'true')
    expect(
      screen.queryByTestId('advanced-filters-count')
    ).not.toBeInTheDocument()
  })
})
