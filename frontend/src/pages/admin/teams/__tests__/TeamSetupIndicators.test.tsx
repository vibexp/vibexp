import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { AdminTeamListItem } from '@/services/adminService'

import { TeamSetupIndicators } from '../TeamSetupIndicators'

type Configuration = AdminTeamListItem['configuration']

const ALL_OFF: Configuration = {
  embedding_configured: false,
  llm_configured: false,
  ai_summary_enabled: false,
  email_configured: false,
  github_configured: false,
  search_settings_customized: false,
  freshness_enabled: false,
}

const ALL_ON: Configuration = Object.fromEntries(
  Object.keys(ALL_OFF).map(key => [key, true])
) as Configuration

it('renders every configuration flag, in a fixed order', () => {
  render(<TeamSetupIndicators configuration={ALL_OFF} />)

  expect(
    screen.getAllByRole('img').map(icon => icon.getAttribute('aria-label'))
  ).toEqual([
    'Embedding: not configured',
    'LLM: not configured',
    'AI summary: disabled',
    'Email: not configured',
    'GitHub: not configured',
    'Search settings: instance defaults',
    'Freshness: disabled',
  ])
})

it.each([
  ['Embedding: configured', 'Embedding: not configured'],
  ['LLM: configured', 'LLM: not configured'],
  ['AI summary: enabled', 'AI summary: disabled'],
  ['Email: configured', 'Email: not configured'],
  ['GitHub: configured', 'GitHub: not configured'],
  ['Search settings: customized', 'Search settings: instance defaults'],
  ['Freshness: enabled', 'Freshness: disabled'],
])('names the state as text: "%s" / "%s"', (onName, offName) => {
  const { unmount } = render(<TeamSetupIndicators configuration={ALL_ON} />)
  expect(screen.getByRole('img', { name: onName })).toBeInTheDocument()
  unmount()

  render(<TeamSetupIndicators configuration={ALL_OFF} />)
  expect(screen.getByRole('img', { name: offName })).toBeInTheDocument()
})

it('dims an unconfigured flag in addition to its text, never by colour alone', () => {
  render(
    <TeamSetupIndicators
      configuration={{ ...ALL_OFF, embedding_configured: true }}
    />
  )

  expect(
    screen.getByRole('img', { name: 'Embedding: configured' })
  ).not.toHaveClass('opacity-30')
  expect(screen.getByRole('img', { name: 'LLM: not configured' })).toHaveClass(
    'opacity-30'
  )
})

it('anchors the tooltip on the real icon box (#891)', async () => {
  render(<TeamSetupIndicators configuration={ALL_OFF} />)
  const icon = screen.getByRole('img', { name: 'Email: not configured' })

  // The Radix trigger props must land on the span itself — a box floating-ui can
  // measure — not on a `display: contents` wrapper. jsdom has no layout, so the
  // geometry belongs to Playwright; this pins the structure.
  expect(icon).toHaveClass('inline-flex', 'size-5')
  expect(icon).toHaveAttribute('data-state', 'closed')

  await userEvent.hover(icon)

  expect(await screen.findByRole('tooltip')).toHaveTextContent(
    'Email: not configured'
  )
  expect(icon).toHaveAttribute('data-state', 'delayed-open')
  expect(icon).toHaveAttribute('aria-describedby')
})
