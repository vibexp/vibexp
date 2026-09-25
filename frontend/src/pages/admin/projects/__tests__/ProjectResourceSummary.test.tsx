import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { AdminProjectResourceCounts } from '@/services/adminService'

import { ProjectResourceSummary } from '../ProjectResourceSummary'

// Distinct per type, so a transposed breakdown cannot pass unnoticed.
const COUNTS: AdminProjectResourceCounts = {
  prompts: 3,
  memories: 7,
  artifacts: 2,
  blueprints: 1,
  feed_items: 5,
  total: 18,
}

it('shows the total and names the breakdown as accessible text', () => {
  render(<ProjectResourceSummary counts={COUNTS} />)

  const total = screen.getByRole('img', {
    name: '18 resources: 3 prompts, 7 memories, 2 artifacts, 1 blueprint, 5 feed items',
  })
  expect(total).toHaveTextContent('18')
})

it('uses the singular for a count of one', () => {
  render(
    <ProjectResourceSummary
      counts={{
        prompts: 1,
        memories: 0,
        artifacts: 0,
        blueprints: 0,
        feed_items: 0,
        total: 1,
      }}
    />
  )

  expect(
    screen.getByRole('img', {
      name: '1 resource: 1 prompt, 0 memories, 0 artifacts, 0 blueprints, 0 feed items',
    })
  ).toBeInTheDocument()
})

it('lists only the project-scoped types, counts only, in the tooltip', async () => {
  render(<ProjectResourceSummary counts={COUNTS} />)

  await userEvent.hover(screen.getByRole('img'))

  const tooltip = await screen.findByRole('tooltip')
  // Radix renders the content twice (visible + a11y copy); the text is the same.
  expect(tooltip).toHaveTextContent(
    'Prompts3Memories7Artifacts2Blueprints1Feed items5'
  )
  for (const absent of ['Agents', 'Feeds', 'Comments', 'Attachments']) {
    expect(tooltip).not.toHaveTextContent(new RegExp(`${absent}\\b`))
  }
})

it('anchors the tooltip on the real count box (#891)', async () => {
  render(<ProjectResourceSummary counts={COUNTS} />)
  const total = screen.getByRole('img')

  // The Radix trigger props must land on the span itself — a box floating-ui can
  // measure — not on a `display: contents` wrapper. jsdom has no layout, so the
  // geometry belongs to Playwright (#1151); this pins the structure.
  expect(total).toHaveClass('inline-flex')
  expect(total).not.toHaveClass('contents')
  expect(total).toHaveAttribute('data-state', 'closed')

  await userEvent.hover(total)

  await screen.findByRole('tooltip')
  expect(total).toHaveAttribute('data-state', 'delayed-open')
  expect(total).toHaveAttribute('aria-describedby')
})
