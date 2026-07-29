import type { Mock } from 'vitest'
import { renderHook, act } from '@testing-library/react'

// Mock the centralized storage utilities - MUST be before imports
vi.mock('../../src/utils/storage', () => ({
  storage: {
    get: vi.fn(),
    set: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
    has: vi.fn(),
  },
  sessionStore: {
    get: vi.fn(),
    set: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
    has: vi.fn(),
  },
  STORAGE_KEYS: {
    CURRENT_TEAM_ID: 'vibexp_current_team_id',
    PENDING_INVITATION_TOKEN: 'pending_invitation_token',
    INVITATION_BANNER_DISMISSED: 'invitation_banner_dismissed',
    ANALYTICS_REFERRER: 'analytics_referrer',
    PURCHASE_TRACKED: 'purchase_tracked',
    COOKIE_CONSENT: 'cookieConsent',
  },
}))

import { useLocalStorage } from '../../src/hooks/useLocalStorage'
import { storage, STORAGE_KEYS } from '../../src/utils/storage'

// Get the mocked functions
const mockGet = storage.get as Mock
const mockSet = storage.set as Mock
const mockClear = storage.clear as Mock

describe('useLocalStorage', () => {
  beforeEach(() => {
    mockClear.mockImplementation(() => {})
    vi.clearAllMocks()
    mockGet.mockReturnValue(null)
  })

  it('returns initial value when storage is empty', () => {
    mockGet.mockReturnValue(null)

    const { result } = renderHook(() =>
      useLocalStorage(STORAGE_KEYS.CURRENT_TEAM_ID, 'initial')
    )

    expect(result.current[0]).toBe('initial')
  })

  it('returns stored value from storage', () => {
    mockGet.mockReturnValue('stored-value')

    const { result } = renderHook(() =>
      useLocalStorage(STORAGE_KEYS.CURRENT_TEAM_ID, 'initial')
    )

    expect(result.current[0]).toBe('stored-value')
  })

  it('updates storage when value changes', () => {
    mockGet.mockReturnValue(null)

    const { result } = renderHook(() =>
      useLocalStorage(STORAGE_KEYS.CURRENT_TEAM_ID, 'initial')
    )

    act(() => {
      result.current[1]('new-value')
    })

    expect(result.current[0]).toBe('new-value')
    expect(mockSet).toHaveBeenCalledWith(
      STORAGE_KEYS.CURRENT_TEAM_ID,
      'new-value'
    )
  })

  it('handles function updates', () => {
    mockGet.mockReturnValue(0)

    const { result } = renderHook(() =>
      useLocalStorage(STORAGE_KEYS.CURRENT_TEAM_ID, 0)
    )

    act(() => {
      result.current[1]((prev: number) => prev + 1)
    })

    expect(result.current[0]).toBe(1)
    expect(mockSet).toHaveBeenCalledWith(STORAGE_KEYS.CURRENT_TEAM_ID, 1)
  })
})
