import type { Row } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'

import type { Artifact } from '@/services/artifactService'

import { buildArtifactsColumns } from '../artifactsColumns'

function renderUpdatedCell(updatedAt: string) {
  const columns = buildArtifactsColumns({
    navigate: vi.fn(),
    onDelete: vi.fn(),
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
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2024-06-01T12:00:00Z'))
    renderUpdatedCell(new Date('2024-06-01T09:00:00Z').toISOString())
    expect(screen.getByText('3h ago')).toBeInTheDocument()
    vi.useRealTimers()
  })
})

function renderTitleCell(artifact: Partial<Artifact>) {
  const columns = buildArtifactsColumns({
    navigate: vi.fn(),
    onDelete: vi.fn(),
    canDelete: () => true,
  })
  const titleCol = columns.find(
    col => 'accessorKey' in col && col.accessorKey === 'title'
  )
  if (!titleCol?.cell || typeof titleCol.cell !== 'function') {
    throw new Error('Title column cell renderer not found')
  }
  const cell = titleCol.cell as (ctx: { row: Row<Artifact> }) => ReactNode
  const row = {
    original: {
      title: 'Runbook',
      slug: 'runbook',
      project_id: 'p1',
      ...artifact,
    },
  } as Row<Artifact>
  // In a router: the title is a real link (react-router Link).
  return render(<MemoryRouter>{cell({ row })}</MemoryRouter>)
}

describe('artifactsColumns — stale badge (#738)', () => {
  it('shows the badge for a stale artifact', () => {
    renderTitleCell({
      freshness: {
        status: 'stale',
        since: '2026-08-01T00:00:00Z',
        matched_rule_ids: ['r1'],
        reason: 'rule_run',
      },
    })
    expect(screen.getByTestId('freshness-badge')).toBeInTheDocument()
  })

  it('shows no badge for a fresh artifact', () => {
    // A fresh resource omits the field entirely, so the row must be unchanged
    // from before this feature — no badge, no reserved space.
    renderTitleCell({})
    expect(screen.queryByTestId('freshness-badge')).not.toBeInTheDocument()
  })

  it('still renders the title link for a stale artifact', () => {
    renderTitleCell({
      freshness: {
        status: 'stale',
        since: '2026-08-01T00:00:00Z',
        matched_rule_ids: ['r1'],
        reason: 'rule_run',
      },
    })
    expect(screen.getByRole('link', { name: 'Runbook' })).toBeInTheDocument()
  })
})

const lock = {
  locked_by: { user_id: 'user-2', display_name: 'Ada Lovelace' },
  comment: 'Used by the nightly release agent',
  locked_at: '2026-10-11T10:15:30Z',
}

describe('artifactsColumns — lock icon (#1292)', () => {
  it('shows the icon, naming the locker, for a locked artifact', () => {
    renderTitleCell({ lock })
    expect(
      screen.getByRole('img', { name: 'Locked by Ada Lovelace' })
    ).toBeInTheDocument()
  })

  it('shows no icon for an unlocked artifact', () => {
    renderTitleCell({ lock: null })
    expect(screen.queryByTestId('lock-indicator')).not.toBeInTheDocument()
  })

  it('shows the stale badge and the lock icon together', () => {
    renderTitleCell({
      lock,
      freshness: {
        status: 'stale',
        since: '2026-08-01T00:00:00Z',
        matched_rule_ids: ['r1'],
        reason: 'rule_run',
      },
    })
    expect(screen.getByTestId('freshness-badge')).toBeInTheDocument()
    expect(screen.getByTestId('lock-indicator')).toBeInTheDocument()
    // The name is still the row's link.
    expect(screen.getByRole('link', { name: 'Runbook' })).toBeInTheDocument()
  })
})

function renderActionsCell(
  artifact: Partial<Artifact>,
  canDelete: (artifact: Artifact) => boolean = () => true
) {
  const columns = buildArtifactsColumns({
    navigate: vi.fn(),
    onDelete: vi.fn(),
    canDelete,
  })
  const actionsCol = columns.find(col => col.id === 'actions')
  if (!actionsCol?.cell || typeof actionsCol.cell !== 'function') {
    throw new Error('Actions column cell renderer not found')
  }
  const cell = actionsCol.cell as (ctx: { row: Row<Artifact> }) => ReactNode
  const row = {
    original: {
      title: 'Runbook',
      slug: 'runbook',
      project_id: 'p1',
      ...artifact,
    },
  } as Row<Artifact>
  return render(<>{cell({ row })}</>)
}

describe('artifactsColumns — Delete on a locked row (#1292)', () => {
  it('offers Delete on an unlocked row the user may delete', () => {
    renderActionsCell({ lock: null })
    expect(screen.getByTestId('delete-artifact-button')).toBeInTheDocument()
  })

  it('offers no Delete on a locked row, whatever the permission says', () => {
    renderActionsCell({ lock })
    expect(
      screen.queryByTestId('delete-artifact-button')
    ).not.toBeInTheDocument()
    // View and Edit are not gated by the lock here.
    expect(screen.getByTestId('view-artifact-button')).toBeInTheDocument()
    expect(screen.getByTestId('edit-artifact-button')).toBeInTheDocument()
  })

  it('still hides Delete on an unlocked row the user may not delete', () => {
    renderActionsCell({ lock: null }, () => false)
    expect(
      screen.queryByTestId('delete-artifact-button')
    ).not.toBeInTheDocument()
  })
})
