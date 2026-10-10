import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { toast } from '@/lib/toast'
import {
  type AdminAuthAllowlist,
  authSettingsService,
} from '@/services/authSettingsService'

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/authSettingsService', () => ({
  authSettingsService: {
    getAllowlist: vi.fn(),
    updateAllowlist: vi.fn(),
    resetAllowlist: vi.fn(),
    previewAllowlist: vi.fn(),
  },
}))

import { AllowlistSection } from '../AllowlistSection'
import { allowlist, apiError, versionConflict } from './fixtures'

const service = vi.mocked(authSettingsService)
const onSaved = vi.fn()

const stored = allowlist({
  domains: ['example.com'],
  emails: ['contractor@partner.example'],
  active: true,
  version: 3,
  updated_at: '2026-10-01T10:00:00Z',
})

const nobody = { count: 0, sample: [], sample_truncated: false }

async function renderSection(initial: AdminAuthAllowlist) {
  service.getAllowlist.mockResolvedValue(initial)
  render(<AllowlistSection onSaved={onSaved} />)
  await screen.findByTestId('auth-allowlist-section')
  return userEvent.setup()
}

const save = () => screen.getByRole('button', { name: 'Save allowlist' })

beforeEach(() => {
  vi.clearAllMocks()
})

describe('AllowlistSection', () => {
  it('shows open access while nothing is stored, with nothing to reset', async () => {
    await renderSection(allowlist())
    expect(screen.getByTestId('allowlist-status')).toHaveTextContent(
      'Open access'
    )
    expect(
      screen.queryByRole('button', { name: 'Reset to open access' })
    ).toBeNull()
    expect(save()).toBeDisabled()
  })

  it('shows the stored lists as chips', async () => {
    await renderSection(stored)
    expect(screen.getByTestId('allowlist-status')).toHaveTextContent(
      'Restricted'
    )
    expect(screen.getByText('example.com')).toBeVisible()
    expect(screen.getByText('contractor@partner.example')).toBeVisible()
  })

  it('saves directly when the preview signs nobody out', async () => {
    service.previewAllowlist.mockResolvedValue(nobody)
    service.updateAllowlist.mockResolvedValue(
      allowlist({ domains: ['example.com'], active: true, version: 1 })
    )
    const user = await renderSection(allowlist())
    await user.type(screen.getByLabelText('Allowed domains'), 'Example.com,')
    await user.click(save())

    await waitFor(() => {
      expect(service.updateAllowlist).toHaveBeenCalledTimes(1)
    })
    // The preview ran first, on exactly what is then saved.
    expect(service.previewAllowlist).toHaveBeenCalledWith({
      domains: ['example.com'],
      emails: [],
    })
    expect(service.previewAllowlist.mock.invocationCallOrder[0]).toBeLessThan(
      service.updateAllowlist.mock.invocationCallOrder[0]
    )
    // Nothing is stored yet, so the first save is checked against version 0.
    expect(service.updateAllowlist).toHaveBeenCalledWith({
      domains: ['example.com'],
      emails: [],
      expected_version: 0,
    })
    expect(screen.queryByRole('alertdialog')).toBeNull()
    await waitFor(() => {
      expect(screen.getByTestId('allowlist-status')).toHaveTextContent(
        'Restricted'
      )
    })
    expect(toast.success).toHaveBeenCalledWith('Access allowlist saved')
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  it('asks before signing users out, listing the sample', async () => {
    service.previewAllowlist.mockResolvedValue({
      count: 25,
      sample: ['ada@other.example', 'bob@other.example'],
      sample_truncated: true,
    })
    service.updateAllowlist.mockResolvedValue({ ...stored, version: 4 })
    const user = await renderSection(stored)
    await user.click(
      screen.getByRole('button', { name: 'Remove contractor@partner.example' })
    )
    await user.click(save())

    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('25 signed-in users will be signed out')
    const impact = within(dialog).getByTestId('allowlist-impact')
    expect(impact).toHaveTextContent('ada@other.example, bob@other.example')
    expect(impact).toHaveTextContent('and more')
    expect(service.updateAllowlist).not.toHaveBeenCalled()

    await user.click(
      within(dialog).getByRole('button', { name: 'Save and sign them out' })
    )
    await waitFor(() => {
      expect(service.updateAllowlist).toHaveBeenCalledWith({
        domains: ['example.com'],
        emails: [],
        expected_version: 3,
      })
    })
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).toBeNull()
    })
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  it('names one user in the singular, with no "and more" for a full sample', async () => {
    service.previewAllowlist.mockResolvedValue({
      count: 1,
      sample: ['ada@other.example'],
      sample_truncated: false,
    })
    const user = await renderSection(allowlist())
    await user.type(screen.getByLabelText('Allowed domains'), 'example.com,')
    await user.click(save())

    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('1 signed-in user will be signed out')
    expect(dialog).not.toHaveTextContent('and more')
  })

  it('saves nothing when the admin cancels the confirmation', async () => {
    service.previewAllowlist.mockResolvedValue({
      count: 2,
      sample: ['ada@other.example', 'bob@other.example'],
      sample_truncated: false,
    })
    const user = await renderSection(allowlist())
    await user.type(screen.getByLabelText('Allowed domains'), 'example.com,')
    await user.click(save())
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).toBeNull()
    })
    expect(service.updateAllowlist).not.toHaveBeenCalled()
    expect(onSaved).not.toHaveBeenCalled()
    // The edit is still there to save.
    expect(save()).toBeEnabled()
  })

  it('blocks an invalid entry before any request', async () => {
    const user = await renderSection(allowlist())
    await user.type(screen.getByLabelText('Allowed domains'), '@nope,')
    expect(screen.getByText(/"@nope" is not a domain/)).toBeVisible()
    expect(screen.getByLabelText('Allowed domains')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(save()).toBeDisabled()

    await user.type(
      screen.getByLabelText('Allowed email addresses'),
      'not-an-email,'
    )
    expect(
      screen.getByText(/"not-an-email" is not a full email address/)
    ).toBeVisible()
    expect(service.previewAllowlist).not.toHaveBeenCalled()
  })

  it('shows the reload prompt on a version conflict and never retries', async () => {
    service.previewAllowlist.mockResolvedValue(nobody)
    service.updateAllowlist.mockRejectedValue(versionConflict())
    const user = await renderSection(stored)
    await user.type(screen.getByLabelText('Allowed domains'), 'other.io,')
    await user.click(save())

    const prompt = await screen.findByTestId('allowlist-conflict')
    expect(service.updateAllowlist).toHaveBeenCalledTimes(1)
    expect(save()).toBeDisabled()
    expect(onSaved).not.toHaveBeenCalled()

    service.getAllowlist.mockResolvedValue({ ...stored, version: 9 })
    await user.click(within(prompt).getByRole('button', { name: 'Reload' }))
    await waitFor(() => {
      expect(screen.queryByTestId('allowlist-conflict')).toBeNull()
    })
    // The unsaved edit is discarded with the reload.
    expect(screen.queryByText('other.io')).toBeNull()
    expect(service.updateAllowlist).toHaveBeenCalledTimes(1)
  })

  it('puts a server validation error on its list', async () => {
    service.previewAllowlist.mockRejectedValue(
      apiError(400, 'VALIDATION_FAILED', {
        validation_errors: [
          { field: 'domains', message: 'domains contains an invalid entry' },
        ],
      })
    )
    const user = await renderSection(allowlist())
    await user.type(screen.getByLabelText('Allowed domains'), 'example.com,')
    await user.click(save())

    expect(
      await screen.findByText('domains contains an invalid entry')
    ).toBeVisible()
    expect(service.updateAllowlist).not.toHaveBeenCalled()
  })

  it('shows any other save failure', async () => {
    service.previewAllowlist.mockResolvedValue(nobody)
    service.updateAllowlist.mockRejectedValue(new Error('database down'))
    const user = await renderSection(allowlist())
    await user.type(screen.getByLabelText('Allowed domains'), 'example.com,')
    await user.click(save())
    expect(await screen.findByTestId('allowlist-error')).toHaveTextContent(
      'database down'
    )
  })

  it('resets to open access after a confirmation', async () => {
    service.resetAllowlist.mockResolvedValue(undefined)
    const user = await renderSection(stored)
    await user.click(
      screen.getByRole('button', { name: 'Reset to open access' })
    )
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('Reset to open access?')
    expect(service.resetAllowlist).not.toHaveBeenCalled()

    service.getAllowlist.mockResolvedValue(allowlist())
    await user.click(
      within(dialog).getByRole('button', { name: 'Reset to open access' })
    )
    await waitFor(() => {
      expect(screen.getByTestId('allowlist-status')).toHaveTextContent(
        'Open access'
      )
    })
    expect(service.resetAllowlist).toHaveBeenCalledTimes(1)
    expect(service.updateAllowlist).not.toHaveBeenCalled()
    expect(onSaved).toHaveBeenCalledTimes(1)
    expect(toast.success).toHaveBeenCalledWith(
      'Access allowlist reset to open access'
    )
  })

  it('reports a reset that failed', async () => {
    service.resetAllowlist.mockRejectedValue(new Error('reset failed'))
    const user = await renderSection(stored)
    await user.click(
      screen.getByRole('button', { name: 'Reset to open access' })
    )
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Reset to open access',
      })
    )
    expect(await screen.findByTestId('allowlist-error')).toHaveTextContent(
      'reset failed'
    )
    expect(onSaved).not.toHaveBeenCalled()
  })

  it('shows a load failure', async () => {
    service.getAllowlist.mockRejectedValue(new Error('boom'))
    render(<AllowlistSection />)
    expect(await screen.findByText('boom')).toBeVisible()
    expect(screen.queryByTestId('auth-allowlist-section')).toBeNull()
  })
})
