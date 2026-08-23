import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import type { ResourceVersion } from '@/types/version'

const showSuccess = vi.fn()
const handleError = vi.fn()
vi.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess, showError: vi.fn() }),
}))
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError }),
}))

// jsdom gaps that Radix (dropdown / alert-dialog) relies on.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

import type { VersionHistorySource } from '../types'
import { VersionHistoryPage } from '../VersionHistoryPage'

function snapshot(
  n: number,
  content: string,
  summary: string
): ResourceVersion {
  return {
    id: `v${String(n)}`,
    team_id: 'team',
    resource_type: 'artifact',
    resource_id: 'res',
    version_number: n,
    content,
    change_summary: summary,
    actor_type: 'human',
    created_by: 'user',
    author: {
      id: 'user',
      display_name: 'Shaharia',
      avatar_url: null,
      initials: 'SA',
    },
    created_at: '2026-06-12T10:00:00.000Z',
  }
}

function buildSource(
  overrides: Partial<VersionHistorySource> = {}
): VersionHistorySource {
  return {
    resourceType: 'artifact',
    resourceLabel: 'artifact',
    backHref: '/artifacts/p/s',
    load: vi.fn().mockResolvedValue({
      currentContent: 'live content\nline two',
      currentUpdatedAt: '2026-06-12T12:00:00.000Z',
      resourceName: 'My artifact',
      versions: [
        snapshot(2, 'second content', 'Second edit'),
        snapshot(1, 'first content', 'Created the artifact'),
      ],
    }),
    restore: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  }
}

function renderPage(source: VersionHistorySource) {
  return render(
    <MemoryRouter>
      <VersionHistoryPage source={source} />
    </MemoryRouter>
  )
}

describe('VersionHistoryPage', () => {
  it('renders the timeline with a Current tag and change summaries', async () => {
    renderPage(buildSource())

    expect(await screen.findByText('Second edit')).toBeInTheDocument()
    expect(screen.getByText('Created the artifact')).toBeInTheDocument()
    expect(screen.getByText('Current')).toBeInTheDocument()
    // synthesized current row = maxSnapshot + 1
    expect(screen.getByText('Version 3')).toBeInTheDocument()
  })

  it('enables Compare only when exactly two rows are selected, then opens the takeover', async () => {
    const user = userEvent.setup()
    renderPage(buildSource())

    await screen.findByText('Second edit')
    const compareButton = screen.getByTestId('compare-button')
    expect(compareButton).toBeDisabled()

    await user.click(screen.getByLabelText('Select version 3'))
    await user.click(screen.getByLabelText('Select version 2'))
    expect(compareButton).toBeEnabled()

    await user.click(compareButton)
    expect(screen.getByTestId('version-compare-view')).toBeInTheDocument()
    expect(screen.getByTestId('version-diff-split')).toBeInTheDocument()
  })

  it('routes restore through the non-destructive confirmation dialog', async () => {
    const user = userEvent.setup()
    const source = buildSource()
    renderPage(source)

    await screen.findByText('Second edit')
    await user.click(screen.getByLabelText('Restore version 2'))

    const dialog = await screen.findByTestId('restore-version-dialog')
    expect(within(dialog).getByText(/Restore Version 2\?/)).toBeInTheDocument()
    expect(within(dialog).getByText(/non-destructive/i)).toBeInTheDocument()

    await user.click(screen.getByTestId('confirm-restore-button'))
    await waitFor(() => {
      expect(source.restore).toHaveBeenCalledWith(2)
    })
  })

  it('does not offer Restore on the current (live) row', async () => {
    renderPage(buildSource())
    await screen.findByText('Second edit')
    expect(screen.queryByLabelText('Restore version 3')).not.toBeInTheDocument()
  })

  describe('date range filter', () => {
    const NOW = new Date('2026-06-20T12:00:00.000Z').getTime()
    const at = (msAgo: number) => new Date(NOW - msAgo).toISOString()
    const HOUR = 60 * 60 * 1000
    const DAY = 24 * HOUR

    // One snapshot just inside each bound and one just outside, so a filter
    // that used the wrong reference instant or a `>=`/`>` slip is visible.
    function agedSource(): VersionHistorySource {
      const aged = (n: number, summary: string, createdAt: string) => ({
        ...snapshot(n, `content ${String(n)}`, summary),
        created_at: createdAt,
      })
      return buildSource({
        load: vi.fn().mockResolvedValue({
          currentContent: 'live content',
          currentUpdatedAt: at(0),
          resourceName: 'My artifact',
          versions: [
            aged(4, 'Just now', at(HOUR)),
            aged(3, 'Two days ago', at(2 * DAY)),
            aged(2, 'Ten days ago', at(10 * DAY)),
            aged(1, 'Ninety days ago', at(90 * DAY)),
          ],
        }),
      })
    }

    const pickRange = async (
      user: ReturnType<typeof userEvent.setup>,
      label: string
    ) => {
      await user.click(screen.getByRole('button', { name: /All time/ }))
      await user.click(await screen.findByRole('menuitem', { name: label }))
    }

    beforeEach(() => {
      vi.useFakeTimers({ shouldAdvanceTime: true })
      vi.setSystemTime(NOW)
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it('shows every entry under "All time"', async () => {
      renderPage(agedSource())

      expect(await screen.findByText('Just now')).toBeInTheDocument()
      expect(screen.getByText('Ninety days ago')).toBeInTheDocument()
    })

    it.each([
      ['Last 24 hours', ['Just now'], ['Two days ago', 'Ninety days ago']],
      ['Last 7 days', ['Just now', 'Two days ago'], ['Ten days ago']],
      [
        'Last 30 days',
        ['Just now', 'Two days ago', 'Ten days ago'],
        ['Ninety days ago'],
      ],
    ])('filters to %s', async (label, visible, hidden) => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
      renderPage(agedSource())
      await screen.findByText('Just now')

      await pickRange(user, label)

      for (const summary of visible) {
        expect(screen.getByText(summary)).toBeInTheDocument()
      }
      for (const summary of hidden) {
        expect(screen.queryByText(summary)).not.toBeInTheDocument()
      }
    })

    // The reference instant is re-stamped when the range is chosen, so a page
    // left open does not keep measuring "last 24 hours" from when it mounted.
    it('measures the window from when the range was picked, not from mount', async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
      renderPage(agedSource())
      await screen.findByText('Just now')

      // The page sits open long enough that the 1-hour-old entry ages out.
      vi.setSystemTime(NOW + 2 * DAY)

      await pickRange(user, 'Last 24 hours')

      expect(screen.queryByText('Just now')).not.toBeInTheDocument()
      expect(screen.queryByText('Two days ago')).not.toBeInTheDocument()
    })
  })
})
