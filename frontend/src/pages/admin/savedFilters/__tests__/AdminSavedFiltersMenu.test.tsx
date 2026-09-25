/**
 * AdminSavedFiltersMenu (#1148): apply, save, rename, delete, the cap message
 * and the load-error state, over the real hook with a mocked service.
 */
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Mocked } from 'vitest'

vi.mock('@/services/adminService', () => ({
  adminService: { getSavedFilters: vi.fn(), replaceSavedFilters: vi.fn() },
}))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import type {
  AdminSavedFilterPreset,
  AdminSavedFilters,
} from '@/services/adminService'
import { adminService } from '@/services/adminService'

import type { AdminSavedFiltersMenuProps } from '../AdminSavedFiltersMenu'
import { AdminSavedFiltersMenu, CAP_MESSAGE } from '../AdminSavedFiltersMenu'

const mockAdminService = adminService as Mocked<typeof adminService>

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

const DORMANT = { id: 'p1', name: 'Dormant', query: { kind: 'team' } }
const POWER = { id: 'p2', name: 'Power', query: { sort_by: 'resource_count' } }

const saved = (
  presets: AdminSavedFilterPreset[],
  version = 1
): AdminSavedFilters => ({ list: 'users', presets, version })

function renderMenu(props: Partial<AdminSavedFiltersMenuProps> = {}) {
  const onApply = vi.fn()
  render(
    <AdminSavedFiltersMenu
      list="users"
      currentQuery={{ status: 'suspended' }}
      onApply={onApply}
      canSave
      {...props}
    />
  )
  return { onApply }
}

async function openMenu() {
  await userEvent.click(screen.getByRole('button', { name: /Presets/ }))
  return screen.findByRole('menu')
}

beforeEach(() => {
  vi.clearAllMocks()
  mockAdminService.getSavedFilters.mockResolvedValue(saved([DORMANT, POWER]))
})

it('loads the presets for its list and shows the count', async () => {
  renderMenu()
  expect(await screen.findByTestId('saved-filters-count')).toHaveTextContent(
    '2'
  )
  expect(mockAdminService.getSavedFilters).toHaveBeenCalledWith('users')
})

it('applies a preset and marks the one matching the current filters', async () => {
  const { onApply } = renderMenu({ currentQuery: { kind: 'team' } })
  await screen.findByTestId('saved-filters-count')
  const menu = await openMenu()

  expect(
    within(menu).getByRole('menuitem', { name: 'Dormant' })
  ).toHaveAttribute('aria-current', 'true')
  const power = within(menu).getByRole('menuitem', { name: 'Power' })
  expect(power).not.toHaveAttribute('aria-current')

  await userEvent.click(power)
  expect(onApply).toHaveBeenCalledWith({ sort_by: 'resource_count' })
})

it('shows an empty state with nothing to manage', async () => {
  mockAdminService.getSavedFilters.mockResolvedValue(saved([]))
  renderMenu()
  const menu = await openMenu()
  expect(await within(menu).findByText('No saved presets yet')).toBeVisible()
  expect(
    within(menu).getByRole('menuitem', { name: 'Manage presets…' })
  ).toHaveAttribute('data-disabled')
})

it('saves the current filters under a validated name', async () => {
  mockAdminService.replaceSavedFilters.mockResolvedValue(
    saved([DORMANT, POWER, { id: 'p3', name: 'Suspended', query: {} }], 2)
  )
  renderMenu()
  await screen.findByTestId('saved-filters-count')
  const menu = await openMenu()
  await userEvent.click(
    within(menu).getByRole('menuitem', { name: 'Save current filters…' })
  )

  const dialog = await screen.findByRole('dialog', {
    name: 'Save current filters',
  })
  const input = within(dialog).getByLabelText('Preset name')

  // Client-side rules run before any request.
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Save preset' })
  )
  expect(within(dialog).getByText('Name is required')).toBeVisible()
  await userEvent.type(input, 'dormant')
  expect(
    within(dialog).getByText('A preset with this name already exists')
  ).toBeVisible()
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Save preset' })
  )
  expect(mockAdminService.replaceSavedFilters).not.toHaveBeenCalled()

  await userEvent.clear(input)
  await userEvent.type(input, 'Suspended{Enter}')

  await waitFor(() => {
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  expect(mockAdminService.replaceSavedFilters).toHaveBeenCalledWith('users', {
    presets: [
      DORMANT,
      POWER,
      { name: 'Suspended', query: { status: 'suspended' } },
    ],
    version: 1,
  })
  expect(screen.getByTestId('saved-filters-count')).toHaveTextContent('3')
})

it('disables saving without filters', async () => {
  renderMenu({ canSave: false, currentQuery: {} })
  await screen.findByTestId('saved-filters-count')
  const menu = await openMenu()
  expect(
    within(menu).getByRole('menuitem', { name: 'Save current filters…' })
  ).toHaveAttribute('data-disabled')
  expect(within(menu).getByText('Apply some filters first')).toBeVisible()
})

it('disables saving at the 20-preset cap with a visible reason', async () => {
  mockAdminService.getSavedFilters.mockResolvedValue(
    saved(
      Array.from({ length: 20 }, (_, i) => ({
        id: `p${String(i)}`,
        name: `Preset ${String(i)}`,
        query: { status: 'active' },
      }))
    )
  )
  renderMenu()
  await screen.findByTestId('saved-filters-count')
  const menu = await openMenu()
  expect(
    within(menu).getByRole('menuitem', { name: 'Save current filters…' })
  ).toHaveAttribute('data-disabled')
  expect(within(menu).getByText(CAP_MESSAGE)).toBeVisible()
  expect(CAP_MESSAGE).toBe('20 of 20 presets — delete one to save another')
})

it('shows a load error with a working Retry', async () => {
  mockAdminService.getSavedFilters.mockRejectedValueOnce(new Error('down'))
  renderMenu()
  const menu = await openMenu()
  expect(await within(menu).findByText("Couldn't load presets")).toBeVisible()

  await userEvent.click(within(menu).getByRole('menuitem', { name: 'Retry' }))
  await waitFor(() => {
    expect(screen.getByTestId('saved-filters-count')).toHaveTextContent('2')
  })
  expect(mockAdminService.getSavedFilters).toHaveBeenCalledTimes(2)
})

async function openManage() {
  await screen.findByTestId('saved-filters-count')
  const menu = await openMenu()
  await userEvent.click(
    within(menu).getByRole('menuitem', { name: 'Manage presets…' })
  )
  return screen.findByRole('dialog', { name: 'Manage presets' })
}

it('renames a preset in the manage dialog', async () => {
  mockAdminService.replaceSavedFilters.mockResolvedValue(
    saved([{ ...DORMANT, name: 'Quiet' }, POWER], 2)
  )
  renderMenu()
  const dialog = await openManage()

  const input = within(dialog).getByLabelText('Name for preset Dormant')
  const [renameDormant] = within(dialog).getAllByRole('button', {
    name: 'Rename',
  })
  expect(renameDormant).toBeDisabled()

  await userEvent.clear(input)
  await userEvent.type(input, 'power')
  expect(
    within(dialog).getByText('A preset with this name already exists')
  ).toBeVisible()
  expect(renameDormant).toBeDisabled()

  await userEvent.clear(input)
  await userEvent.type(input, 'Quiet')
  await userEvent.click(renameDormant)

  await waitFor(() => {
    expect(mockAdminService.replaceSavedFilters).toHaveBeenCalledWith('users', {
      presets: [{ ...DORMANT, name: 'Quiet' }, POWER],
      version: 1,
    })
  })
  expect(
    await within(dialog).findByLabelText('Name for preset Quiet')
  ).toHaveValue('Quiet')
})

it('deletes a preset only after confirming', async () => {
  mockAdminService.replaceSavedFilters.mockResolvedValue(saved([POWER], 2))
  renderMenu()
  const dialog = await openManage()

  const [deleteDormant] = within(dialog).getAllByRole('button', {
    name: 'Delete',
  })
  await userEvent.click(deleteDormant)
  expect(mockAdminService.replaceSavedFilters).not.toHaveBeenCalled()

  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Confirm delete' })
  )
  await waitFor(() => {
    expect(mockAdminService.replaceSavedFilters).toHaveBeenCalledWith('users', {
      presets: [POWER],
      version: 1,
    })
  })
  await waitFor(() => {
    expect(
      within(dialog).queryByLabelText('Name for preset Dormant')
    ).not.toBeInTheDocument()
  })
  expect(within(dialog).getByLabelText('Name for preset Power')).toBeVisible()
})
