import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { FreshnessFilterSelect } from '../FreshnessFilterSelect'

// Radix Select relies on layout APIs jsdom doesn't implement.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

describe('FreshnessFilterSelect', () => {
  it('shows "All freshness" when nothing is filtered', () => {
    render(<FreshnessFilterSelect value={undefined} onChange={vi.fn()} />)
    expect(screen.getByTestId('freshness-filter')).toHaveTextContent(
      'All freshness'
    )
  })

  it('shows "Stale only" when the filter is applied', () => {
    render(<FreshnessFilterSelect value="stale" onChange={vi.fn()} />)
    expect(screen.getByTestId('freshness-filter')).toHaveTextContent(
      'Stale only'
    )
  })

  it('emits "stale" when the stale option is chosen', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<FreshnessFilterSelect value={undefined} onChange={onChange} />)

    await user.click(screen.getByTestId('freshness-filter'))
    await user.click(screen.getByRole('option', { name: 'Stale only' }))

    expect(onChange).toHaveBeenCalledWith('stale')
  })

  it('emits undefined for "all", never a second filter value', async () => {
    // The API 400s on any value other than `stale`, so clearing must drop the
    // param rather than send something like "all" or "fresh".
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<FreshnessFilterSelect value="stale" onChange={onChange} />)

    await user.click(screen.getByTestId('freshness-filter'))
    await user.click(screen.getByRole('option', { name: 'All freshness' }))

    expect(onChange).toHaveBeenCalledWith(undefined)
  })

  it('honours a custom aria-label and test id', () => {
    render(
      <FreshnessFilterSelect
        value={undefined}
        onChange={vi.fn()}
        ariaLabel="Filter artifacts by freshness"
        testId="artifact-freshness-filter"
      />
    )
    expect(
      screen.getByLabelText('Filter artifacts by freshness')
    ).toBeInTheDocument()
    expect(screen.getByTestId('artifact-freshness-filter')).toBeInTheDocument()
  })
})
