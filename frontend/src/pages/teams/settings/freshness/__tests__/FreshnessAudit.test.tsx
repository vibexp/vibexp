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

const PROJECT_ID = '99999999-8888-7777-6666-555555555555'

const entry = (
  overrides: Partial<FreshnessAuditEntry> = {}
): FreshnessAuditEntry => ({
  id: 'a1',
  resource_type: 'artifact',
  resource_id: '11111111-2222-3333-4444-555555555555',
  rule_id: 'r1',
  action: 'marked',
  reason: 'rule_run',
  slug: 'my-artifact',
  project_id: PROJECT_ID,
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

  // All four resource types deep-link now that the payload carries the
  // server-resolved slug/project_id (#789). Memories are keyed by id and carry
  // no slug at all, which is why they are in the same table rather than an
  // exception to it.
  it.each([
    [
      'prompt' as const,
      { slug: 'my-prompt', project_id: PROJECT_ID },
      '/prompts/my-prompt',
    ],
    [
      'artifact' as const,
      { slug: 'my-artifact', project_id: PROJECT_ID },
      `/artifacts/${PROJECT_ID}/my-artifact`,
    ],
    [
      'blueprint' as const,
      { slug: 'my-blueprint', project_id: PROJECT_ID },
      `/blueprints/${PROJECT_ID}/my-blueprint`,
    ],
    [
      'memory' as const,
      { resource_id: 'mem-1', slug: null, project_id: PROJECT_ID },
      '/memories/mem-1',
    ],
  ])('deep-links a %s row', async (type, fields, expected) => {
    mocked.getAudit.mockResolvedValue(
      page([entry({ resource_type: type, ...fields })])
    )
    renderTab()

    await waitFor(() => {
      expect(screen.getByRole('link')).toHaveAttribute('href', expected)
    })
  })

  it.each(['artifact', 'blueprint', 'prompt'] as const)(
    'renders a deleted %s as plain text rather than a broken link',
    async type => {
      // The log is append-only, so an entry outlives its resource. The server
      // resolves null for both identifiers and a fabricated href would 404.
      mocked.getAudit.mockResolvedValue(
        page([entry({ resource_type: type, slug: null, project_id: null })])
      )
      renderTab()

      await waitFor(() => {
        expect(screen.getAllByTestId('audit-row')).toHaveLength(1)
      })
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
    }
  )

  // An artifact/blueprint needs BOTH identifiers; one without the other must not
  // produce a half-built URL.
  it.each([
    ['slug only', { slug: 'my-artifact', project_id: null }],
    ['project only', { slug: null, project_id: PROJECT_ID }],
  ])('renders an artifact with %s as plain text', async (_name, fields) => {
    mocked.getAudit.mockResolvedValue(
      page([entry({ resource_type: 'artifact', ...fields })])
    )
    renderTab()

    await waitFor(() => {
      expect(screen.getAllByTestId('audit-row')).toHaveLength(1)
    })
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

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
