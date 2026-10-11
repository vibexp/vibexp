import { act, renderHook } from '@testing-library/react'

import { lockedWriteError, TEST_LOCK } from '@/lib/testing/resourceLock'
import type { ResourceLock } from '@/types/resourceLock'

import { useLockedWriteRefresh } from '../useLockedWriteRefresh'

interface Row {
  id: string
  status: string
  lock: ResourceLock | null
}

const loaded: Row = { id: 'a1', status: 'active', lock: null }
const locked: Row = { id: 'a1', status: 'archived', lock: TEST_LOCK }

describe('useLockedWriteRefresh', () => {
  it('reports the loaded resource’s lock until a write is refused', () => {
    const refetch = vi.fn()
    const { result, rerender } = renderHook(
      ({ resource }) => useLockedWriteRefresh(resource, refetch),
      { initialProps: { resource: loaded } }
    )
    expect(result.current.lock).toBeNull()
    expect(result.current.record).toBeUndefined()

    rerender({ resource: locked })
    expect(result.current.lock).toEqual(TEST_LOCK)
    expect(refetch).not.toHaveBeenCalled()
  })

  it('refetches on a lock-refused write and hands over the fresh resource', async () => {
    const refetch = vi.fn().mockResolvedValue(locked)
    const { result } = renderHook(() => useLockedWriteRefresh(loaded, refetch))

    let fresh: unknown
    await act(async () => {
      fresh = await result.current.refreshOnLockedWrite(lockedWriteError())
    })

    expect(fresh).toBe(locked)
    expect(result.current.lock).toEqual(TEST_LOCK)
    expect(result.current.record).toBe(locked)
  })

  it('does not refetch for any other failure', async () => {
    const refetch = vi.fn()
    const { result } = renderHook(() => useLockedWriteRefresh(loaded, refetch))

    let fresh: unknown = 'sentinel'
    await act(async () => {
      fresh = await result.current.refreshOnLockedWrite(new Error('offline'))
    })

    expect(fresh).toBeNull()
    expect(refetch).not.toHaveBeenCalled()
    expect(result.current.lock).toBeNull()
  })

  it('leaves the page as it was when the refetch itself fails', async () => {
    const refetch = vi.fn().mockRejectedValue(new Error('offline'))
    const { result } = renderHook(() => useLockedWriteRefresh(loaded, refetch))

    let fresh: unknown = 'sentinel'
    await act(async () => {
      fresh = await result.current.refreshOnLockedWrite(lockedWriteError())
    })

    expect(fresh).toBeNull()
    expect(result.current.lock).toBeNull()
    expect(result.current.record).toBeUndefined()
  })

  it('drops a refetch once the page shows a different resource', async () => {
    const refetch = vi.fn().mockResolvedValue(locked)
    const { result, rerender } = renderHook(
      ({ resource }) => useLockedWriteRefresh(resource, refetch),
      { initialProps: { resource: loaded } }
    )
    await act(async () => {
      await result.current.refreshOnLockedWrite(lockedWriteError())
    })
    expect(result.current.lock).toEqual(TEST_LOCK)

    rerender({ resource: { id: 'a2', status: 'active', lock: null } })
    expect(result.current.lock).toBeNull()
    expect(result.current.record).toBeUndefined()
  })
})
