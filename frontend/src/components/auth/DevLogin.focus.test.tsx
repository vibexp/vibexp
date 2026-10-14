import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { DevLogin } from './DevLogin'

vi.mock('../../services/authService', () => ({
  authService: { devLogin: vi.fn() },
}))

vi.mock('../../services/environmentService', () => ({
  environmentService: { isDevLoginEnabled: () => true },
}))

// The real Collapsible is used here on purpose: the focus move depends on the
// form mounting when the section opens.
function renderDevLogin() {
  render(
    <MemoryRouter>
      <DevLogin />
    </MemoryRouter>
  )
  return screen.getByRole('button', { name: 'Development login' })
}

describe('DevLogin focus', () => {
  it('starts collapsed and does not take focus on mount', () => {
    const toggle = renderDevLogin()

    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByLabelText('Email')).not.toBeInTheDocument()
    expect(document.body).toHaveFocus()
  })

  it('moves focus to the email field when the section is expanded', async () => {
    const user = userEvent.setup()
    const toggle = renderDevLogin()

    await user.click(toggle)

    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    const email = screen.getByLabelText('Email')
    expect(email).toHaveFocus()
    // The toggle names the region it controls, and that region holds the form.
    const region = document.getElementById(
      toggle.getAttribute('aria-controls') ?? ''
    )
    expect(region).toContainElement(email)
  })

  it('opens from the keyboard and lands on the email field', async () => {
    const user = userEvent.setup()
    const toggle = renderDevLogin()

    await user.tab()
    expect(toggle).toHaveFocus()
    await user.keyboard('{Enter}')

    expect(screen.getByLabelText('Email')).toHaveFocus()
  })

  it('returns focus to the toggle when the section is collapsed', async () => {
    const user = userEvent.setup()
    const toggle = renderDevLogin()

    await user.click(toggle)
    await user.type(screen.getByLabelText('Email'), 'dev@example.com')
    await user.click(toggle)

    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveFocus()
  })

  // The fields' focus ring is drawn outside their box, so any clipping
  // container around them cuts it into a stray line above and below (#1497).
  // jsdom cannot see clipping; the geometry is checked in
  // e2e/features/auth/dev-login-focus-ring.spec.ts. This pins the structure.
  it('puts no clipping container between the fields and the section', async () => {
    const user = userEvent.setup()
    const toggle = renderDevLogin()
    await user.click(toggle)

    const section = toggle.parentElement
    if (!section) throw new Error('the toggle has no parent')
    for (const label of ['Email', /Name/]) {
      let node = screen.getByLabelText(label).parentElement
      while (node && node !== section) {
        expect(node.className).not.toMatch(
          /\boverflow-(?:x-|y-)?(?:hidden|clip|auto|scroll)\b/
        )
        node = node.parentElement
      }
      expect(node).toBe(section)
    }
  })
})
