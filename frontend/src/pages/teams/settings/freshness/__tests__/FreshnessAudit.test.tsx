import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

vi.mock('@/services/freshnessService', () => ({
  freshnessService: { getAudit: vi.fn() },
}))

import type { FreshnessAuditEntry } from '@/services/freshnessService'
import { freshnessService } from '@/services/freshnessService'

import { FreshnessAudit } from '../FreshnessAudit'

const mocked = vi.mocked(freshnessService)

const entry = (
  overrides: Partial<FreshnessAuditEntry> = {}
): FreshnessAuditEntry => ({
  id: 'a1',
  resource_type: 'artifact',
  resource_id: '11111111-2222-3333-4444-555555555555',
  rule_id: 'r1',
  action: 'marked',
  reason: 'rule_run',
  created_at: '2026-08-10T12:00:00Z',
  ...overrides,
})

const page = (entries: FreshnessAuditEntry[], totalPages = 1, total = 1) => ({
  entries,
  total_count: total,
  page: 1,
  per_page: 20,
  total_pages: totalPages,
})

beforeEach(() => {
  vi.clearAllMocks()
  mocked.getAudit.mockResolvedValue(page([entry()]))
})

const renderTab = () =>
  render(
    <MemoryRouter>
      <FreshnessAudit teamId="team-1" />
    </MemoryRouter>
  )

describe('FreshnessAudit', () => {
  it('requests the first page for the team', async () => {
    renderTab()

    await waitFor(() => {
      expect(screen.getAllByTestId('audit-row')).toHaveLength(1)
    })
    expect(mocked.getAudit).toHaveBeenCalledWith(
      'team-1',
      1,
      20,
      expect.anything()
    )
  })

  it('explains a mark in words rather than showing raw enums', async () => {
    renderTab()

    await waitFor(() => {
      expect(
        screen.getByText('Marked stale — a rule matched it')
      ).toBeInTheDocument()
    })
  })

  it.each([
    ['accessed' as const, 'Cleared — someone opened it'],
    ['edited' as const, 'Cleared — someone edited it'],
    ['rule_run' as const, 'Cleared — no rule matches it any more'],
  ])('describes a clear caused by %s', async (reason, expected) => {
    mocked.getAudit.mockResolvedValue(
      page([entry({ action: 'cleared', reason, rule_id: null })])
    )
    renderTab()

    await waitFor(() => {
      expect(screen.getByText(expected)).toBeInTheDocument()
    })
  })

  it('links a memory row to the memory', async () => {
    mocked.getAudit.mockResolvedValue(
      page([entry({ resource_type: 'memory', resource_id: 'mem-1' })])
    )
    renderTab()

    await waitFor(() => {
      expect(screen.getByRole('link')).toHaveAttribute(
        'href',
        '/memories/mem-1'
      )
    })
  })

  it.each(['artifact', 'blueprint', 'prompt'] as const)(
    'renders a %s row unlinked, because the payload carries no slug',
    async type => {
      // buildResourceUrl needs a slug (and a project id for artifacts and
      // blueprints); a fabricated href would 404, so the row stays plain text.
      mocked.getAudit.mockResolvedValue(page([entry({ resource_type: type })]))
      renderTab()

      await waitFor(() => {
        expect(screen.getAllByTestId('audit-row')).toHaveLength(1)
      })
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
    }
  )

  it('shows an explanatory empty state', async () => {
    mocked.getAudit.mockResolvedValue(page([], 0, 0))
    renderTab()

    await waitFor(() => {
      expect(screen.getByText(/Nothing logged yet/)).toBeInTheDocument()
    })
  })

  it('hides pagination when there is a single page', async () => {
    renderTab()

    await waitFor(() => {
      expect(screen.getAllByTestId('audit-row')).toHaveLength(1)
    })
    expect(
      screen.queryByRole('button', { name: 'Next' })
    ).not.toBeInTheDocument()
  })

  it('advances to the next page and refetches', async () => {
    const user = userEvent.setup()
    mocked.getAudit.mockResolvedValue(page([entry()], 3, 45))
    renderTab()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Next' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('button', { name: 'Next' }))

    await waitFor(() => {
      expect(mocked.getAudit).toHaveBeenCalledWith(
        'team-1',
        2,
        20,
        expect.anything()
      )
    })
  })

  it('disables Previous on the first page', async () => {
    mocked.getAudit.mockResolvedValue(page([entry()], 3, 45))
    renderTab()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    })
  })

  it('surfaces a load failure', async () => {
    mocked.getAudit.mockRejectedValue(new Error('Audit unavailable'))
    renderTab()

    await waitFor(() => {
      expect(screen.getByText('Audit unavailable')).toBeInTheDocument()
    })
  })

  it('keeps the table and pagination when a later page fails', async () => {
    // Returning only the alert would take the pagination controls with it,
    // stranding the user on a failed page with no way back.
    const user = userEvent.setup()
    mocked.getAudit.mockResolvedValueOnce(page([entry()], 3, 45))
    renderTab()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Next' })).toBeInTheDocument()
    })

    mocked.getAudit.mockRejectedValueOnce(new Error('Page 2 exploded'))
    await user.click(screen.getByRole('button', { name: 'Next' }))

    await waitFor(() => {
      expect(screen.getByText('Page 2 exploded')).toBeInTheDocument()
    })
    expect(screen.getAllByTestId('audit-row')).toHaveLength(1)
    expect(screen.getByRole('button', { name: 'Previous' })).toBeInTheDocument()
  })
})
