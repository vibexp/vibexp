/**
 * useInstanceEmailStatus (#1192): the one place the instance email payload is
 * turned into a state, and the fetch/refetch lifecycle both the admin banner
 * and the dashboard card depend on.
 */
import { act, renderHook, waitFor } from '@testing-library/react'

import type { AdminInstanceEmailSettings } from '@/services/adminService'
import { adminService } from '@/services/adminService'

vi.mock('@/services/adminService', () => ({
  adminService: { getInstanceEmailSettings: vi.fn() },
}))

import { emitInstanceEmailChanged } from '../instanceEmailEvents'
import {
  instanceEmailState,
  useInstanceEmailStatus,
} from '../useInstanceEmailStatus'

const service = vi.mocked(adminService)

const unconfigured: AdminInstanceEmailSettings = {
  configured: false,
  provider_type: null,
  has_credential: false,
  is_healthy: null,
}

const configured = (
  overrides: Partial<AdminInstanceEmailSettings> = {}
): AdminInstanceEmailSettings => ({
  configured: true,
  provider_type: 'smtp',
  has_credential: true,
  is_healthy: true,
  last_success_at: '2026-09-20T10:00:00Z',
  ...overrides,
})

beforeEach(() => {
  vi.clearAllMocks()
})

describe('instanceEmailState', () => {
  it.each([
    ['nothing configured', unconfigured, 'unconfigured'],
    ['configured and healthy', configured(), 'healthy'],
    [
      'configured, never sent',
      configured({
        is_healthy: true,
        last_success_at: null,
        last_error_at: null,
      }),
      'healthy',
    ],
    [
      'configured, last send failed',
      configured({
        is_healthy: false,
        last_error: 'connection refused',
        last_error_at: '2026-09-21T10:00:00Z',
      }),
      'failing',
    ],
    [
      // `is_healthy` is the verdict; a retained earlier error is not failing.
      'recovered after an earlier error',
      configured({
        is_healthy: true,
        last_error: 'connection refused',
        last_error_at: '2026-09-19T10:00:00Z',
      }),
      'healthy',
    ],
    [
      // The rule lives server-side: timestamps are not compared again here.
      'server says failing even though the timestamps disagree',
      configured({
        is_healthy: false,
        last_success_at: '2026-09-22T10:00:00Z',
        last_error_at: '2026-09-21T10:00:00Z',
      }),
      'failing',
    ],
  ])('%s → %s', (_label, settings, expected) => {
    expect(instanceEmailState(settings)).toBe(expected)
  })
})

describe('useInstanceEmailStatus', () => {
  it('is loading until the first read settles, then maps the payload', async () => {
    service.getInstanceEmailSettings.mockResolvedValue(unconfigured)
    const { result } = renderHook(() => useInstanceEmailStatus())

    expect(result.current).toEqual({ state: 'loading', settings: null })
    await waitFor(() => {
      expect(result.current.state).toBe('unconfigured')
    })
    expect(result.current.settings).toEqual(unconfigured)
  })

  it('reports a failed read as unknown, never as unconfigured, and does not throw', async () => {
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)
    service.getInstanceEmailSettings.mockRejectedValue(new Error('boom'))
    const { result } = renderHook(() => useInstanceEmailStatus())

    await waitFor(() => {
      expect(result.current).toEqual({ state: 'unknown', settings: null })
    })
    expect(consoleError).toHaveBeenCalled()
    consoleError.mockRestore()
  })

  it('refetches when the settings change', async () => {
    service.getInstanceEmailSettings
      .mockResolvedValueOnce(unconfigured)
      .mockResolvedValueOnce(configured())
    const { result } = renderHook(() => useInstanceEmailStatus())
    await waitFor(() => {
      expect(result.current.state).toBe('unconfigured')
    })

    act(() => {
      emitInstanceEmailChanged()
    })

    await waitFor(() => {
      expect(result.current.state).toBe('healthy')
    })
    expect(service.getInstanceEmailSettings).toHaveBeenCalledTimes(2)
  })

  it('never lets a slow earlier read overwrite a newer one', async () => {
    let resolveFirst: (value: AdminInstanceEmailSettings) => void = () =>
      undefined
    service.getInstanceEmailSettings
      .mockReturnValueOnce(
        new Promise(resolve => {
          resolveFirst = resolve
        })
      )
      .mockResolvedValueOnce(configured())
    const { result } = renderHook(() => useInstanceEmailStatus())

    act(() => {
      emitInstanceEmailChanged()
    })
    await waitFor(() => {
      expect(result.current.state).toBe('healthy')
    })

    await act(async () => {
      resolveFirst(unconfigured)
      await Promise.resolve()
    })
    expect(result.current.state).toBe('healthy')
  })

  it('stops listening once unmounted', async () => {
    service.getInstanceEmailSettings.mockResolvedValue(unconfigured)
    const { result, unmount } = renderHook(() => useInstanceEmailStatus())
    await waitFor(() => {
      expect(result.current.state).toBe('unconfigured')
    })
    unmount()

    emitInstanceEmailChanged()

    expect(service.getInstanceEmailSettings).toHaveBeenCalledTimes(1)
  })
})
