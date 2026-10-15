import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { STORAGE_KEYS } from '@/constants/storageKeys'

import { resetAutoOpened } from '../autoOpen'
import { OnboardingRedirect } from '../OnboardingRedirect'

const mockAuth = vi.hoisted(() => ({
  user: null as { id: string; onboarding_completed: boolean } | null,
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockAuth,
}))

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <OnboardingRedirect />
      <Routes>
        <Route path="/" element={<p>dashboard</p>} />
        <Route path="/getting-started" element={<p>onboarding</p>} />
        <Route path="*" element={<p>deep link</p>} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  window.sessionStorage.clear()
  resetAutoOpened()
  mockAuth.user = { id: 'u1', onboarding_completed: false }
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('OnboardingRedirect', () => {
  it('opens the onboarding for a new user landing on the dashboard', async () => {
    renderAt('/')
    expect(await screen.findByText('onboarding')).toBeInTheDocument()
    expect(
      window.sessionStorage.getItem(STORAGE_KEYS.ONBOARDING_AUTO_OPENED)
    ).toBe('u1')
  })

  it('does not redirect once onboarding is completed', () => {
    mockAuth.user = { id: 'u1', onboarding_completed: true }
    renderAt('/')
    expect(screen.getByText('dashboard')).toBeInTheDocument()
  })

  it('does not hijack a deep link', () => {
    renderAt('/memories/123')
    expect(screen.getByText('deep link')).toBeInTheDocument()
  })

  it('opens only once per browser session', () => {
    window.sessionStorage.setItem(STORAGE_KEYS.ONBOARDING_AUTO_OPENED, 'u1')
    renderAt('/')
    expect(screen.getByText('dashboard')).toBeInTheDocument()
  })

  it('still opens once per page load when sessionStorage is unavailable', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    vi.spyOn(console, 'error').mockImplementation(() => undefined)

    const first = renderAt('/')
    expect(await screen.findByText('onboarding')).toBeInTheDocument()
    first.unmount()

    renderAt('/')
    expect(screen.getByText('dashboard')).toBeInTheDocument()
  })

  it('lets a pending invitation resume first', () => {
    window.sessionStorage.setItem(STORAGE_KEYS.PENDING_INVITATION_TOKEN, 'tok')
    renderAt('/')
    expect(screen.getByText('dashboard')).toBeInTheDocument()
  })

  it('lets a pending return_to resume first, but not the default "/"', async () => {
    window.sessionStorage.setItem(STORAGE_KEYS.RETURN_TO, '/oauth/consent?x=1')
    const view = renderAt('/')
    expect(screen.getByText('dashboard')).toBeInTheDocument()
    view.unmount()

    window.sessionStorage.setItem(STORAGE_KEYS.RETURN_TO, '/')
    renderAt('/')
    expect(await screen.findByText('onboarding')).toBeInTheDocument()
  })

  it('waits for a signed-in user', () => {
    mockAuth.user = null
    renderAt('/')
    expect(screen.getByText('dashboard')).toBeInTheDocument()
  })
})
