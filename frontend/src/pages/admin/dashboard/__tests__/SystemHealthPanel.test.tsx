/**
 * SystemHealthPanel (#1192): the "Instance email" card sits in its own row,
 * outside the overview's loading and error states.
 */
import { render, screen } from '@testing-library/react'

vi.mock('@/pages/admin/dashboard/InstanceEmailCard', () => ({
  InstanceEmailCard: function InstanceEmailCardStub() {
    return <div data-testid="instance-email-card" />
  },
}))

import { SystemHealthPanel } from '../SystemHealthPanel'

const health = {
  database_size_bytes: 1024,
  tables: [{ table: 'prompts', estimated_rows: 12 }],
}

it('renders the instance email card next to the database health', () => {
  render(<SystemHealthPanel health={health} version="1.2.3" loading={false} />)

  expect(screen.getByTestId('instance-email-card')).toBeInTheDocument()
  expect(screen.getByText('Database size')).toBeInTheDocument()
  expect(screen.getByText('Backend version 1.2.3')).toBeInTheDocument()
})

it('keeps the instance email card while the overview loads', () => {
  const { container } = render(
    <SystemHealthPanel health={null} version="unknown" loading />
  )

  expect(screen.getByTestId('instance-email-card')).toBeInTheDocument()
  expect(container.querySelectorAll('.animate-pulse')).toHaveLength(1)
  expect(screen.queryByText('Database size')).not.toBeInTheDocument()
})

it('keeps the instance email card when the overview has no health', () => {
  render(<SystemHealthPanel health={null} version="unknown" loading={false} />)

  expect(screen.getByTestId('instance-email-card')).toBeInTheDocument()
  expect(screen.queryByText('Database size')).not.toBeInTheDocument()
})
