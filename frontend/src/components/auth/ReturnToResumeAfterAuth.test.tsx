import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { STORAGE_KEYS } from '@/constants/storageKeys'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockNavigate = jest.fn()
let mockPathname = '/'
let mockSearch = ''

jest.mock('react-router-dom', () => {
  const actual =
    jest.requireActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useLocation: () => ({
      pathname: mockPathname,
      search: mockSearch,
      hash: '',
      state: null,
      key: 'test',
    }),
  }
})

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { ReturnToResumeAfterAuth } from './ReturnToResumeAfterAuth'

const renderHere = () =>
  render(
    <MemoryRouter>
      <ReturnToResumeAfterAuth />
    </MemoryRouter>
  )

describe('ReturnToResumeAfterAuth', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    window.sessionStorage.clear()
    mockPathname = '/'
    mockSearch = ''
  })

  it('does nothing when no return_to is stashed', () => {
    renderHere()
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('navigates to the stashed return_to and clears it', () => {
    window.sessionStorage.setItem(
      STORAGE_KEYS.RETURN_TO,
      '/oauth/consent?login=abc'
    )

    renderHere()

    expect(mockNavigate).toHaveBeenCalledWith('/oauth/consent?login=abc', {
      replace: true,
    })
    // Single-use: consumed.
    expect(window.sessionStorage.getItem(STORAGE_KEYS.RETURN_TO)).toBeNull()
  })

  it('does nothing when return_to is "/"', () => {
    window.sessionStorage.setItem(STORAGE_KEYS.RETURN_TO, '/')

    renderHere()

    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('does nothing when already on the return_to location', () => {
    mockPathname = '/oauth/consent'
    mockSearch = '?login=abc'
    window.sessionStorage.setItem(
      STORAGE_KEYS.RETURN_TO,
      '/oauth/consent?login=abc'
    )

    renderHere()

    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('ignores an unsafe stashed value (open-redirect guard)', () => {
    window.sessionStorage.setItem(STORAGE_KEYS.RETURN_TO, '//evil.com')

    renderHere()

    expect(mockNavigate).not.toHaveBeenCalled()
  })
})
