import { fireEvent, render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'

// recharts' ResponsiveContainer measures its parent, which has no layout in
// jsdom. Render children with a fixed size so the chart mounts deterministically.
jest.mock('recharts', () => {
  const actual = jest.requireActual('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: ReactNode }) => (
      <div style={{ width: 400, height: 180 }}>{children}</div>
    ),
  }
})

import {
  parseLocalDate,
  TimeSeriesBarChart,
  type TimeSeriesBarChartProps,
} from '../TimeSeriesBarChart'

const SERIES = [
  { key: 'prompts', label: 'Prompts', fill: 'var(--chart-1)' },
  { key: 'artifacts', label: 'Artifacts', fill: 'var(--chart-2)' },
]

function buildProps(
  overrides: Partial<TimeSeriesBarChartProps> = {}
): TimeSeriesBarChartProps {
  return {
    title: 'Resources created',
    totalLabel: 'Total created',
    total: 4,
    series: SERIES,
    data: [{ date: '2026-05-01', prompts: 3, artifacts: 1, total: 4 }],
    range: '30d',
    onRangeChange: jest.fn(),
    loading: false,
    error: false,
    errorMessage: "Couldn't load resource creation",
    emptyMessage: 'No resources created yet',
    ...overrides,
  }
}

describe('TimeSeriesBarChart', () => {
  it('renders the title and the running total', () => {
    render(<TimeSeriesBarChart {...buildProps()} />)

    expect(screen.getByText('Resources created')).toBeInTheDocument()
    expect(screen.getByText(/Total created/)).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
  })

  it('hides the total behind a skeleton while loading', () => {
    render(<TimeSeriesBarChart {...buildProps({ loading: true })} />)

    expect(screen.getByText('Resources created')).toBeInTheDocument()
    expect(screen.queryByText('4')).not.toBeInTheDocument()
  })

  it('renders the error message on error', () => {
    render(<TimeSeriesBarChart {...buildProps({ error: true })} />)

    expect(
      screen.getByText("Couldn't load resource creation")
    ).toBeInTheDocument()
  })

  it('renders the empty state when the total is zero', () => {
    render(<TimeSeriesBarChart {...buildProps({ total: 0, data: [] })} />)

    expect(screen.getByText('No resources created yet')).toBeInTheDocument()
  })

  it('renders no legend when legend is none', () => {
    render(<TimeSeriesBarChart {...buildProps({ legend: 'none' })} />)

    // Without a legend (and with no hover-driven tooltip) the series labels are
    // absent from the DOM.
    expect(screen.queryByText('Prompts')).not.toBeInTheDocument()
  })

  it('renders a clickable strip legend that toggles a series', () => {
    render(<TimeSeriesBarChart {...buildProps({ legend: 'strip' })} />)

    const promptsToggle = screen.getByRole('button', { name: 'Prompts' })
    expect(promptsToggle).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(promptsToggle)

    expect(promptsToggle).toHaveAttribute('aria-pressed', 'false')
  })

  it('renders the in-card range selector by default', () => {
    render(<TimeSeriesBarChart {...buildProps()} />)

    expect(screen.getByRole('combobox')).toBeInTheDocument()
  })

  it('hides the in-card range selector when hideRangeControl is set', () => {
    render(<TimeSeriesBarChart {...buildProps({ hideRangeControl: true })} />)

    // The card still renders, but an external (page-level) control owns the range.
    expect(screen.getByText('Resources created')).toBeInTheDocument()
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  })
})

// The breakdown legend (one row per series with a proportional bar + count),
// migrated from the former AccessActivityCard. Exercised via the compact size,
// matching how AccessActivityPanel renders it.
describe('TimeSeriesBarChart — breakdown legend', () => {
  const BREAKDOWN_SERIES = [
    { key: 'web', label: 'Web', fill: 'var(--chart-1)' },
    { key: 'cli', label: 'CLI', fill: 'var(--chart-2)' },
    { key: 'mcp', label: 'MCP', fill: 'var(--chart-3)' },
    { key: 'api', label: 'API', fill: 'var(--chart-4)' },
  ]
  const BREAKDOWN_DATA = [
    { date: '2026-05-01', web: 10, cli: 4, mcp: 6, api: 2, total: 22 },
    { date: '2026-05-02', web: 14, cli: 3, mcp: 1, api: 1, total: 19 },
  ]

  function breakdownProps(
    overrides: Partial<TimeSeriesBarChartProps> = {}
  ): TimeSeriesBarChartProps {
    return buildProps({
      title: 'Access activity',
      totalLabel: 'Total accesses',
      total: 41,
      series: BREAKDOWN_SERIES,
      data: BREAKDOWN_DATA,
      errorMessage: "Couldn't load access activity",
      emptyMessage: 'No activity yet',
      legend: 'breakdown',
      size: 'compact',
      stacked: true,
      ...overrides,
    })
  }

  function breakdownRows() {
    const list = screen.getByRole('list')
    return within(list).getAllByRole('listitem')
  }

  it('sums per-series counts across all days', () => {
    render(<TimeSeriesBarChart {...breakdownProps()} />)
    // Web 24, MCP 7, CLI 7, API 3
    expect(screen.getByText('24')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getAllByText('7')).toHaveLength(2)
  })

  it('lists every series and leads with the highest count', () => {
    render(<TimeSeriesBarChart {...breakdownProps()} />)
    const names = breakdownRows().map(
      row => within(row).getByText(/^(Web|MCP|CLI|API)$/).textContent
    )
    expect(names).toHaveLength(4)
    expect(new Set(names)).toEqual(new Set(['Web', 'MCP', 'CLI', 'API']))
    // Web (24) is the largest total, so it leads the descending breakdown.
    expect(names[0]).toBe('Web')
  })

  it('scales the breakdown bars relative to the largest series', () => {
    render(<TimeSeriesBarChart {...breakdownProps()} />)
    const rows = breakdownRows()
    const webBar = rows[0].querySelector('span > span')!
    // Web is the max series, so its bar fills the full track.
    expect(webBar).toHaveStyle({ width: '100.0%' })
  })

  it('still lists a series with zero count when others have data', () => {
    render(
      <TimeSeriesBarChart
        {...breakdownProps({
          total: 10,
          data: [
            { date: '2026-05-01', web: 10, cli: 0, mcp: 0, api: 0, total: 10 },
          ],
        })}
      />
    )
    expect(screen.getByText('API')).toBeInTheDocument()
    // Three series report 0.
    expect(screen.getAllByText('0')).toHaveLength(3)
  })

  it('renders first and last date ticks under the compact chart', () => {
    render(<TimeSeriesBarChart {...breakdownProps()} />)
    expect(screen.getByText('May 1')).toBeInTheDocument()
    expect(screen.getByText('May 2')).toBeInTheDocument()
  })

  it('hides the breakdown list while loading', () => {
    render(<TimeSeriesBarChart {...breakdownProps({ loading: true })} />)
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })
})

describe('parseLocalDate', () => {
  it('parses a date-only string as local midnight, not UTC', () => {
    const date = parseLocalDate('2026-05-01')

    expect(date.getFullYear()).toBe(2026)
    expect(date.getMonth()).toBe(4) // May, zero-indexed
    expect(date.getDate()).toBe(1)
    expect(date.getHours()).toBe(0)
  })
})
