import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { AdminSearchPicker } from '../AdminSearchPicker'
import type { AdminPagedSearchResult } from '../useAdminPagedSearch'

interface Person {
  id: string
  name: string
  email: string
}

const ADA: Person = { id: 'u-1', name: 'Ada', email: 'ada@example.com' }
const GRACE: Person = { id: 'u-2', name: 'Grace', email: 'grace@example.com' }

beforeAll(() => {
  // Radix Popover/Command rely on layout APIs jsdom does not implement.
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

function search(
  overrides: Partial<AdminPagedSearchResult<Person>> = {}
): AdminPagedSearchResult<Person> {
  return {
    items: [ADA, GRACE],
    loading: false,
    loadingMore: false,
    error: null,
    hasMore: false,
    loadMore: vi.fn(),
    setQuery: vi.fn(),
    query: '',
    ...overrides,
  }
}

function renderPicker(
  props: Partial<Parameters<typeof AdminSearchPicker<Person>>[0]> = {}
) {
  const onChange = vi.fn()
  render(
    <AdminSearchPicker<Person>
      value=""
      onChange={onChange}
      search={search()}
      getId={p => p.id}
      getLabel={p => p.name}
      getDetail={p => p.email}
      allLabel="All users"
      selectedFallback="Selected user"
      searchPlaceholder="Search users…"
      emptyText="No users found."
      aria-label="Filter by user"
      {...props}
    />
  )
  return { onChange }
}

const trigger = () => screen.getByRole('combobox', { name: 'Filter by user' })

it('reads "All" with nothing selected and picks an item by id', async () => {
  const { onChange } = renderPicker()
  expect(trigger()).toHaveTextContent('All users')

  await userEvent.click(trigger())
  expect(screen.getByText('grace@example.com')).toBeInTheDocument()
  await userEvent.click(screen.getByText('Grace'))

  expect(onChange).toHaveBeenCalledWith('u-2')
})

it('clears the selection through the "All" item', async () => {
  const { onChange } = renderPicker({ value: 'u-1' })
  expect(trigger()).toHaveTextContent('Ada')

  await userEvent.click(trigger())
  await userEvent.click(screen.getByRole('option', { name: 'All users' }))

  expect(onChange).toHaveBeenCalledWith('')
})

it('resolves the label of a selected id no loaded page contains', async () => {
  const resolveLabel = vi.fn().mockResolvedValue('Linus')
  renderPicker({ value: 'u-9', resolveLabel })

  await waitFor(() => {
    expect(trigger()).toHaveTextContent('Linus')
  })
  expect(resolveLabel).toHaveBeenCalledWith('u-9')
})

it('keeps the fallback when the id cannot be resolved, or there is no resolver', async () => {
  const resolveLabel = vi.fn().mockRejectedValue(new Error('gone'))
  renderPicker({ value: 'u-9', resolveLabel })
  await waitFor(() => {
    expect(resolveLabel).toHaveBeenCalled()
  })
  expect(trigger()).toHaveTextContent('Selected user')
})

it('shows the loading and error states of the search', async () => {
  renderPicker({ search: search({ loading: true, items: [] }) })
  await userEvent.click(trigger())
  expect(screen.getByText('Searching…')).toBeInTheDocument()
})

it('shows a failed search', async () => {
  renderPicker({ search: search({ error: 'Failed to load users', items: [] }) })
  await userEvent.click(trigger())
  expect(screen.getByText('Failed to load users')).toBeInTheDocument()
})

it('pulls the next page when the list is scrolled to the bottom', async () => {
  const loadMore = vi.fn()
  renderPicker({ search: search({ hasMore: true, loadMore }) })
  await userEvent.click(trigger())
  expect(screen.getByText('Scroll for more')).toBeInTheDocument()

  const list = screen.getByRole('listbox')
  Object.defineProperty(list, 'scrollHeight', { value: 500 })
  Object.defineProperty(list, 'clientHeight', { value: 200 })
  Object.defineProperty(list, 'scrollTop', { value: 290 })
  fireEvent.scroll(list)

  expect(loadMore).toHaveBeenCalled()
})

it('passes typed text to the search', async () => {
  const setQuery = vi.fn()
  renderPicker({ search: search({ setQuery }) })
  await userEvent.click(trigger())
  await userEvent.type(screen.getByPlaceholderText('Search users…'), 'g')
  expect(setQuery).toHaveBeenCalledWith('g')
})
