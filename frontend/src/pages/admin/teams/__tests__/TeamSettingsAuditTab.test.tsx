import { fireEvent, render, screen } from '@testing-library/react'

const mockList = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    listTeamSettingsAudit: (...args: unknown[]) => mockList(...args),
  },
}))

import { TeamSettingsAuditTab } from '../detail/TeamSettingsAuditTab'
import {
  auditPage,
  expectNoSentinel,
  expectReadOnly,
  withSentinels,
} from './teamConfigFixtures'

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a loading skeleton while the request is pending', () => {
  mockList.mockReturnValue(new Promise(() => {}))
  render(<TeamSettingsAuditTab teamId="t1" />)
  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})

it('shows an error alert when the request fails', async () => {
  mockList.mockRejectedValue(new Error('audit down'))
  render(<TeamSettingsAuditTab teamId="t1" />)
  expect(
    await screen.findByText('Failed to load the settings audit log')
  ).toBeInTheDocument()
})

it('shows the empty state', async () => {
  mockList.mockResolvedValue(
    auditPage({ entries: [], total_count: 0, total_pages: 0 })
  )
  render(<TeamSettingsAuditTab teamId="t1" />)
  expect(await screen.findByTestId('config-empty')).toBeInTheDocument()
})

it('renders allowlisted detail fields only, never raw detail', async () => {
  const page = auditPage()
  mockList.mockResolvedValue(
    withSentinels({
      ...page,
      entries: page.entries.map(entry =>
        withSentinels({ ...entry, detail: withSentinels(entry.detail) })
      ),
    })
  )
  render(<TeamSettingsAuditTab teamId="t1" />)

  expect(await screen.findByText('Copied OpenAI')).toBeInTheDocument()
  expect(mockList).toHaveBeenCalledWith('t1', { page: 1, limit: 20 })
  expect(screen.getAllByTestId('settings-audit-row')).toHaveLength(2)
  expect(screen.getByText('Model provider')).toBeInTheDocument()
  expect(screen.getByText('Platform')).toBeInTheDocument()
  expect(screen.getByText('Included an API key')).toBeInTheDocument()
  expect(screen.getByText('2 types: runbook, adr')).toBeInTheDocument()
  expect(screen.getByText('Deleted team 9b8c7d6e')).toBeInTheDocument()
  expect(screen.getByText('Deleted user')).toBeInTheDocument()
  // Previous + Next are the only controls.
  expectReadOnly(2)
  expectNoSentinel()
})

it('pages with Next and Previous', async () => {
  mockList.mockResolvedValue(auditPage())
  render(<TeamSettingsAuditTab teamId="t1" />)

  const prev = await screen.findByRole('button', { name: 'Previous' })
  expect(prev).toBeDisabled()

  fireEvent.click(screen.getByRole('button', { name: 'Next' }))
  expect(await screen.findByText(/Page 2 of 3/)).toBeInTheDocument()
  expect(mockList).toHaveBeenLastCalledWith('t1', { page: 2, limit: 20 })

  fireEvent.click(screen.getByRole('button', { name: 'Previous' }))
  expect(await screen.findByText(/Page 1 of 3/)).toBeInTheDocument()
  expect(mockList).toHaveBeenLastCalledWith('t1', { page: 1, limit: 20 })
})

it('disables Next on the last page', async () => {
  mockList.mockResolvedValue(auditPage({ total_pages: 1 }))
  render(<TeamSettingsAuditTab teamId="t1" />)
  expect(await screen.findByRole('button', { name: 'Next' })).toBeDisabled()
})
