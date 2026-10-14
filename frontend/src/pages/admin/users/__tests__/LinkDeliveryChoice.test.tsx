import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { LinkDeliveryChoice } from '../LinkDeliveryChoice'

function renderChoice(emailLinks: boolean | null, busy = false) {
  const onChoose = vi.fn()
  render(
    <MemoryRouter>
      <LinkDeliveryChoice
        email="grace@example.com"
        emailLinks={emailLinks}
        busy={busy}
        onChoose={onChoose}
      />
    </MemoryRouter>
  )
  return { onChoose, user: userEvent.setup() }
}

const emailButton = () =>
  screen.getByRole('button', { name: 'Email it to grace@example.com' })
const showButton = () =>
  screen.getByRole('button', { name: 'Show the link here' })

describe('LinkDeliveryChoice (#1494)', () => {
  it('offers both, emailing first, when email works', async () => {
    const { onChoose, user } = renderChoice(true)

    expect(emailButton()).toBeEnabled()
    expect(showButton()).toBeEnabled()
    expect(screen.getAllByRole('button')[0]).toBe(emailButton())
    expect(screen.queryByTestId('link-delivery-no-email')).toBeNull()

    await user.click(emailButton())
    expect(onChoose).toHaveBeenCalledWith('email')
  })

  it('disables emailing with the reason, and leads with showing the link, when no email provider is configured', async () => {
    const { onChoose, user } = renderChoice(false)

    expect(emailButton()).toBeDisabled()
    expect(showButton()).toBeEnabled()
    expect(screen.getAllByRole('button')[0]).toBe(showButton())
    const reason = screen.getByTestId('link-delivery-no-email')
    expect(reason).toHaveTextContent(
      'No email provider is configured, so the link cannot be emailed'
    )
    expect(screen.getByRole('link', { name: 'Set up email' })).toHaveAttribute(
      'href',
      '/admin/settings/email'
    )

    await user.click(showButton())
    expect(onChoose).toHaveBeenCalledWith('link')
  })

  it('offers both while it is not known whether email works', () => {
    renderChoice(null)

    expect(emailButton()).toBeEnabled()
    expect(showButton()).toBeEnabled()
    expect(screen.queryByTestId('link-delivery-no-email')).toBeNull()
  })

  it('disables both while a request is running', () => {
    renderChoice(false, true)

    expect(emailButton()).toBeDisabled()
    expect(showButton()).toBeDisabled()
  })
})
