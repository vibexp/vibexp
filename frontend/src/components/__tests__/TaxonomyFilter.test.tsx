import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { TaxonomyFilter } from '@/components/TaxonomyFilter'

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

const onChange = vi.fn()
const onOpen = vi.fn()

function renderFilter(
  props: Partial<Parameters<typeof TaxonomyFilter>[0]> = {}
) {
  return render(
    <TaxonomyFilter
      value={[]}
      onChange={onChange}
      options={['api', 'review']}
      onOpen={onOpen}
      label="Filter by labels"
      {...props}
    />
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('TaxonomyFilter', () => {
  it('reads as unfiltered until something is picked', () => {
    renderFilter()
    expect(screen.getByLabelText('Filter by labels')).toHaveTextContent(
      'All labels'
    )
  })

  it('names the single selection rather than counting it', () => {
    renderFilter({ value: ['api'] })
    expect(screen.getByLabelText('Filter by labels')).toHaveTextContent('api')
  })

  it('counts once naming them all would not fit', () => {
    renderFilter({ value: ['api', 'review'] })
    expect(screen.getByLabelText('Filter by labels')).toHaveTextContent(
      '2 labels'
    )
  })

  it('asks the host to load the catalog when it opens', async () => {
    const user = userEvent.setup()
    renderFilter()
    expect(onOpen).not.toHaveBeenCalled()
    await user.click(screen.getByLabelText('Filter by labels'))
    expect(onOpen).toHaveBeenCalledTimes(1)
  })

  it('adds a value on pick and removes it on a second pick', async () => {
    const user = userEvent.setup()
    const { rerender } = renderFilter()
    await user.click(screen.getByLabelText('Filter by labels'))
    await user.click(await screen.findByRole('option', { name: /api/ }))
    expect(onChange).toHaveBeenCalledWith(['api'])

    rerender(
      <TaxonomyFilter
        value={['api']}
        onChange={onChange}
        options={['api', 'review']}
        label="Filter by labels"
      />
    )
    await user.click(await screen.findByRole('option', { name: /api/ }))
    expect(onChange).toHaveBeenLastCalledWith([])
  })

  it('stays open across picks so several can be chosen', async () => {
    const user = userEvent.setup()
    renderFilter()
    await user.click(screen.getByLabelText('Filter by labels'))
    await user.click(await screen.findByRole('option', { name: /api/ }))
    expect(screen.getByRole('option', { name: /review/ })).toBeInTheDocument()
  })

  it('clears everything from inside the popover', async () => {
    const user = userEvent.setup()
    renderFilter({ value: ['api'] })
    await user.click(screen.getByLabelText('Filter by labels'))
    await user.click(
      await screen.findByRole('button', { name: 'Clear labels filter' })
    )
    expect(onChange).toHaveBeenCalledWith([])
  })

  it('offers no Clear while nothing is selected', async () => {
    const user = userEvent.setup()
    renderFilter()
    await user.click(screen.getByLabelText('Filter by labels'))
    expect(
      screen.queryByRole('button', { name: 'Clear labels filter' })
    ).not.toBeInTheDocument()
  })

  it('shows the catalog error instead of an empty list', async () => {
    const user = userEvent.setup()
    renderFilter({ options: [], error: 'Failed to load labels' })
    await user.click(screen.getByLabelText('Filter by labels'))
    expect(await screen.findByText('Failed to load labels')).toBeInTheDocument()
    expect(screen.queryByText('No labels found.')).not.toBeInTheDocument()
  })

  it('shows neither options nor an empty state while loading', async () => {
    const user = userEvent.setup()
    renderFilter({ loading: true })
    await user.click(screen.getByLabelText('Filter by labels'))
    expect(screen.queryByRole('option')).not.toBeInTheDocument()
    expect(screen.queryByText('No labels found.')).not.toBeInTheDocument()
  })
})
