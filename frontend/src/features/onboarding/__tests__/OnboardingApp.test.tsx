import { render, screen } from '@testing-library/react'
import { HelmetProvider } from 'react-helmet-async'
import { MemoryRouter } from 'react-router'
import { describe, expect, it, vi } from 'vitest'

import { OnboardingApp } from '../OnboardingApp'

// Sonner reads matchMedia, which jsdom lacks.
vi.mock('@/components/ui/sonner', () => ({ Toaster: () => null }))
vi.mock('@/lib/theme', () => ({
  useTheme: () => ({ resolvedTheme: 'light' }),
  useResolvedTheme: () => 'light',
}))
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({
    isAuthenticated: true,
    isLoading: false,
    user: { id: 'u1', onboarding_completed: false },
    markOnboardingComplete: vi.fn(),
  }),
}))

describe('OnboardingApp', () => {
  it('renders the onboarding for a signed-in user, outside the app shell', () => {
    render(
      <HelmetProvider>
        <MemoryRouter initialEntries={['/getting-started?step=0']}>
          <OnboardingApp />
        </MemoryRouter>
      </HelmetProvider>
    )
    expect(
      screen.getByRole('heading', {
        name: "Your team's shared brain for AI agents",
      })
    ).toBeInTheDocument()
    // No app-shell navigation: only the onboarding's own step list.
    expect(
      screen.getByRole('navigation', { name: 'Onboarding steps' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Skip to content' })).toBeNull()
  })
})
