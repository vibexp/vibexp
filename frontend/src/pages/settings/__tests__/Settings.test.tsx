import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { Settings } from '../Settings'

// Guards the #537 extraction of the card grid into
// components/settings/SettingsGrid: the hub must keep rendering the same
// sections and the same cards after the markup moved out of this page.
const SECTIONS = ['General', 'Integration', 'Customization', 'Collaboration']

const CARDS = [
  'Activities',
  'Notification Preferences',
  'API Keys',
  'Embedding Providers',
  'Model Providers',
  'GitHub Integration',
  'Artifact Types',
  'Search Settings',
  'Teams',
  'Projects',
]

describe('Settings', () => {
  beforeEach(() => {
    render(
      <MemoryRouter>
        <Settings />
      </MemoryRouter>
    )
  })

  it('renders the page header', () => {
    expect(
      screen.getByRole('heading', { level: 1, name: 'Settings' })
    ).toBeInTheDocument()
  })

  it.each(SECTIONS)('renders the %s section', section => {
    expect(
      screen.getByRole('heading', { level: 2, name: section })
    ).toBeInTheDocument()
  })

  it.each(CARDS)('renders the %s card', card => {
    expect(screen.getByText(card)).toBeInTheDocument()
  })

  it('renders exactly one card per setting item', () => {
    expect(screen.getAllByRole('button')).toHaveLength(CARDS.length)
  })
})
