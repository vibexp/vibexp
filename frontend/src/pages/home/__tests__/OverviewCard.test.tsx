import { FileText } from 'lucide-react'
import { render, screen } from '@testing-library/react'

import { OverviewCard, type OverviewStat } from '../OverviewCard'

function makeStat(overrides: Partial<OverviewStat> = {}): OverviewStat {
  return {
    label: 'Total Artifacts',
    value: 1862,
    icon: FileText,
    trend: null,
    ...overrides,
  }
}

describe('OverviewCard', () => {
  it('renders the label and a thousands-formatted value', () => {
    render(<OverviewCard stat={makeStat()} loading={false} />)
    expect(screen.getByText('Total Artifacts')).toBeInTheDocument()
    expect(screen.getByText('1,862')).toBeInTheDocument()
  })

  it('renders an up-trend badge and "this week" subtitle', () => {
    render(
      <OverviewCard
        stat={makeStat({
          trend: { label: '+96', tone: 'up' },
          subtitle: '+96 this week',
        })}
        loading={false}
      />
    )
    expect(screen.getByText('+96')).toBeInTheDocument()
    expect(screen.getByText('+96 this week')).toBeInTheDocument()
  })

  it('hides the value (shows a skeleton) and trend while loading', () => {
    render(
      <OverviewCard
        stat={makeStat({ trend: { label: '+96', tone: 'up' } })}
        loading
      />
    )
    expect(screen.queryByText('1,862')).not.toBeInTheDocument()
    expect(screen.queryByText('+96')).not.toBeInTheDocument()
  })
})
