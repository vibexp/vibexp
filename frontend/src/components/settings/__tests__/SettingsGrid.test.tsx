import { createEvent, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Bell, Key } from 'lucide-react'
import { MemoryRouter, Route, Routes } from 'react-router'

import { type SettingItem, SettingSection } from '../SettingsGrid'

const ITEMS: SettingItem[] = [
  {
    title: 'Notification Preferences',
    description: 'Manage your email notification settings and preferences.',
    icon: Bell,
    href: '/settings/notifications',
  },
  {
    title: 'API Keys',
    description: 'Create and manage API keys for programmatic access.',
    icon: Key,
    href: '/settings/api-keys',
  },
]

function renderSection(items: SettingItem[] = ITEMS) {
  return render(
    <MemoryRouter initialEntries={['/settings']}>
      <Routes>
        <Route
          path="/settings"
          element={<SettingSection title="General" items={items} />}
        />
        <Route path="/settings/notifications" element={<p>notifications</p>} />
        <Route path="/settings/api-keys" element={<p>api keys</p>} />
      </Routes>
    </MemoryRouter>
  )
}

describe('SettingSection', () => {
  it('renders the section heading and one card per item', () => {
    renderSection()

    expect(
      screen.getByRole('heading', { level: 2, name: 'General' })
    ).toBeInTheDocument()
    expect(screen.getAllByRole('button')).toHaveLength(ITEMS.length)
    for (const item of ITEMS) {
      expect(screen.getByText(item.title)).toBeInTheDocument()
      expect(screen.getByText(item.description)).toBeInTheDocument()
    }
  })

  it('navigates to the item href on click', async () => {
    renderSection()

    await userEvent.click(
      screen.getByRole('button', { name: /Notification Preferences/ })
    )

    expect(screen.getByText('notifications')).toBeInTheDocument()
  })

  it('navigates to the item href on Enter', () => {
    renderSection()

    fireEvent.keyDown(screen.getByRole('button', { name: /API Keys/ }), {
      key: 'Enter',
    })

    expect(screen.getByText('api keys')).toBeInTheDocument()
  })

  it('navigates to the item href on Space and suppresses the default scroll', () => {
    renderSection()

    // Space on a focused element scrolls the page, so the handler must cancel
    // the event as well as navigate. Asserting navigation alone would stay
    // green if the preventDefault() call were removed.
    const event = createEvent.keyDown(
      screen.getByRole('button', { name: /API Keys/ }),
      { key: ' ' }
    )
    fireEvent(screen.getByRole('button', { name: /API Keys/ }), event)

    expect(event.defaultPrevented).toBe(true)
    expect(screen.getByText('api keys')).toBeInTheDocument()
  })

  it('suppresses the default action on Enter too', () => {
    renderSection()

    const event = createEvent.keyDown(
      screen.getByRole('button', { name: /API Keys/ }),
      { key: 'Enter' }
    )
    fireEvent(screen.getByRole('button', { name: /API Keys/ }), event)

    expect(event.defaultPrevented).toBe(true)
  })

  it('does not suppress the default action for other keys', () => {
    renderSection()

    const event = createEvent.keyDown(
      screen.getByRole('button', { name: /API Keys/ }),
      { key: 'a' }
    )
    fireEvent(screen.getByRole('button', { name: /API Keys/ }), event)

    expect(event.defaultPrevented).toBe(false)
  })

  it('ignores other keys', () => {
    renderSection()

    fireEvent.keyDown(screen.getByRole('button', { name: /API Keys/ }), {
      key: 'a',
    })

    expect(screen.queryByText('api keys')).not.toBeInTheDocument()
    expect(
      screen.getByRole('heading', { level: 2, name: 'General' })
    ).toBeInTheDocument()
  })

  it('exposes each card as a focusable button', () => {
    renderSection()

    for (const card of screen.getAllByRole('button')) {
      expect(card).toHaveAttribute('tabindex', '0')
    }
  })

  it('renders nothing but the heading when there are no items', () => {
    renderSection([])

    expect(
      screen.getByRole('heading', { level: 2, name: 'General' })
    ).toBeInTheDocument()
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })
})
