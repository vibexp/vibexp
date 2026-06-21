import '@testing-library/jest-dom'

import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ListPage } from './ListPage'

describe('<ListPage.Body>', () => {
  it('renders 6 skeleton rows by default when status is loading', () => {
    render(
      <ListPage.Body status="loading">
        <div data-testid="children">should not render</div>
      </ListPage.Body>
    )

    expect(screen.getAllByTestId('list-page-skeleton-row')).toHaveLength(6)
    expect(screen.queryByTestId('children')).not.toBeInTheDocument()
  })

  it('honors a custom loadingRows count', () => {
    render(
      <ListPage.Body status="loading" loadingRows={3}>
        <div />
      </ListPage.Body>
    )

    expect(screen.getAllByTestId('list-page-skeleton-row')).toHaveLength(3)
  })

  it('renders the error alert with default title and provided message', () => {
    render(
      <ListPage.Body status="error" errorMessage="Network unavailable">
        <div data-testid="children">should not render</div>
      </ListPage.Body>
    )

    expect(screen.getByText('Failed to load')).toBeInTheDocument()
    expect(screen.getByText('Network unavailable')).toBeInTheDocument()
    expect(screen.queryByTestId('children')).not.toBeInTheDocument()
  })

  it('renders a custom error title when provided', () => {
    render(
      <ListPage.Body
        status="error"
        errorTitle="Failed to load prompts"
        errorMessage="boom"
      >
        <div />
      </ListPage.Body>
    )

    expect(screen.getByText('Failed to load prompts')).toBeInTheDocument()
  })

  it('renders the empty slot when status is empty', () => {
    render(
      <ListPage.Body
        status="empty"
        empty={<div data-testid="empty-state">No items</div>}
      >
        <div data-testid="children">should not render</div>
      </ListPage.Body>
    )

    expect(screen.getByTestId('empty-state')).toBeInTheDocument()
    expect(screen.getByText('No items')).toBeInTheDocument()
    expect(screen.queryByTestId('children')).not.toBeInTheDocument()
  })

  it('renders children when status is ready', () => {
    render(
      <ListPage.Body status="ready">
        <div data-testid="children">table content</div>
      </ListPage.Body>
    )

    expect(screen.getByTestId('children')).toBeInTheDocument()
    expect(screen.getByText('table content')).toBeInTheDocument()
  })
})

describe('<ListPage.Footer>', () => {
  describe('count display', () => {
    it('renders "Showing X of Y <noun>" when count is provided', () => {
      render(
        <ListPage.Footer
          count={{ visible: 10, total: 47, noun: 'prompt' }}
          pagination={{ page: 1, totalPages: 3, onPageChange: jest.fn() }}
        />
      )

      expect(screen.getByText(/Showing 10 of 47 prompts/)).toBeInTheDocument()
    })

    it('uses singular noun when total is 1', () => {
      render(
        <ListPage.Footer count={{ visible: 1, total: 1, noun: 'prompt' }} />
      )

      expect(screen.getByText(/Showing 1 of 1 prompt/)).toBeInTheDocument()
      expect(screen.queryByText(/prompts/)).not.toBeInTheDocument()
    })

    it('uses nounPlural when provided for irregular plurals', () => {
      render(
        <ListPage.Footer
          count={{
            visible: 5,
            total: 12,
            noun: 'memory',
            nounPlural: 'memories',
          }}
        />
      )

      expect(screen.getByText(/Showing 5 of 12 memories/)).toBeInTheDocument()
    })

    it('falls back to noun + "s" when nounPlural is omitted', () => {
      render(
        <ListPage.Footer count={{ visible: 5, total: 10, noun: 'prompt' }} />
      )

      expect(screen.getByText(/Showing 5 of 10 prompts/)).toBeInTheDocument()
    })

    it('hides count line when hideCount is true', () => {
      render(
        <ListPage.Footer
          count={{ visible: 10, total: 47, noun: 'prompt' }}
          hideCount
        />
      )

      expect(screen.queryByText(/Showing/)).not.toBeInTheDocument()
    })

    it('renders the optional note line beneath the count', () => {
      render(
        <ListPage.Footer
          count={{ visible: 10, total: 47, noun: 'prompt' }}
          note="Some additional context"
        />
      )

      expect(screen.getByText('Some additional context')).toBeInTheDocument()
    })
  })

  describe('pagination', () => {
    it('hides Prev/Next buttons when totalPages <= 1', () => {
      render(
        <ListPage.Footer
          count={{ visible: 5, total: 5, noun: 'prompt' }}
          pagination={{ page: 1, totalPages: 1, onPageChange: jest.fn() }}
        />
      )

      expect(
        screen.queryByRole('button', { name: 'Previous' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Next' })
      ).not.toBeInTheDocument()
    })

    it('hides Prev/Next buttons when pagination is undefined', () => {
      render(
        <ListPage.Footer count={{ visible: 5, total: 5, noun: 'prompt' }} />
      )

      expect(
        screen.queryByRole('button', { name: 'Previous' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Next' })
      ).not.toBeInTheDocument()
    })

    it('disables Prev on first page, enables Next', () => {
      render(
        <ListPage.Footer
          count={{ visible: 10, total: 30, noun: 'prompt' }}
          pagination={{ page: 1, totalPages: 3, onPageChange: jest.fn() }}
        />
      )

      expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
      expect(screen.getByRole('button', { name: 'Next' })).not.toBeDisabled()
    })

    it('disables Next on last page, enables Prev', () => {
      render(
        <ListPage.Footer
          count={{ visible: 7, total: 30, noun: 'prompt' }}
          pagination={{ page: 3, totalPages: 3, onPageChange: jest.fn() }}
        />
      )

      expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
      expect(
        screen.getByRole('button', { name: 'Previous' })
      ).not.toBeDisabled()
    })

    it('calls onPageChange with page-1 when Prev clicked', async () => {
      const user = userEvent.setup()
      const onPageChange = jest.fn()

      render(
        <ListPage.Footer
          count={{ visible: 10, total: 30, noun: 'prompt' }}
          pagination={{ page: 2, totalPages: 3, onPageChange }}
        />
      )

      await user.click(screen.getByRole('button', { name: 'Previous' }))
      expect(onPageChange).toHaveBeenCalledWith(1)
    })

    it('calls onPageChange with page+1 when Next clicked', async () => {
      const user = userEvent.setup()
      const onPageChange = jest.fn()

      render(
        <ListPage.Footer
          count={{ visible: 10, total: 30, noun: 'prompt' }}
          pagination={{ page: 2, totalPages: 3, onPageChange }}
        />
      )

      await user.click(screen.getByRole('button', { name: 'Next' }))
      expect(onPageChange).toHaveBeenCalledWith(3)
    })
  })
})

describe('<ListPage> compound', () => {
  it('renders the header with title, description, and actions', () => {
    render(
      <ListPage>
        <ListPage.Header
          title="My List"
          description="Manage your stuff."
          actions={<button>Create</button>}
        />
      </ListPage>
    )

    expect(screen.getByRole('heading', { name: 'My List' })).toBeInTheDocument()
    expect(screen.getByText('Manage your stuff.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create' })).toBeInTheDocument()
  })

  it('exposes Header, Container, Filters, Body, and Footer as subcomponents', () => {
    expect(ListPage.Header).toBeDefined()
    expect(ListPage.Container).toBeDefined()
    expect(ListPage.Filters).toBeDefined()
    expect(ListPage.Body).toBeDefined()
    expect(ListPage.Footer).toBeDefined()
  })
})
