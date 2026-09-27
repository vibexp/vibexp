import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { AdminInstanceSettingsAuditEntry } from '@/services/adminService'
import { adminService } from '@/services/adminService'

vi.mock('@/services/adminService', () => ({
  adminService: { listInstanceEmailSettingsAudit: vi.fn() },
}))

import { InstanceEmailAuditList } from '../InstanceEmailAuditList'

const list = vi.mocked(adminService.listInstanceEmailSettingsAudit)

const entry = (
  overrides: Partial<AdminInstanceSettingsAuditEntry>
): AdminInstanceSettingsAuditEntry => ({
  id: 'e-1',
  setting: 'email_provider',
  action: 'upsert',
  actor_user_id: 'u-1',
  actor_name: 'Ada Admin',
  before: null,
  after: null,
  created_at: '2026-09-20T10:00:00Z',
  ...overrides,
})

const smtp = { provider_type: 'smtp', settings: { host: 'mailpit' } }

const upsert = entry({
  id: 'e-upsert',
  before: { ...smtp, from_address: 'old@acme.test' },
  after: { ...smtp, from_address: 'new@acme.test', secret: 'changed' },
})
const imported = entry({
  id: 'e-import',
  action: 'import',
  actor_user_id: null,
  actor_name: null,
  after: { ...smtp, from_address: 'boot@acme.test' },
})
const removed = entry({
  id: 'e-delete',
  action: 'delete',
  before: { ...smtp, from_address: 'new@acme.test' },
})

beforeEach(() => {
  vi.clearAllMocks()
})

describe('InstanceEmailAuditList', () => {
  it('renders an upsert with its actor, diff and credential marker', async () => {
    list.mockResolvedValue({ entries: [upsert], next_cursor: null })
    render(<InstanceEmailAuditList refreshKey={0} />)

    const row = await screen.findByTestId('instance-email-audit-entry')
    expect(within(row).getByText('Saved')).toBeInTheDocument()
    expect(within(row).getByText('Ada Admin')).toBeInTheDocument()
    expect(within(row).getByText('Credential changed')).toBeInTheDocument()
    expect(within(row).getByText('From address')).toBeInTheDocument()
    expect(within(row).getByText('old@acme.test')).toBeInTheDocument()
    expect(row).toHaveTextContent('→ new@acme.test')
    expect(list).toHaveBeenCalledWith({ limit: 20 })
  })

  it('renders an import as coming from config.yaml', async () => {
    list.mockResolvedValue({ entries: [imported], next_cursor: null })
    render(<InstanceEmailAuditList refreshKey={0} />)

    const row = await screen.findByTestId('instance-email-audit-entry')
    expect(within(row).getByText('Imported')).toBeInTheDocument()
    expect(
      within(row).getByText('Imported from config.yaml')
    ).toBeInTheDocument()
    expect(row).toHaveTextContent('→ boot@acme.test')
  })

  it('renders a delete', async () => {
    list.mockResolvedValue({ entries: [removed], next_cursor: null })
    render(<InstanceEmailAuditList refreshKey={0} />)

    const row = await screen.findByTestId('instance-email-audit-entry')
    expect(within(row).getByText('Removed')).toBeInTheDocument()
    expect(row).toHaveTextContent('new@acme.test → —')
  })

  it('says so when nothing is recorded', async () => {
    list.mockResolvedValue({ entries: [], next_cursor: null })
    render(<InstanceEmailAuditList refreshKey={0} />)

    expect(
      await screen.findByText(/no changes have been recorded yet/i)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /load more/i })
    ).not.toBeInTheDocument()
  })

  it('appends the next page on "Load more" and stops at the last', async () => {
    const user = userEvent.setup()
    list
      .mockResolvedValueOnce({ entries: [upsert], next_cursor: 'c-2' })
      .mockResolvedValueOnce({ entries: [imported], next_cursor: null })
    render(<InstanceEmailAuditList refreshKey={0} />)

    await user.click(await screen.findByRole('button', { name: /load more/i }))

    await waitFor(() => {
      expect(screen.getAllByTestId('instance-email-audit-entry')).toHaveLength(
        2
      )
    })
    expect(list).toHaveBeenLastCalledWith({ limit: 20, cursor: 'c-2' })
    expect(
      screen.queryByRole('button', { name: /load more/i })
    ).not.toBeInTheDocument()
  })

  it('keeps the loaded entries and reports a failed next page', async () => {
    const user = userEvent.setup()
    list
      .mockResolvedValueOnce({ entries: [upsert], next_cursor: 'c-2' })
      .mockRejectedValueOnce(new Error('page failed'))
    render(<InstanceEmailAuditList refreshKey={0} />)

    await user.click(await screen.findByRole('button', { name: /load more/i }))

    expect(await screen.findByText('page failed')).toBeInTheDocument()
    expect(screen.getAllByTestId('instance-email-audit-entry')).toHaveLength(1)
    expect(screen.getByRole('button', { name: /load more/i })).toBeEnabled()
  })

  it('reports a failed first page', async () => {
    list.mockRejectedValue(new Error('audit down'))
    render(<InstanceEmailAuditList refreshKey={0} />)

    expect(await screen.findByText('audit down')).toBeInTheDocument()
  })

  it('reloads from the first page when refreshKey changes', async () => {
    list.mockResolvedValue({ entries: [upsert], next_cursor: null })
    const { rerender } = render(<InstanceEmailAuditList refreshKey={0} />)
    await screen.findByTestId('instance-email-audit-entry')

    list.mockResolvedValue({ entries: [removed, upsert], next_cursor: null })
    rerender(<InstanceEmailAuditList refreshKey={1} />)

    await waitFor(() => {
      expect(screen.getAllByTestId('instance-email-audit-entry')).toHaveLength(
        2
      )
    })
    expect(list).toHaveBeenCalledTimes(2)
  })
})
