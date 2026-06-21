import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { ResourceVersion } from '@/types/version'

const showSuccess = jest.fn()
const handleError = jest.fn()
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess, showError: jest.fn() }),
}))
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError }),
}))

// jsdom gaps that Radix (dropdown / alert-dialog) relies on.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
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
    load: jest.fn().mockResolvedValue({
      currentContent: 'live content\nline two',
      currentUpdatedAt: '2026-06-12T12:00:00.000Z',
      resourceName: 'My artifact',
      versions: [
        snapshot(2, 'second content', 'Second edit'),
        snapshot(1, 'first content', 'Created the artifact'),
      ],
    }),
    restore: jest.fn().mockResolvedValue(undefined),
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
})
