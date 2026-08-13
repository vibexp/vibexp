import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

// Radix Select drives pointer-capture and scroll APIs jsdom does not implement;
// opening the range selector throws without these. Safe here because the Select
// is NOT inside a Dialog — that combination stack-overflows jsdom regardless.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

vi.mock('@/services/freshnessService', () => ({
  freshnessService: {
    getOverTimeMetrics: vi.fn(),
    getByTypeMetrics: vi.fn(),
    getByProjectMetrics: vi.fn(),
    getByRuleMetrics: vi.fn(),
  },
}))

import { freshnessService } from '@/services/freshnessService'

import { FreshnessAnalytics } from '../FreshnessAnalytics'

const mocked = vi.mocked(freshnessService)

const overTime = {
  range: '30d' as const,
  total_marked: 7,
  total_cleared: 3,
  counts: [
    { date: '2026-08-01', marked: 4, cleared: 1, stale_total: 10, total: 5 },
    { date: '2026-08-02', marked: 3, cleared: 2, stale_total: 11, total: 5 },
  ],
}

const byType = {
  total_stale: 11,
  counts: [
    { resource_type: 'artifact' as const, count: 6 },
    { resource_type: 'prompt' as const, count: 5 },
    { resource_type: 'blueprint' as const, count: 0 },
    { resource_type: 'memory' as const, count: 0 },
  ],
}

const byProject = {
  total_stale: 11,
  counts: [
    { project_id: 'p1', name: 'Marketing', slug: 'marketing', count: 8 },
    { project_id: 'p2', name: 'Platform', slug: 'platform', count: 3 },
  ],
}

const byRule = {
  total_stale: 11,
  counts: [
    {
      rule_id: 'r1',
      project_id: null,
      resource_types: ['artifact' as const],
      threshold_days: 90,
      enabled: true,
      count: 8,
    },
    {
      rule_id: 'r2',
      project_id: 'p1',
      resource_types: ['prompt' as const],
      threshold_days: 30,
      enabled: false,
      count: 0,
    },
  ],
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.getOverTimeMetrics.mockResolvedValue(overTime)
  mocked.getByTypeMetrics.mockResolvedValue(byType)
  mocked.getByProjectMetrics.mockResolvedValue(byProject)
  mocked.getByRuleMetrics.mockResolvedValue(byRule)
})

const renderTab = () => render(<FreshnessAnalytics teamId="team-1" />)

const waitForCharts = async () => {
  await waitFor(() => {
    expect(screen.getByText('Stale by resource type')).toBeInTheDocument()
  })
}

describe('FreshnessAnalytics', () => {
  it('renders all four charts for the given team', async () => {
    renderTab()
    await waitForCharts()

    expect(screen.getByText('Staleness over time')).toBeInTheDocument()
    expect(screen.getByText('Stale by resource type')).toBeInTheDocument()
    expect(screen.getByText('Stale by project')).toBeInTheDocument()
    expect(screen.getByText('Impact per rule')).toBeInTheDocument()

    expect(mocked.getByTypeMetrics).toHaveBeenCalledWith(
      'team-1',
      expect.anything()
    )
  })

  it('requests the default 30d window for the time series', async () => {
    renderTab()
    await waitForCharts()

    expect(mocked.getOverTimeMetrics).toHaveBeenCalledWith(
      'team-1',
      '30d',
      expect.anything()
    )
  })

  it('refetches only the time series when the range changes', async () => {
    const user = userEvent.setup()
    renderTab()
    await waitForCharts()

    await user.click(
      screen.getByRole('combobox', { name: /select time range/i })
    )
    await user.click(screen.getByRole('option', { name: 'Last 7 days' }))

    await waitFor(() => {
      expect(mocked.getOverTimeMetrics).toHaveBeenCalledWith(
        'team-1',
        '7d',
        expect.anything()
      )
    })
    // The breakdowns are point-in-time counts with no window — a range change
    // must not refetch them.
    expect(mocked.getByTypeMetrics).toHaveBeenCalledTimes(1)
  })

  it('labels resource types rather than showing raw wire values', async () => {
    renderTab()
    await waitForCharts()

    const typeCard = screen
      .getByText('Stale by resource type')
      .closest('[data-testid="category-breakdown-chart"]')
    expect(
      within(typeCard as HTMLElement).getByText('Artifacts')
    ).toBeInTheDocument()
  })

  it('marks a disabled rule and shows its threshold', async () => {
    renderTab()
    await waitForCharts()

    expect(screen.getByText('30d · disabled')).toBeInTheDocument()
  })

  it('explains that per-rule counts can exceed the total', async () => {
    // Staleness is a union across rules, so the figures legitimately over-sum.
    renderTab()
    await waitForCharts()

    expect(
      screen.getByText(/can sum to more than the total/i)
    ).toBeInTheDocument()
  })

  it('explains an empty time series as a new workspace, not a failure', async () => {
    mocked.getOverTimeMetrics.mockResolvedValue({
      range: '30d',
      total_marked: 0,
      total_cleared: 0,
      counts: [],
    })
    renderTab()
    await waitForCharts()

    expect(
      screen.getByText(/Freshness starts clean and builds up as rules run/i)
    ).toBeInTheDocument()
  })

  it('keeps the other panels alive when one endpoint fails', async () => {
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)
    mocked.getByProjectMetrics.mockRejectedValue(new Error('boom'))
    renderTab()
    await waitForCharts()

    await waitFor(() => {
      expect(
        screen.getByText("Couldn't load the project breakdown")
      ).toBeInTheDocument()
    })
    // The page is still usable.
    expect(screen.getByText('Staleness over time')).toBeInTheDocument()
    expect(screen.getByText('Stale by resource type')).toBeInTheDocument()
    consoleError.mockRestore()
  })
})
