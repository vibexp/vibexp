import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { PromptGalleryCategory } from '@/services/promptGalleryService'

jest.mock('@/services/promptGalleryService', () => ({
  promptGalleryService: {
    getCategories: jest.fn(),
    getPrompts: jest.fn(),
    getPromptById: jest.fn(),
    trackPromptUsage: jest.fn(),
  },
}))

const mockShowAlert = jest.fn()
jest.mock('@/contexts/AlertContext', () => ({
  useAlertContext: () => ({ showAlert: mockShowAlert }),
}))

import { promptGalleryService } from '@/services/promptGalleryService'

import { PromptGallery } from '../PromptGallery'

function buildCategory(
  overrides: Partial<PromptGalleryCategory> = {}
): PromptGalleryCategory {
  return {
    category: 'Engineering',
    count: 3,
    ...overrides,
  }
}

function renderGallery() {
  return render(
    <MemoryRouter initialEntries={['/prompt-gallery']}>
      <Routes>
        <Route path="/prompt-gallery" element={<PromptGallery />} />
        <Route
          path="/prompt-gallery/:category"
          element={<div data-testid="category-probe">Category probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('PromptGallery page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    ;(promptGalleryService.getCategories as jest.Mock).mockResolvedValue([])
  })

  it('shows a loading spinner while categories are in flight', () => {
    ;(promptGalleryService.getCategories as jest.Mock).mockImplementation(
      () => new Promise(() => undefined)
    )

    renderGallery()

    expect(screen.getByText('Prompt Gallery')).toBeInTheDocument()
    expect(screen.getByTestId('loader2-icon')).toBeInTheDocument()
  })

  it('shows the empty state when no categories exist', async () => {
    renderGallery()

    await waitFor(() => {
      expect(screen.getByTestId('empty-state')).toBeInTheDocument()
    })
    expect(screen.getByText('No categories available')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Prompt categories will appear here once they are added.'
      )
    ).toBeInTheDocument()
  })

  it('renders one card per category with a pluralized prompt count', async () => {
    ;(promptGalleryService.getCategories as jest.Mock).mockResolvedValue([
      buildCategory(),
      buildCategory({ category: 'Writing', count: 1 }),
    ])

    renderGallery()

    await waitFor(() => {
      expect(screen.getAllByTestId('gallery-category-card')).toHaveLength(2)
    })
    expect(screen.getByText('Engineering')).toBeInTheDocument()
    expect(screen.getByText('Writing')).toBeInTheDocument()
    expect(screen.getByText(/3\s*prompts/)).toBeInTheDocument()
    expect(screen.getByText(/1\s*prompt$/)).toBeInTheDocument()
  })

  it('navigates to the encoded category route when a card is clicked', async () => {
    ;(promptGalleryService.getCategories as jest.Mock).mockResolvedValue([
      buildCategory({ category: 'Code Review' }),
    ])

    renderGallery()

    const user = userEvent.setup()
    await user.click(await screen.findByTestId('gallery-category-card'))

    expect(screen.getByTestId('category-probe')).toBeInTheDocument()
  })

  it('navigates when a focused card is activated with the keyboard', async () => {
    ;(promptGalleryService.getCategories as jest.Mock).mockResolvedValue([
      buildCategory(),
    ])

    renderGallery()

    const card = await screen.findByTestId('gallery-category-card')
    card.focus()
    const user = userEvent.setup()
    await user.keyboard('{Enter}')

    expect(screen.getByTestId('category-probe')).toBeInTheDocument()
  })

  it('surfaces a fetch failure through the alert context', async () => {
    ;(promptGalleryService.getCategories as jest.Mock).mockRejectedValue(
      new Error('gallery unavailable')
    )

    renderGallery()

    await waitFor(() => {
      expect(mockShowAlert).toHaveBeenCalledWith({
        type: 'error',
        message: 'gallery unavailable',
      })
    })
    // After the failure the page settles on the empty state.
    expect(screen.getByTestId('empty-state')).toBeInTheDocument()
  })
})
