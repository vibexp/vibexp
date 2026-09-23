import { act, render, screen, waitFor, within } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import type { Mock } from 'vitest'

import { summaryKey } from '@/pages/search/aiSummary'
import {
  getSearchSummary,
  resetSearchSummaries,
} from '@/pages/search/searchSummaryStore'
import type { TeamAISummarySettings } from '@/services/aiSummarySettingsService'
import type { SearchSummaryResponse } from '@/services/searchService'
import { ApiError } from '@/types/errors'

const mockNavigate = vi.hoisted(() => vi.fn())
vi.mock('react-router', async () => ({
  ...(await vi.importActual('react-router')),
  useNavigate: () => mockNavigate,
}))

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: { id: 'team-1', name: 'Test Team' } }),
}))

vi.mock('@/services/aiSummarySettingsService', () => ({
  aiSummarySettingsService: { getAISummarySettings: vi.fn() },
}))

vi.mock('@/services/searchService', () => ({
  searchService: { summarize: vi.fn() },
}))

import { aiSummarySettingsService } from '@/services/aiSummarySettingsService'
import { searchService } from '@/services/searchService'

import { SearchModal } from '../SearchModal'

const mockGetSettings = aiSummarySettingsService.getAISummarySettings as Mock
const mockSummarize = searchService.summarize as Mock

function settings(available: boolean, enabled: boolean): TeamAISummarySettings {
  const values = {
    enabled,
    model_provider_id: null,
    top_n: 5,
    style: 'concise',
    max_output_tokens: 512,
  } as TeamAISummarySettings['values']
  return {
    source: 'instance',
    values,
    instance_defaults: values,
    max_top_n: 10,
    available,
  }
}

const summary: SearchSummaryResponse = {
  summary: 'Retries are configured per client [1].',
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
  ],
  model: 'gpt-test',
  provider_id: 'prov-1',
  generated_at: '2026-01-01T00:00:00Z',
}

// Radix sets pointer-events:none on the body while a dialog is open; disable
// userEvent's pointer-events guard so clicks inside the portal still register.
function setup() {
  const user = userEvent.setup({ pointerEventsCheck: 0 })
  render(
    <MemoryRouter>
      <SearchModal />
    </MemoryRouter>
  )
  return user
}

async function openModal(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Search' }))
  return screen.getByRole('dialog')
}

describe('SearchModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetSearchSummaries()
    mockGetSettings.mockResolvedValue(settings(true, true))
    mockSummarize.mockResolvedValue(summary)
  })

  it('opens the dialog with a query textarea and a submit button', async () => {
    const user = setup()
    const dialog = await openModal(user)

    expect(
      within(dialog).getByRole('textbox', { name: 'Search query' })
    ).toBeInTheDocument()
    expect(
      within(dialog).getByRole('button', { name: /search/i })
    ).toBeInTheDocument()
  })

  it('disables the Search button until a non-empty query is entered', async () => {
    const user = setup()
    const dialog = await openModal(user)

    const submit = within(dialog).getByRole('button', { name: /search/i })
    expect(submit).toBeDisabled()

    await user.type(
      within(dialog).getByRole('textbox', { name: 'Search query' }),
      '  '
    )
    expect(submit).toBeDisabled() // whitespace only

    await user.type(
      within(dialog).getByRole('textbox', { name: 'Search query' }),
      'retry config'
    )
    expect(submit).toBeEnabled()
  })

  it('navigates to the encoded results URL when the Search button is clicked', async () => {
    const user = setup()
    const dialog = await openModal(user)

    await user.type(
      within(dialog).getByRole('textbox', { name: 'Search query' }),
      'a & b'
    )
    await user.click(within(dialog).getByRole('button', { name: /search/i }))

    expect(mockNavigate).toHaveBeenCalledWith('/search?q=a%20%26%20b')
  })

  it('submits on Enter but inserts a newline on Shift+Enter', async () => {
    const user = setup()
    const dialog = await openModal(user)
    const textarea = within(dialog).getByRole('textbox', {
      name: 'Search query',
    })

    // Shift+Enter must NOT submit
    await user.type(textarea, 'line one{Shift>}{Enter}{/Shift}line two')
    expect(mockNavigate).not.toHaveBeenCalled()

    // Plain Enter submits the accumulated value
    await user.type(textarea, '{Enter}')
    expect(mockNavigate).toHaveBeenCalledTimes(1)
    expect(mockNavigate).toHaveBeenCalledWith(
      expect.stringContaining('/search?q=')
    )
  })

  it('does not navigate when the query is empty or whitespace', async () => {
    const user = setup()
    const dialog = await openModal(user)
    const textarea = within(dialog).getByRole('textbox', {
      name: 'Search query',
    })

    await user.type(textarea, '   {Enter}')
    expect(mockNavigate).not.toHaveBeenCalled()
  })
})

describe('SearchModal AI Summary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetSearchSummaries()
    mockGetSettings.mockResolvedValue(settings(true, true))
    mockSummarize.mockResolvedValue(summary)
  })

  const row = () => screen.findByRole('button', { name: /AI Summary/ })

  async function typeQuery(
    user: ReturnType<typeof userEvent.setup>,
    dialog: HTMLElement,
    text: string
  ) {
    await user.type(
      within(dialog).getByRole('textbox', { name: 'Search query' }),
      text
    )
  }

  it('shows a collapsed row once a query is typed, without generating', async () => {
    const user = setup()
    const dialog = await openModal(user)
    await waitFor(() => {
      expect(mockGetSettings).toHaveBeenCalledWith('team-1')
    })
    expect(
      within(dialog).queryByRole('button', { name: /AI Summary/ })
    ).not.toBeInTheDocument()

    await typeQuery(user, dialog, 'retry config')

    expect(await row()).toHaveAttribute('aria-expanded', 'false')
    expect(mockSummarize).not.toHaveBeenCalled()
  })

  it.each([
    ['no model provider', () => Promise.resolve(settings(false, true))],
    ['AI Summary turned off', () => Promise.resolve(settings(true, false))],
    ['the settings call failing', () => Promise.reject(new Error('boom'))],
  ])('hides the row with %s', async (_, response) => {
    mockGetSettings.mockImplementation(response)
    const user = setup()
    const dialog = await openModal(user)
    await typeQuery(user, dialog, 'retry config')
    await waitFor(() => {
      expect(mockGetSettings).toHaveBeenCalled()
    })

    expect(
      within(dialog).queryByRole('button', { name: /AI Summary/ })
    ).not.toBeInTheDocument()
    expect(within(dialog).queryByText(/Configure/)).not.toBeInTheDocument()
  })

  it('generates exactly once across expand, collapse and expand', async () => {
    const user = setup()
    const dialog = await openModal(user)
    await typeQuery(user, dialog, 'retry config')

    await user.click(await row())
    expect(await screen.findByText('Generated by gpt-test')).toBeVisible()
    await user.click(await row())
    await user.click(await row())
    expect(await screen.findByText('Generated by gpt-test')).toBeVisible()

    expect(mockSummarize).toHaveBeenCalledTimes(1)
    expect(mockSummarize).toHaveBeenCalledWith('team-1', {
      query: 'retry config',
    })
  })

  it('renders a clamped, sanitized answer with superscript citations', async () => {
    mockSummarize.mockResolvedValue({
      ...summary,
      summary: 'Answer [1] <script>window.pwned = true</script>',
    })
    const user = setup()
    const dialog = await openModal(user)
    await typeQuery(user, dialog, 'retry config')
    await user.click(await row())

    const content = await screen.findByTestId('ai-summary-compact')
    expect(content.querySelector('script')).toBeNull()
    expect(content.querySelector('sup[data-citation="1"]')).toHaveTextContent(
      '1'
    )
    expect(content.querySelector('a')).toBeNull()
    expect(content.parentElement).toHaveClass('max-h-40', 'overflow-hidden')
  })

  it('collapses on a query edit and generates anew for the new query', async () => {
    const user = setup()
    const dialog = await openModal(user)
    await typeQuery(user, dialog, 'retry')
    await user.click(await row())
    expect(await screen.findByText('Generated by gpt-test')).toBeVisible()

    await typeQuery(user, dialog, ' policy')

    expect(await row()).toHaveAttribute('aria-expanded', 'false')
    expect(mockSummarize).toHaveBeenCalledTimes(1)

    await user.click(await row())
    await waitFor(() => {
      expect(mockSummarize).toHaveBeenCalledTimes(2)
    })
    expect(mockSummarize).toHaveBeenLastCalledWith('team-1', {
      query: 'retry policy',
    })
  })

  it('shows the classified error with a working Retry', async () => {
    mockSummarize.mockRejectedValueOnce(
      new ApiError({
        type: 'about:blank',
        title: 'Service Unavailable',
        status: 503,
        detail: 'unreachable',
        code: 'AI_SUMMARY_PROVIDER_UNREACHABLE',
        request_id: 'req-1',
      } as ConstructorParameters<typeof ApiError>[0])
    )
    const user = setup()
    const dialog = await openModal(user)
    await typeQuery(user, dialog, 'retry config')
    await user.click(await row())

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('The model provider could not be reached')

    await user.click(within(alert).getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Generated by gpt-test')).toBeVisible()
    expect(mockSummarize).toHaveBeenCalledTimes(2)
  })

  it('"See full results" opens /search with the summary pre-expanded', async () => {
    const user = setup()
    const dialog = await openModal(user)
    await typeQuery(user, dialog, 'a & b')
    await user.click(await row())

    await user.click(
      await screen.findByRole('button', { name: /See full results/ })
    )

    expect(mockNavigate).toHaveBeenCalledWith(
      '/search?q=a%20%26%20b&summary=open'
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('still submits on Enter with the row showing', async () => {
    const user = setup()
    const dialog = await openModal(user)
    await typeQuery(user, dialog, 'retry config')
    await row()

    await typeQuery(user, dialog, '{Enter}')

    expect(mockNavigate).toHaveBeenCalledWith('/search?q=retry%20config')
    expect(mockSummarize).not.toHaveBeenCalled()
  })

  it('caches an answer that lands after the dialog closed, without warnings', async () => {
    const consoleError = vi.spyOn(console, 'error')
    let resolve: (value: SearchSummaryResponse) => void = () => undefined
    mockSummarize.mockReturnValue(
      new Promise<SearchSummaryResponse>(r => {
        resolve = r
      })
    )
    const user = setup()
    const dialog = await openModal(user)
    await typeQuery(user, dialog, 'retry config')
    await user.click(await row())
    expect(await screen.findByTestId('ai-summary-skeleton')).toBeVisible()

    await user.keyboard('{Escape}')
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
    await act(async () => {
      resolve(summary)
      await Promise.resolve()
    })

    expect(
      getSearchSummary(
        summaryKey('team-1', 'retry config', undefined, undefined)
      )
    ).toEqual({ status: 'ready', data: summary })
    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })
})
