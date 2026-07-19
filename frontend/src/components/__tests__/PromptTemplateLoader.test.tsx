import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { Prompt } from '@/services/promptService'

// Drive every UI branch (loading / error / empty / results) by mutating this
// module-stable state — the component re-reads it on each render. The real
// usePromptSearch is service-backed and is tested through its own consumers.
const searchState: {
  prompts: Prompt[]
  loading: boolean
  error: string | null
  searchPrompts: jest.Mock
  clearResults: jest.Mock
} = {
  prompts: [],
  loading: false,
  error: null,
  searchPrompts: jest.fn(),
  clearResults: jest.fn(),
}
let capturedOptions: unknown
jest.mock('@/hooks/usePromptSearch', () => ({
  usePromptSearch: (options: unknown) => {
    capturedOptions = options
    return searchState
  },
}))

import { PromptTemplateLoader } from '../PromptTemplateLoader'

function buildPrompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    id: 'prompt-1',
    name: 'Code Review Template',
    slug: 'code-review-template',
    description: 'Template for code reviews',
    body: 'Please review this code for: {{criteria}}',
    user_id: 'user-1',
    team_id: 'team-1',
    project_id: 'proj-1',
    status: 'published',
    mcp_expose: true,
    is_shared: false,
    labels: ['code-review'],
    created_at: '2026-01-15T00:00:00Z',
    updated_at: '2026-01-16T00:00:00Z',
    version: 1,
    ...overrides,
  }
}

const onClose = jest.fn()
const onSelectPrompt = jest.fn()

function renderLoader(
  props: Partial<React.ComponentProps<typeof PromptTemplateLoader>> = {}
) {
  return render(
    <PromptTemplateLoader
      isOpen
      onClose={onClose}
      onSelectPrompt={onSelectPrompt}
      {...props}
    />
  )
}

describe('PromptTemplateLoader', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    searchState.prompts = []
    searchState.loading = false
    searchState.error = null
    capturedOptions = undefined
  })

  describe('open/closed', () => {
    it('renders nothing when closed', () => {
      renderLoader({ isOpen: false })

      expect(
        screen.queryByText('Load from Existing Prompt')
      ).not.toBeInTheDocument()
    })

    it('renders the modal with the initial search hint when open', () => {
      renderLoader()

      expect(screen.getByText('Load from Existing Prompt')).toBeInTheDocument()
      expect(
        screen.getByText('Search for a prompt to load')
      ).toBeInTheDocument()
      expect(
        screen.getByText(
          'Start typing to find prompts you can use as templates'
        )
      ).toBeInTheDocument()
    })

    it('focuses the search input shortly after opening', async () => {
      renderLoader()

      const input = screen.getByPlaceholderText(
        'Search prompts by name or content...'
      )
      await waitFor(() => {
        expect(input).toHaveFocus()
      })
    })

    it('passes the exclusion slug and limit through to the search hook', () => {
      renderLoader({ excludeCurrentPrompt: 'current-slug' })

      expect(capturedOptions).toEqual({
        limit: 20,
        excludeCurrentPrompt: 'current-slug',
      })
    })
  })

  describe('searching', () => {
    it('debounces the query into searchPrompts', async () => {
      renderLoader()

      const user = userEvent.setup()
      await user.type(
        screen.getByPlaceholderText('Search prompts by name or content...'),
        'review'
      )

      await waitFor(
        () => {
          expect(searchState.searchPrompts).toHaveBeenCalledWith('review')
        },
        { timeout: 2000 }
      )
      // Intermediate keystrokes were debounced away.
      expect(searchState.searchPrompts).toHaveBeenCalledTimes(1)
    })

    it('clears results instead of searching when the query is emptied', async () => {
      renderLoader()

      const user = userEvent.setup()
      const input = screen.getByPlaceholderText(
        'Search prompts by name or content...'
      )
      await user.type(input, 'review')
      await waitFor(
        () => {
          expect(searchState.searchPrompts).toHaveBeenCalledWith('review')
        },
        { timeout: 2000 }
      )

      searchState.clearResults.mockClear()
      await user.clear(input)

      await waitFor(
        () => {
          expect(searchState.clearResults).toHaveBeenCalled()
        },
        { timeout: 2000 }
      )
      expect(searchState.searchPrompts).toHaveBeenCalledTimes(1)
    })

    it('shows the loading state while a search is in flight', () => {
      searchState.loading = true

      renderLoader()

      expect(screen.getByText('Searching prompts...')).toBeInTheDocument()
      expect(
        screen.queryByText('Search for a prompt to load')
      ).not.toBeInTheDocument()
    })

    it('shows the error state when the search fails', () => {
      searchState.error = 'No team selected'

      renderLoader()

      expect(screen.getByText('No team selected')).toBeInTheDocument()
      expect(
        screen.queryByText('Search for a prompt to load')
      ).not.toBeInTheDocument()
    })

    it('shows the no-results state for a query with no matches', async () => {
      renderLoader()

      const user = userEvent.setup()
      await user.type(
        screen.getByPlaceholderText('Search prompts by name or content...'),
        'zzz'
      )

      expect(await screen.findByText('No prompts found')).toBeInTheDocument()
      expect(
        screen.getByText('Try adjusting your search terms')
      ).toBeInTheDocument()
    })
  })

  describe('results', () => {
    it('renders result cards with name, slug, date, status, and truncated body', () => {
      searchState.prompts = [
        buildPrompt({ body: 'x'.repeat(200) }),
        buildPrompt({
          id: 'prompt-2',
          name: 'Bug Triage',
          slug: 'bug-triage',
          status: 'draft',
          body: 'Short body',
          created_at: '2026-02-03T00:00:00Z',
        }),
      ]

      renderLoader()

      expect(screen.getByText('Code Review Template')).toBeInTheDocument()
      expect(screen.getByText('code-review-template')).toBeInTheDocument()
      expect(screen.getByText('Jan 15, 2026')).toBeInTheDocument()
      expect(screen.getByText('Published')).toBeInTheDocument()
      // Long body truncated at 150 chars with an ellipsis.
      expect(screen.getByText(`${'x'.repeat(150)}...`)).toBeInTheDocument()

      expect(screen.getByText('Bug Triage')).toBeInTheDocument()
      expect(screen.getByText('Draft')).toBeInTheDocument()
      expect(screen.getByText('Feb 3, 2026')).toBeInTheDocument()
      // Short body untouched.
      expect(screen.getByText('Short body')).toBeInTheDocument()

      expect(screen.getByText('Found 2 prompts')).toBeInTheDocument()
    })

    it('uses the singular noun for a single result', () => {
      searchState.prompts = [buildPrompt()]

      renderLoader()

      expect(screen.getByText('Found 1 prompt')).toBeInTheDocument()
    })
  })

  describe('selecting a prompt', () => {
    it('selects via the card and closes, clearing the search state', async () => {
      searchState.prompts = [buildPrompt()]

      renderLoader()

      const user = userEvent.setup()
      await user.click(
        screen.getByRole('button', {
          name: 'Load prompt: Code Review Template',
        })
      )

      expect(onSelectPrompt).toHaveBeenCalledWith(searchState.prompts[0])
      expect(onClose).toHaveBeenCalled()
      expect(searchState.clearResults).toHaveBeenCalled()
    })

    it('selects via the Load Template button', async () => {
      searchState.prompts = [buildPrompt()]

      renderLoader()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Load Template' }))

      expect(onSelectPrompt).toHaveBeenCalledTimes(1)
      expect(onSelectPrompt).toHaveBeenCalledWith(searchState.prompts[0])
      expect(onClose).toHaveBeenCalled()
    })

    it('selects via Enter on the focused card', async () => {
      searchState.prompts = [buildPrompt()]

      renderLoader()

      screen
        .getByRole('button', { name: 'Load prompt: Code Review Template' })
        .focus()
      const user = userEvent.setup()
      await user.keyboard('{Enter}')

      expect(onSelectPrompt).toHaveBeenCalledWith(searchState.prompts[0])
    })

    it('ignores other keys on the focused card', async () => {
      searchState.prompts = [buildPrompt()]

      renderLoader()

      screen
        .getByRole('button', { name: 'Load prompt: Code Review Template' })
        .focus()
      const user = userEvent.setup()
      await user.keyboard('{ArrowDown}')

      expect(onSelectPrompt).not.toHaveBeenCalled()
      expect(onClose).not.toHaveBeenCalled()
    })

    it('selects via Space on the focused card', async () => {
      searchState.prompts = [buildPrompt()]

      renderLoader()

      screen
        .getByRole('button', { name: 'Load prompt: Code Review Template' })
        .focus()
      const user = userEvent.setup()
      await user.keyboard(' ')

      expect(onSelectPrompt).toHaveBeenCalledWith(searchState.prompts[0])
    })
  })

  describe('closing without selecting', () => {
    it('closes from the Cancel button', async () => {
      renderLoader()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Cancel' }))

      expect(onClose).toHaveBeenCalled()
      expect(onSelectPrompt).not.toHaveBeenCalled()
    })

    it('closes from the backdrop', async () => {
      renderLoader()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Close modal' }))

      expect(onClose).toHaveBeenCalled()
    })

    it('closes from the header X button and resets the query', async () => {
      renderLoader()

      const user = userEvent.setup()
      const input = screen.getByPlaceholderText(
        'Search prompts by name or content...'
      )
      await user.type(input, 'review')

      // The header close button carries the X icon.
      const xIcon = screen.getByTestId('x-icon')
      await user.click(xIcon.closest('button') as HTMLElement)

      expect(onClose).toHaveBeenCalled()
      expect(searchState.clearResults).toHaveBeenCalled()
    })
  })
})
