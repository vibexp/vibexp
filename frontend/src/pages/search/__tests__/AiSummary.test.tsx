import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import type { SearchSummaryResponse } from '@/services/searchService'
import { storage } from '@/utils/storage'

import { AiSummary } from '../AiSummary'

type Props = ComponentProps<typeof AiSummary>

const data: SearchSummaryResponse = {
  summary: 'Retries are configured per client [1], see also [2].',
  sources: [
    {
      index: 1,
      type: 'memory',
      id: 'mem-1',
      title: 'Retry memory',
      slug: '',
      project_id: 'proj-1',
      project_name: 'Project One',
      updated_at: '2026-01-01T00:00:00Z',
      truncated: false,
    },
    {
      index: 2,
      type: 'prompt',
      id: 'pr-2',
      title: 'Retry prompt',
      slug: 'retry-prompt',
      project_id: 'proj-1',
      project_name: 'Project One',
      updated_at: '2026-01-01T00:00:00Z',
      truncated: true,
    },
  ],
  model: 'gpt-test-mini',
  provider_id: 'prov-1',
  generated_at: '2026-01-01T00:00:00Z',
}

function renderSummary(overrides: Partial<Props> = {}) {
  const props: Props = {
    availability: { available: true, enabled: true },
    canConfigure: false,
    teamId: 'team-1',
    state: undefined,
    onGenerate: vi.fn(),
    onRetry: vi.fn(),
    isOnPage: vi.fn(() => false),
    onShowResult: vi.fn(),
    ...overrides,
  }
  const view = render(
    <MemoryRouter initialEntries={['/search?q=x']}>
      <Routes>
        <Route path="/search" element={<AiSummary {...props} />} />
        <Route path="*" element={<div data-testid="navigated" />} />
      </Routes>
    </MemoryRouter>
  )
  return { ...view, props }
}

async function expand() {
  await userEvent.click(screen.getByRole('button', { name: /AI Summary/ }))
}

describe('AiSummary', () => {
  beforeEach(() => {
    storage.remove(STORAGE_KEYS.SEARCH_AI_SUMMARY_EXPANDED)
  })

  it.each([
    ['absent', undefined],
    ['disabled', { available: true, enabled: false }],
    ['disabled and unavailable', { available: false, enabled: false }],
  ])('renders nothing when ai_summary is %s', (_, availability) => {
    const { container } = renderSummary({ availability, canConfigure: true })
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when unavailable and the user cannot configure', () => {
    const { container } = renderSummary({
      availability: { available: false, enabled: true },
      canConfigure: false,
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('links a user who can configure to the model providers settings', () => {
    renderSummary({
      availability: { available: false, enabled: true },
      canConfigure: true,
    })
    expect(
      screen.getByRole('link', {
        name: /Configure a model provider to enable AI Summary/,
      })
    ).toHaveAttribute('href', '/teams/team-1/settings/model-providers')
  })

  it('starts collapsed and generates nothing', () => {
    const { props } = renderSummary()
    expect(screen.getByRole('button', { name: /AI Summary/ })).toHaveAttribute(
      'aria-expanded',
      'false'
    )
    expect(props.onGenerate).not.toHaveBeenCalled()
  })

  it('asks for a summary on expand and remembers the preference', async () => {
    const { props } = renderSummary()
    await expand()
    expect(props.onGenerate).toHaveBeenCalledTimes(1)
    expect(storage.getJSON(STORAGE_KEYS.SEARCH_AI_SUMMARY_EXPANDED)).toBe(true)
  })

  it('opens expanded when the user left it expanded', () => {
    storage.set(STORAGE_KEYS.SEARCH_AI_SUMMARY_EXPANDED, true)
    const { props } = renderSummary()
    expect(screen.getByRole('button', { name: /AI Summary/ })).toHaveAttribute(
      'aria-expanded',
      'true'
    )
    expect(props.onGenerate).toHaveBeenCalledTimes(1)
  })

  it('does not ask again when the search already has a summary', async () => {
    const { props } = renderSummary({ state: { status: 'ready', data } })
    await expand()
    expect(props.onGenerate).not.toHaveBeenCalled()
  })

  it('still renders collapsed when storage is unavailable', () => {
    const getItem = vi
      .spyOn(Storage.prototype, 'getItem')
      .mockImplementation(() => {
        throw new Error('SecurityError')
      })
    try {
      renderSummary()
      expect(
        screen.getByRole('button', { name: /AI Summary/ })
      ).toHaveAttribute('aria-expanded', 'false')
    } finally {
      getItem.mockRestore()
    }
  })

  it('shows a skeleton while generating', async () => {
    renderSummary({ state: { status: 'loading' } })
    await expand()
    expect(screen.getByTestId('ai-summary-skeleton')).toBeInTheDocument()
  })

  it('renders the summary with citations, sources and the model', async () => {
    renderSummary({ state: { status: 'ready', data } })
    await expand()

    expect(screen.getByText(/Retries are configured per client/)).toBeVisible()
    const citation = screen.getByRole('link', { name: '[1]' })
    expect(citation).toHaveAttribute('href', '/memories/mem-1')

    expect(screen.getByRole('link', { name: 'Retry memory' })).toHaveAttribute(
      'href',
      '/memories/mem-1'
    )
    expect(screen.getByRole('link', { name: 'Retry prompt' })).toHaveAttribute(
      'href',
      '/prompts/retry-prompt'
    )
    expect(screen.getByText('(truncated)')).toBeInTheDocument()
    expect(screen.getByText('Generated by gpt-test-mini')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Retry/ })
    ).not.toBeInTheDocument()
  })

  it('scrolls to a cited result that is on the page', async () => {
    const { props } = renderSummary({
      state: { status: 'ready', data },
      isOnPage: vi.fn((id: string) => id === 'mem-1'),
    })
    await expand()
    await userEvent.click(screen.getByRole('link', { name: '[1]' }))
    expect(props.onShowResult).toHaveBeenCalledWith('mem-1')
    expect(screen.queryByTestId('navigated')).not.toBeInTheDocument()
  })

  it('opens a cited resource that is not on the page', async () => {
    const { props } = renderSummary({ state: { status: 'ready', data } })
    await expand()
    await userEvent.click(screen.getByRole('link', { name: '[2]' }))
    expect(props.onShowResult).not.toHaveBeenCalled()
    expect(screen.getByTestId('navigated')).toBeInTheDocument()
  })

  it('never injects a scripted summary', async () => {
    renderSummary({
      state: {
        status: 'ready',
        data: {
          ...data,
          summary:
            'Evil <script>window.pwned = 1</script><img src="x" onerror="window.pwned = 1">',
        },
      },
    })
    await expand()
    const region = screen.getByText(/Evil/)
    expect(region.querySelector('script')).toBeNull()
    expect(region.innerHTML).not.toContain('onerror')
  })

  it('shows the classified error with a working Retry', async () => {
    const { props } = renderSummary({
      state: {
        status: 'error',
        code: 'AI_SUMMARY_TIMEOUT',
        message: 'The model took too long to respond.',
      },
    })
    await expand()
    expect(screen.getByRole('alert')).toHaveTextContent(
      'The model took too long to respond.'
    )
    await userEvent.click(screen.getByRole('button', { name: /Retry/ }))
    expect(props.onRetry).toHaveBeenCalledTimes(1)
  })
})
