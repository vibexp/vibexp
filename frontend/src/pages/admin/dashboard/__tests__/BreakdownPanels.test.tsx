import { render, screen } from '@testing-library/react'

import { BreakdownPanels } from '@/pages/admin/dashboard/BreakdownPanels'
import type { AdminEntityBreakdown } from '@/services/adminService'

const breakdown = (entity: string, field: string): AdminEntityBreakdown => ({
  entity,
  field,
  buckets: [{ value: 'draft', count: 1 }],
})

it('titles a panel as "<Entity> by <field>"', () => {
  render(
    <BreakdownPanels
      breakdowns={[breakdown('prompts', 'status')]}
      loading={false}
    />
  )

  expect(screen.getByText('Prompts by status')).toBeInTheDocument()
})

it('spaces every underscore in a multi-underscore field', () => {
  render(
    <BreakdownPanels
      breakdowns={[breakdown('artifacts', 'content_mime_type')]}
      loading={false}
    />
  )

  expect(screen.getByText('Artifacts by content mime type')).toBeInTheDocument()
})
