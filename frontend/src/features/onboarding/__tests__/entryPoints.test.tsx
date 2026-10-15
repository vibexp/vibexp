import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { STORAGE_KEYS } from '@/constants/storageKeys'

import { OnboardingCard } from '../OnboardingCard'
import {
  type OnboardingProgress,
  readProgress,
  resetProgressCache,
  withStepStatus,
  writeProgress,
} from '../progress'

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'u1', onboarding_completed: true } }),
}))

function setProgress(status: OnboardingProgress['status']) {
  writeProgress('u1', { status, lastStep: 0 })
}

function renderIn(node: React.ReactNode) {
  return render(<MemoryRouter>{node}</MemoryRouter>)
}

beforeEach(() => {
  window.localStorage.clear()
  resetProgressCache()
})

describe('OnboardingCard', () => {
  it('offers Resume and Replay while steps are left', () => {
    setProgress(['done', 'skipped', 'todo', 'todo', 'todo', 'todo'])
    renderIn(<OnboardingCard />)

    expect(
      screen.getByRole('heading', { name: 'Finish setting up VibeXP' })
    ).toBeInTheDocument()
    expect(
      screen.getByText('1 of 5 steps complete. Pick up where you left off.')
    ).toBeInTheDocument()
    expect(screen.getByRole('progressbar')).toHaveAttribute(
      'aria-valuenow',
      '20'
    )
    expect(screen.getByRole('link', { name: /Resume setup/ })).toHaveAttribute(
      'href',
      '/getting-started?step=1'
    )
    expect(
      screen.getByRole('link', { name: /Replay onboarding/ })
    ).toHaveAttribute('href', '/getting-started?step=0')
  })

  it('follows progress made elsewhere without a reload', () => {
    renderIn(<OnboardingCard />)
    expect(screen.getByText(/0 of 5 steps complete/)).toBeInTheDocument()
    act(() => {
      setProgress(['done', 'done', 'todo', 'todo', 'todo', 'todo'])
    })
    expect(screen.getByText(/2 of 5 steps complete/)).toBeInTheDocument()
  })

  it('can be dismissed for good, without touching the progress', async () => {
    const user = userEvent.setup()
    setProgress(['done', 'skipped', 'todo', 'todo', 'todo', 'todo'])
    const { container, unmount } = renderIn(<OnboardingCard />)

    await user.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(container).toBeEmptyDOMElement()
    expect(readProgress('u1').status[0]).toBe('done')

    // Persisted per user: still hidden after a reload.
    unmount()
    resetProgressCache()
    const stored = JSON.parse(
      window.localStorage.getItem(STORAGE_KEYS.ONBOARDING_PROGRESS) ?? '{}'
    ) as Record<string, { cardDismissed?: boolean }>
    expect(stored.u1.cardDismissed).toBe(true)
    expect(renderIn(<OnboardingCard />).container).toBeEmptyDOMElement()
  })

  it('stays dismissed through progress and a replay', () => {
    writeProgress('u1', {
      status: ['done', 'todo', 'todo', 'todo', 'todo', 'todo'],
      lastStep: 0,
      cardDismissed: true,
    })
    const { container } = renderIn(<OnboardingCard />)
    act(() => {
      writeProgress('u1', withStepStatus(readProgress('u1'), 1, 'done'))
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('is gone once all five steps are done', () => {
    setProgress(['done', 'done', 'done', 'done', 'done', 'todo'])
    const { container } = renderIn(<OnboardingCard />)
    expect(container).toBeEmptyDOMElement()
  })
})
