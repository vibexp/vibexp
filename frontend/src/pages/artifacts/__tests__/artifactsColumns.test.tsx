import type { Row } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { Artifact } from '@/services/artifactService'

import { buildArtifactsColumns } from '../artifactsColumns'

// The Updated column now renders RelativeTime (Radix Tooltip → ResizeObserver).
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
})

function renderUpdatedCell(updatedAt: string) {
  const columns = buildArtifactsColumns({
    navigate: jest.fn(),
    onDelete: jest.fn(),
    canDelete: () => true,
  })
  const updatedCol = columns.find(
    col => 'accessorKey' in col && col.accessorKey === 'updated_at'
  )
  if (!updatedCol?.cell || typeof updatedCol.cell !== 'function') {
    throw new Error('Updated column cell renderer not found')
  }
  const cell = updatedCol.cell as (ctx: { row: Row<Artifact> }) => ReactNode
  const row = { original: { updated_at: updatedAt } } as Row<Artifact>
  return render(<>{cell({ row })}</>)
}

describe('artifactsColumns — Updated column', () => {
  it('renders a compact short date for old dates', () => {
    renderUpdatedCell('2024-01-15T12:00:00Z')
    expect(screen.getByText(/Jan 15, 2024/)).toBeInTheDocument()
  })

  it('does not render the old inline date-time (local formatDate removed)', () => {
    renderUpdatedCell('2024-01-15T12:00:00Z')
    const label = screen.getByText(/Jan 15, 2024/)
    // The old local formatDate appended an "HH:MM" time inline; the compact
    // relative label must not.
    expect(label.textContent).not.toMatch(/\d{1,2}:\d{2}/)
  })

  it('renders a relative label for a recent date', () => {
    jest.useFakeTimers()
    jest.setSystemTime(new Date('2024-06-01T12:00:00Z'))
    renderUpdatedCell(new Date('2024-06-01T09:00:00Z').toISOString())
    expect(screen.getByText('3h ago')).toBeInTheDocument()
    jest.useRealTimers()
  })
})
