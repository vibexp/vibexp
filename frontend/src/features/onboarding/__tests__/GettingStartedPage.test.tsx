import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HelmetProvider } from 'react-helmet-async'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { STORAGE_KEYS } from '@/constants/storageKeys'

import { resetAutoOpened } from '../autoOpen'
import { RULES, RULES_APPEND } from '../content'
import { GettingStartedPage } from '../GettingStartedPage'
import { readProgress, resetProgressCache } from '../progress'

const mockAuth = vi.hoisted(() => ({
  user: { id: 'u1', onboarding_completed: false },
  markOnboardingComplete: vi.fn<() => Promise<void>>(),
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockAuth,
}))

const mockToastError = vi.hoisted(() => vi.fn())
vi.mock('@/lib/toast', () => ({
  toast: { error: mockToastError },
}))

function Where() {
  const { pathname, search } = useLocation()
  return <output data-testid="where">{pathname + search}</output>
}

function renderFlow(path = '/getting-started') {
  return render(
    <HelmetProvider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/getting-started" element={<GettingStartedPage />} />
          <Route path="/" element={<p>dashboard</p>} />
        </Routes>
        <Where />
      </MemoryRouter>
    </HelmetProvider>
  )
}

const where = () => screen.getByTestId('where').textContent

const nextButton = (name: string | RegExp) =>
  screen.getByRole('button', { name })

const writeText = vi.fn<(text: string) => Promise<void>>()

beforeEach(() => {
  window.localStorage.clear()
  window.sessionStorage.clear()
  resetProgressCache()
  resetAutoOpened()
  mockAuth.user = { id: 'u1', onboarding_completed: false }
  mockAuth.markOnboardingComplete.mockReset().mockResolvedValue(undefined)
  mockToastError.mockReset()
  writeText.mockReset().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText },
    configurable: true,
  })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('GettingStartedPage', () => {
  it('opens on the welcome step with the video and pins the step in the URL', () => {
    renderFlow()
    expect(
      screen.getByRole('heading', {
        level: 1,
        name: "Your team's shared brain for AI agents",
      })
    ).toBeInTheDocument()
    expect(
      screen.getByTitle("How VibeXP's shared brain works")
    ).toHaveAttribute(
      'src',
      'https://www.youtube-nocookie.com/embed/-9NFGBjoc5g?rel=0'
    )
    expect(where()).toBe('/getting-started?step=0')
    // No Back and no "Skip this step" on the first step.
    expect(screen.queryByRole('button', { name: /Back/ })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Skip this step' })).toBeNull()
    expect(screen.getByText('About 5 minutes')).toBeInTheDocument()
  })

  it('walks forward, marking steps done, and back', async () => {
    const user = userEvent.setup()
    renderFlow()

    await user.click(nextButton(/Get started/))
    expect(where()).toBe('/getting-started?step=1')
    expect(
      screen.getByRole('heading', { name: 'What lives in the shared brain' })
    ).toBeInTheDocument()
    expect(readProgress('u1').status[0]).toBe('done')

    await user.click(nextButton(/Continue/))
    expect(
      screen.getByRole('heading', { name: 'Install the VibeXP plugin' })
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Back/ }))
    expect(where()).toBe('/getting-started?step=1')
    expect(readProgress('u1').lastStep).toBe(1)
  })

  it('moves focus to the new step heading', async () => {
    const user = userEvent.setup()
    renderFlow()
    await user.click(nextButton(/Get started/))
    expect(
      screen.getByRole('heading', { name: 'What lives in the shared brain' })
    ).toHaveFocus()
  })

  it('skips a step, and the rail and summary show it', async () => {
    const user = userEvent.setup()
    renderFlow('/getting-started?step=2')

    await user.click(screen.getByRole('button', { name: 'Skip this step' }))
    expect(where()).toBe('/getting-started?step=3')
    expect(readProgress('u1').status[2]).toBe('skipped')

    const rail = screen.getByRole('navigation', { name: 'Onboarding steps' })
    expect(
      within(rail).getByRole('button', { name: /Claude Code plugin, skipped/ })
    ).toBeInTheDocument()
    expect(
      within(rail).getByRole('button', { name: /Team rules/ })
    ).toHaveAttribute('aria-current', 'step')

    // The celebration lists the unfinished step with a way back to it.
    await user.click(within(rail).getByRole('button', { name: /All set/ }))
    await user.click(
      screen.getByRole('button', {
        name: 'Finish this step: Claude Code plugin',
      })
    )
    expect(where()).toBe('/getting-started?step=2')
  })

  it('resumes at the first step not done when no step is given', () => {
    window.localStorage.setItem(
      STORAGE_KEYS.ONBOARDING_PROGRESS,
      JSON.stringify({
        u1: {
          status: ['done', 'done', 'skipped', 'todo', 'todo', 'todo'],
          lastStep: 4,
        },
      })
    )
    renderFlow()
    expect(where()).toBe('/getting-started?step=2')
  })

  it('clamps an out-of-range ?step', () => {
    renderFlow('/getting-started?step=99')
    expect(where()).toBe('/getting-started?step=5')
    expect(
      screen.getByRole('heading', {
        name: 'Your agents now learn with your team',
      })
    ).toBeInTheDocument()
  })

  it('"Skip setup" completes onboarding and goes to the dashboard', async () => {
    const user = userEvent.setup()
    renderFlow('/getting-started?step=1')
    await user.click(screen.getByRole('button', { name: 'Skip setup' }))

    expect(mockAuth.markOnboardingComplete).toHaveBeenCalledTimes(1)
    expect(await screen.findByText('dashboard')).toBeInTheDocument()
    expect(
      window.sessionStorage.getItem(STORAGE_KEYS.ONBOARDING_AUTO_OPENED)
    ).toBe('u1')
  })

  it('"Go to VibeXP" on the last step completes onboarding', async () => {
    const user = userEvent.setup()
    renderFlow('/getting-started?step=5')
    await user.click(nextButton(/Go to VibeXP/))
    expect(mockAuth.markOnboardingComplete).toHaveBeenCalledTimes(1)
    expect(await screen.findByText('dashboard')).toBeInTheDocument()
  })

  it('does not call the server again for a user who already completed it', async () => {
    mockAuth.user = { id: 'u1', onboarding_completed: true }
    const user = userEvent.setup()
    renderFlow('/getting-started?step=5')
    await user.click(nextButton(/Go to VibeXP/))
    expect(mockAuth.markOnboardingComplete).not.toHaveBeenCalled()
    expect(await screen.findByText('dashboard')).toBeInTheDocument()
  })

  it('still leaves, with a toast, when saving completion fails', async () => {
    mockAuth.markOnboardingComplete.mockRejectedValue(new Error('500'))
    const user = userEvent.setup()
    renderFlow()
    await user.click(screen.getByRole('button', { name: 'Skip setup' }))
    expect(await screen.findByText('dashboard')).toBeInTheDocument()
    expect(mockToastError).toHaveBeenCalledTimes(1)
  })

  it('switches the building-block tabs', async () => {
    const user = userEvent.setup()
    renderFlow('/getting-started?step=1')
    const tablist = screen.getByRole('tablist', { name: 'Building blocks' })
    const tabs = within(tablist).getAllByRole('tab')
    expect(tabs.map(t => t.textContent)).toEqual([
      'Memory',
      'Blueprints',
      'Artifacts',
      'AI Feeds',
    ])
    expect(screen.getByRole('tabpanel')).toHaveTextContent('Idempotency-Key')

    await user.click(screen.getByRole('tab', { name: 'AI Feeds' }))
    expect(screen.getByRole('tab', { name: 'AI Feeds' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByRole('tabpanel')).toHaveTextContent(
      'Terraform module naming'
    )
  })

  it('copies the plugin commands for the chosen place', () => {
    renderFlow('/getting-started?step=2')

    fireEvent.click(
      screen.getByRole('button', { name: 'Copy marketplace command' })
    )
    expect(writeText).toHaveBeenLastCalledWith(
      '/plugin marketplace add vibexp/plugin-marketplace'
    )
    expect(
      screen.getByRole('button', { name: 'Copied marketplace command' })
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'From your terminal' }))
    expect(
      screen.getByRole('button', { name: 'From your terminal' })
    ).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(
      screen.getByRole('button', { name: 'Copy install command' })
    )
    expect(writeText).toHaveBeenLastCalledWith(
      'claude plugin install vibexp@vibexp'
    )
  })

  it('copies the rules, or the command that appends them', () => {
    renderFlow('/getting-started?step=3')
    fireEvent.click(screen.getByRole('button', { name: 'Copy rules' }))
    expect(writeText).toHaveBeenLastCalledWith(RULES)

    fireEvent.click(
      screen.getByRole('button', { name: 'Append with one command' })
    )
    // Still "Copied" from the click before; the label follows the mode.
    fireEvent.click(
      screen.getByRole('button', { name: /^(Copied|Copy command)$/ })
    )
    expect(writeText).toHaveBeenLastCalledWith(RULES_APPEND)
    expect(RULES_APPEND).toMatch(/^cat >> ~\/\.claude\/CLAUDE\.md <<'EOF'\n\n/)
    expect(RULES_APPEND.endsWith('\nEOF')).toBe(true)
  })

  it('offers the CLI install with this instance as its argument', () => {
    renderFlow('/getting-started?step=4')
    const command = `claude "/vibexp:install ${window.location.origin}"`
    expect(screen.getByText(command)).toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Copy install command' })
    )
    expect(writeText).toHaveBeenLastCalledWith(command)
  })

  it('shows "Copied" only for a moment', () => {
    vi.useFakeTimers()
    renderFlow('/getting-started?step=4')
    fireEvent.click(
      screen.getByRole('button', { name: 'Copy install command' })
    )
    expect(
      screen.getByRole('button', { name: 'Copied install command' })
    ).toBeInTheDocument()
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(
      screen.getByRole('button', { name: 'Copy install command' })
    ).toBeInTheDocument()
  })

  it('falls back to a selection copy outside a secure context', () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: undefined,
      configurable: true,
    })
    // jsdom has no execCommand; the page must not depend on its result.
    const execCommand = vi.fn(() => true)
    Object.defineProperty(document, 'execCommand', {
      value: execCommand,
      configurable: true,
    })
    renderFlow('/getting-started?step=4')
    fireEvent.click(
      screen.getByRole('button', { name: 'Copy install command' })
    )
    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(document.querySelector('textarea')).toBeNull()
  })
})
