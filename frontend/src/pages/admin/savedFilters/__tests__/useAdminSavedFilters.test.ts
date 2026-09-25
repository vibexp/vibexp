/**
 * useAdminSavedFilters (#1148): load, each mutation's PUT payload, and the
 * 409 refetch-and-retry-once path.
 */
import { act, renderHook, waitFor } from '@testing-library/react'
import type { Mocked } from 'vitest'

vi.mock('@/services/adminService', () => ({
  adminService: { getSavedFilters: vi.fn(), replaceSavedFilters: vi.fn() },
}))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { toast } from 'sonner'

import type { AdminSavedFilters } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { ApiError } from '@/types/errors'

import { CONFLICT_MESSAGE, useAdminSavedFilters } from '../useAdminSavedFilters'

const mockAdminService = adminService as Mocked<typeof adminService>

const apiError = (status: number, detail = 'nope') =>
  new ApiError({
    type: 'about:blank',
    title: 'Error',
    status,
    detail,
    code: 'TEST',
    request_id: 'req-1',
    timestamp: '2026-01-01T00:00:00Z',
  })

const DORMANT = { id: 'p1', name: 'Dormant', query: { kind: 'team' } }
const POWER = { id: 'p2', name: 'Power', query: { sort_by: 'resource_count' } }

function saved(
  presets: AdminSavedFilters['presets'],
  version: number
): AdminSavedFilters {
  return { list: 'teams', presets, version }
}

async function renderLoaded(initial = saved([DORMANT], 3)) {
  mockAdminService.getSavedFilters.mockResolvedValueOnce(initial)
  const hook = renderHook(() => useAdminSavedFilters('teams'))
  await waitFor(() => {
    expect(hook.result.current.status).toBe('ready')
  })
  return hook
}

const lastPut = () => {
  const { calls } = mockAdminService.replaceSavedFilters.mock
  return calls[calls.length - 1]
}

beforeEach(() => {
  vi.clearAllMocks()
})

it('loads the presets and version for its list', async () => {
  const { result } = await renderLoaded()
  expect(mockAdminService.getSavedFilters).toHaveBeenCalledWith('teams')
  expect(result.current.presets).toEqual([DORMANT])
  expect(result.current.version).toBe(3)
})

it('reports a load failure and recovers on reload', async () => {
  mockAdminService.getSavedFilters.mockRejectedValueOnce(new Error('down'))
  const { result } = renderHook(() => useAdminSavedFilters('teams'))
  await waitFor(() => {
    expect(result.current.status).toBe('error')
  })

  mockAdminService.getSavedFilters.mockResolvedValueOnce(saved([DORMANT], 1))
  await act(() => result.current.reload())
  expect(result.current.status).toBe('ready')
  expect(result.current.presets).toEqual([DORMANT])
})

it('saves a new preset without an id, sending the held version', async () => {
  const { result } = await renderLoaded()
  const created = { id: 'p9', name: 'Mine', query: { status: 'active' } }
  mockAdminService.replaceSavedFilters.mockResolvedValueOnce(
    saved([DORMANT, created], 4)
  )

  let ok = false
  await act(async () => {
    ok = await result.current.save('  Mine ', { status: 'active' })
  })

  expect(ok).toBe(true)
  expect(lastPut()).toEqual([
    'teams',
    {
      presets: [DORMANT, { name: 'Mine', query: { status: 'active' } }],
      version: 3,
    },
  ])
  expect(result.current.presets).toEqual([DORMANT, created])
  expect(result.current.version).toBe(4)
})

it('renames a preset, keeping its id and query', async () => {
  const { result } = await renderLoaded(saved([DORMANT, POWER], 3))
  mockAdminService.replaceSavedFilters.mockResolvedValueOnce(
    saved([{ ...DORMANT, name: 'Quiet' }, POWER], 4)
  )

  await act(async () => {
    await result.current.rename('p1', 'Quiet')
  })

  expect(lastPut()[1]).toEqual({
    presets: [{ ...DORMANT, name: 'Quiet' }, POWER],
    version: 3,
  })
})

it('deletes only the named preset', async () => {
  const { result } = await renderLoaded(saved([DORMANT, POWER], 3))
  mockAdminService.replaceSavedFilters.mockResolvedValueOnce(saved([POWER], 4))

  await act(async () => {
    await result.current.remove('p1')
  })

  expect(lastPut()[1]).toEqual({ presets: [POWER], version: 3 })
  expect(result.current.presets).toEqual([POWER])
})

it('on a 409, refetches and retries once against the fresh state', async () => {
  const { result } = await renderLoaded(saved([DORMANT], 3))
  // Another tab added POWER and moved the version on.
  mockAdminService.replaceSavedFilters.mockRejectedValueOnce(apiError(409))
  mockAdminService.getSavedFilters.mockResolvedValueOnce(
    saved([DORMANT, POWER], 7)
  )
  const created = { id: 'p9', name: 'Mine', query: { kind: 'personal' } }
  mockAdminService.replaceSavedFilters.mockResolvedValueOnce(
    saved([DORMANT, POWER, created], 8)
  )

  let ok = false
  await act(async () => {
    ok = await result.current.save('Mine', { kind: 'personal' })
  })

  expect(ok).toBe(true)
  expect(mockAdminService.getSavedFilters).toHaveBeenCalledTimes(2)
  expect(mockAdminService.replaceSavedFilters).toHaveBeenCalledTimes(2)
  expect(lastPut()[1]).toEqual({
    presets: [DORMANT, POWER, { name: 'Mine', query: { kind: 'personal' } }],
    version: 7,
  })
  expect(result.current.presets).toEqual([DORMANT, POWER, created])
  expect(toast.error).not.toHaveBeenCalled()
})

it('on a second 409, toasts and keeps the refetched presets', async () => {
  const { result } = await renderLoaded(saved([DORMANT], 3))
  mockAdminService.replaceSavedFilters.mockRejectedValue(apiError(409))
  mockAdminService.getSavedFilters.mockResolvedValueOnce(
    saved([DORMANT, POWER], 7)
  )

  let ok = true
  await act(async () => {
    ok = await result.current.remove('p1')
  })

  expect(ok).toBe(false)
  expect(mockAdminService.getSavedFilters).toHaveBeenCalledTimes(2)
  expect(mockAdminService.replaceSavedFilters).toHaveBeenCalledTimes(2)
  expect(toast.error).toHaveBeenCalledWith(CONFLICT_MESSAGE)
  expect(result.current.presets).toEqual([DORMANT, POWER])
  expect(result.current.version).toBe(7)
})

it('on any other error, toasts the server detail without retrying', async () => {
  const { result } = await renderLoaded()
  mockAdminService.replaceSavedFilters.mockRejectedValueOnce(
    apiError(400, 'preset name already used')
  )

  let ok = true
  await act(async () => {
    ok = await result.current.save('Dormant', { kind: 'team' })
  })

  expect(ok).toBe(false)
  expect(mockAdminService.replaceSavedFilters).toHaveBeenCalledTimes(1)
  expect(mockAdminService.getSavedFilters).toHaveBeenCalledTimes(1)
  expect(toast.error).toHaveBeenCalledWith('preset name already used')
  expect(result.current.presets).toEqual([DORMANT])
})

it('ignores a second write while one is in flight', async () => {
  const { result } = await renderLoaded()
  let finish: (value: AdminSavedFilters) => void = () => undefined
  mockAdminService.replaceSavedFilters.mockReturnValueOnce(
    new Promise(resolve => {
      finish = resolve
    })
  )

  let first: Promise<boolean> = Promise.resolve(false)
  let second = true
  await act(async () => {
    first = result.current.save('One', { kind: 'team' })
    second = await result.current.save('Two', { kind: 'team' })
  })
  expect(second).toBe(false)
  expect(result.current.saving).toBe(true)

  await act(async () => {
    finish(saved([DORMANT], 4))
    await first
  })
  expect(mockAdminService.replaceSavedFilters).toHaveBeenCalledTimes(1)
  expect(result.current.saving).toBe(false)
})
