import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { ResourceFreshnessState } from '../FreshnessBadge'
import { FreshnessBadge } from '../FreshnessBadge'

const stale = (
  overrides: Partial<ResourceFreshnessState> = {}
): ResourceFreshnessState => ({
  status: 'stale',
  since: '2026-08-01T00:00:00Z',
  matched_rule_ids: ['r1'],
  reason: 'rule_run',
  ...overrides,
})

describe('FreshnessBadge', () => {
  it('renders the badge for a stale resource', () => {
    render(<FreshnessBadge freshness={stale()} />)
    expect(screen.getByTestId('freshness-badge')).toHaveTextContent('Stale')
  })

  it('renders nothing when freshness is absent', () => {
    // A fresh resource omits the field entirely (#735), so the badge must cost
    // no markup at all — that is what keeps fresh rows free of layout shift.
    const { container } = render(<FreshnessBadge freshness={undefined} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the since-date in the tooltip', async () => {
    const user = userEvent.setup()
    render(<FreshnessBadge freshness={stale()} />)

    await user.hover(screen.getByTestId('freshness-badge'))

    await waitFor(() => {
      expect(screen.getAllByText(/Not used since/).length).toBeGreaterThan(0)
    })
  })

  it('reports how many rules flagged it, singular', async () => {
    const user = userEvent.setup()
    render(<FreshnessBadge freshness={stale({ matched_rule_ids: ['r1'] })} />)

    await user.hover(screen.getByTestId('freshness-badge'))

    await waitFor(() => {
      expect(
        screen.getAllByText(/Flagged by 1 freshness rule\./).length
      ).toBeGreaterThan(0)
    })
  })

  it('pluralises the rule count', async () => {
    const user = userEvent.setup()
    render(
      <FreshnessBadge freshness={stale({ matched_rule_ids: ['r1', 'r2'] })} />
    )

    await user.hover(screen.getByTestId('freshness-badge'))

    await waitFor(() => {
      expect(
        screen.getAllByText(/Flagged by 2 freshness rules\./).length
      ).toBeGreaterThan(0)
    })
  })

  it('explains an empty rule list rather than saying "0 rules"', async () => {
    // A rule deleted after the mark leaves the state behind until the next
    // evaluation run, so this is reachable.
    const user = userEvent.setup()
    render(<FreshnessBadge freshness={stale({ matched_rule_ids: [] })} />)

    await user.hover(screen.getByTestId('freshness-badge'))

    await waitFor(() => {
      expect(screen.getAllByText(/no longer exists/).length).toBeGreaterThan(0)
    })
  })
})
