import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { toast } from '@/lib/toast'
import {
  type AdminInstanceSearchSettings,
  adminSettingsService,
} from '@/services/adminSettingsService'
import { ApiError } from '@/types/errors'

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/adminSettingsService', () => ({
  adminSettingsService: {
    getSearchSettings: vi.fn(),
    updateSearchSettings: vi.fn(),
    resetSearchSettings: vi.fn(),
    listSearchSettingsAudit: vi.fn(),
  },
}))

import { AdminSearchSettings } from '../AdminSearchSettings'

const service = vi.mocked(adminSettingsService)

const builtIn: AdminInstanceSearchSettings['values'] = {
  recency_ranking_enabled: false,
  rank_weight_relevance: 1,
  rank_weight_created: 0,
  rank_weight_updated: 0,
  rank_half_life_days: 30,
  rank_candidate_cap: 500,
}

const defaults: AdminInstanceSearchSettings = {
  values: builtIn,
  source: 'default',
  built_in_defaults: builtIn,
  limits: {
    rank_weight_min: 0,
    rank_half_life_days_max: 36500,
    rank_candidate_cap_min: 1,
    rank_candidate_cap_max: 5000,
  },
  teams_with_override: 0,
  updated_at: null,
  updated_by_user_id: null,
  updated_by_name: null,
  version: null,
}

const customized: AdminInstanceSearchSettings = {
  ...defaults,
  source: 'instance',
  values: {
    ...builtIn,
    recency_ranking_enabled: true,
    rank_weight_created: 0.3,
  },
  teams_with_override: 2,
  updated_at: '2026-09-20T10:00:00Z',
  updated_by_user_id: '7d3c0b8e-8f7a-4a51-9a3e-1b2c3d4e5f60',
  updated_by_name: 'Ada Admin',
  version: 4,
}

const apiError = (
  status: number,
  code: string,
  validation_errors?: { field: string; message: string }[]
) =>
  new ApiError({
    type: 'about:blank',
    title: 'Error',
    status,
    detail: 'Request failed',
    code,
    request_id: 'req-1',
    timestamp: '2026-09-27T00:00:00Z',
    validation_errors,
  } as ConstructorParameters<typeof ApiError>[0])

const saveButton = () => screen.getByRole('button', { name: 'Save changes' })

beforeEach(() => {
  vi.clearAllMocks()
  service.getSearchSettings.mockResolvedValue(defaults)
  service.listSearchSettingsAudit.mockResolvedValue({
    entries: [],
    next_cursor: null,
  })
})

describe('AdminSearchSettings — status', () => {
  it('shows built-in defaults with no override note and no reset', async () => {
    render(<AdminSearchSettings />)

    expect(
      await screen.findByTestId('instance-settings-source')
    ).toHaveTextContent('Using built-in defaults')
    expect(screen.getByTestId('instance-settings-overrides')).toHaveTextContent(
      'No team overrides these settings'
    )
    expect(screen.queryByText(/last changed/i)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /reset to defaults/i })
    ).not.toBeInTheDocument()
    expect(screen.getByLabelText('Candidate cap')).toHaveValue(500)
    expect(saveButton()).toBeDisabled()
  })

  it('shows customized values, who changed them and the override count', async () => {
    service.getSearchSettings.mockResolvedValue(customized)
    render(<AdminSearchSettings />)

    const status = await screen.findByTestId('instance-settings-status')
    expect(within(status).getByText('Customized')).toBeInTheDocument()
    expect(status).toHaveTextContent(/last changed by ada admin at/i)
    expect(screen.getByTestId('instance-settings-overrides')).toHaveTextContent(
      /2 teams override these settings/
    )
    expect(screen.getByLabelText('Recency ranking')).toBeChecked()
    expect(screen.getByLabelText('Created-recency weight')).toHaveValue(0.3)
    expect(
      screen.getByRole('button', { name: /reset to defaults/i })
    ).toBeInTheDocument()
  })

  it('uses the singular for one overriding team', async () => {
    service.getSearchSettings.mockResolvedValue({
      ...customized,
      teams_with_override: 1,
    })
    render(<AdminSearchSettings />)

    expect(
      await screen.findByTestId('instance-settings-overrides')
    ).toHaveTextContent(/^1 team overrides these settings/)
  })

  it('shows a load failure', async () => {
    service.getSearchSettings.mockRejectedValue(new Error('backend down'))
    render(<AdminSearchSettings />)

    expect(await screen.findByText('backend down')).toBeInTheDocument()
  })
})

describe('AdminSearchSettings — saving', () => {
  it('sends the whole row with the last-read version', async () => {
    const user = userEvent.setup()
    service.getSearchSettings.mockResolvedValue(customized)
    service.updateSearchSettings.mockResolvedValue({
      ...customized,
      values: { ...customized.values, rank_candidate_cap: 800 },
      version: 5,
    })
    render(<AdminSearchSettings />)

    const cap = await screen.findByLabelText('Candidate cap')
    await user.clear(cap)
    await user.type(cap, '800')
    expect(screen.getByText('You have unsaved changes.')).toBeInTheDocument()
    await user.click(saveButton())

    expect(service.updateSearchSettings).toHaveBeenCalledWith({
      ...customized.values,
      rank_candidate_cap: 800,
      expected_version: 4,
    })
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith(
        'Instance search settings saved'
      )
    })
    // The history reloads to show the new entry.
    expect(service.listSearchSettingsAudit).toHaveBeenCalledTimes(2)
    expect(saveButton()).toBeDisabled()
  })

  it('sends version 0 when nothing is stored yet', async () => {
    const user = userEvent.setup()
    service.updateSearchSettings.mockResolvedValue(customized)
    render(<AdminSearchSettings />)

    await user.click(await screen.findByLabelText('Recency ranking'))
    await user.click(saveButton())

    expect(service.updateSearchSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        recency_ranking_enabled: true,
        expected_version: 0,
      })
    )
  })

  it('offers a reload when someone else saved first over the defaults', async () => {
    const user = userEvent.setup()
    service.updateSearchSettings.mockRejectedValue(
      apiError(409, 'INSTANCE_SETTINGS_VERSION_CONFLICT')
    )
    render(<AdminSearchSettings />)

    await user.click(await screen.findByLabelText('Recency ranking'))
    await user.click(saveButton())

    expect(
      await screen.findByTestId('instance-settings-conflict')
    ).toHaveTextContent(/someone else changed these settings/i)
    expect(service.updateSearchSettings).toHaveBeenCalledWith(
      expect.objectContaining({ expected_version: 0 })
    )
    expect(saveButton()).toBeDisabled()
  })

  it('blocks saving while a field is invalid and says why under it', async () => {
    const user = userEvent.setup()
    render(<AdminSearchSettings />)

    const halfLife = await screen.findByLabelText('Half-life (days)')
    await user.clear(halfLife)
    await user.type(halfLife, '0')

    expect(halfLife).toHaveAttribute('aria-invalid', 'true')
    expect(halfLife).toHaveAccessibleDescription(
      /Enter a number above 0 and at most 36500\./
    )
    expect(saveButton()).toBeDisabled()
  })

  it('flags all three weights when they are all zero', async () => {
    const user = userEvent.setup()
    render(<AdminSearchSettings />)

    const relevance = await screen.findByLabelText('Relevance weight')
    await user.clear(relevance)
    await user.type(relevance, '0')

    expect(
      screen.getAllByText('The three weights must not all be zero.')
    ).toHaveLength(3)
    expect(saveButton()).toBeDisabled()
  })

  it('puts server field errors under their fields until edited', async () => {
    const user = userEvent.setup()
    service.updateSearchSettings.mockRejectedValue(
      apiError(400, 'INSTANCE_SETTINGS_VALIDATION_FAILED', [
        { field: 'rank_candidate_cap', message: 'cap is too large' },
        { field: 'unknown_field', message: 'something else' },
      ])
    )
    render(<AdminSearchSettings />)

    const cap = await screen.findByLabelText('Candidate cap')
    await user.clear(cap)
    await user.type(cap, '600')
    await user.click(saveButton())

    expect(await screen.findByText('cap is too large')).toBeInTheDocument()
    expect(cap).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByTestId('instance-settings-error')).toHaveTextContent(
      'something else'
    )

    await user.type(cap, '1')
    expect(screen.queryByText('cap is too large')).not.toBeInTheDocument()
  })

  it('shows any other failure at form level', async () => {
    const user = userEvent.setup()
    service.updateSearchSettings.mockRejectedValue(new Error('boom'))
    render(<AdminSearchSettings />)

    await user.click(await screen.findByLabelText('Recency ranking'))
    await user.click(saveButton())

    expect(
      await screen.findByTestId('instance-settings-error')
    ).toHaveTextContent('boom')
  })

  it('on a version conflict, keeps the edits, blocks saving and offers a reload', async () => {
    const user = userEvent.setup()
    service.getSearchSettings.mockResolvedValue(customized)
    service.updateSearchSettings.mockRejectedValue(
      apiError(409, 'INSTANCE_SETTINGS_VERSION_CONFLICT')
    )
    render(<AdminSearchSettings />)

    const cap = await screen.findByLabelText('Candidate cap')
    await user.clear(cap)
    await user.type(cap, '900')
    await user.click(saveButton())

    const alert = await screen.findByTestId('instance-settings-conflict')
    expect(alert).toHaveTextContent(/someone else changed these settings/i)
    expect(cap).toHaveValue(900)
    expect(saveButton()).toBeDisabled()
    expect(service.updateSearchSettings).toHaveBeenCalledTimes(1)

    service.getSearchSettings.mockResolvedValue({
      ...customized,
      values: { ...customized.values, rank_candidate_cap: 700 },
      version: 5,
    })
    await user.click(within(alert).getByRole('button', { name: 'Reload' }))

    await waitFor(() => {
      expect(screen.getByLabelText('Candidate cap')).toHaveValue(700)
    })
    expect(
      screen.queryByTestId('instance-settings-conflict')
    ).not.toBeInTheDocument()
  })
})

describe('AdminSearchSettings — reset', () => {
  it('previews the built-in defaults and does nothing on cancel', async () => {
    const user = userEvent.setup()
    service.getSearchSettings.mockResolvedValue(customized)
    render(<AdminSearchSettings />)

    await user.click(
      await screen.findByRole('button', { name: /reset to defaults/i })
    )
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent(
      /recency ranking off, weights 1 \/ 0 \/ 0 .* half-life 30 days, candidate cap 500/
    )
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    expect(service.resetSearchSettings).not.toHaveBeenCalled()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('resets on confirm and shows the built-in defaults', async () => {
    const user = userEvent.setup()
    service.getSearchSettings.mockResolvedValue(customized)
    service.resetSearchSettings.mockResolvedValue(undefined)
    render(<AdminSearchSettings />)

    await user.click(
      await screen.findByRole('button', { name: /reset to defaults/i })
    )
    service.getSearchSettings.mockResolvedValue(defaults)
    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Reset to defaults' })
    )

    await waitFor(() => {
      expect(screen.getByTestId('instance-settings-source')).toHaveTextContent(
        'Using built-in defaults'
      )
    })
    expect(service.resetSearchSettings).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Recency ranking')).not.toBeChecked()
    expect(service.listSearchSettingsAudit).toHaveBeenCalledTimes(2)
  })

  it('shows a failed reset at form level and keeps the values', async () => {
    const user = userEvent.setup()
    service.getSearchSettings.mockResolvedValue(customized)
    service.resetSearchSettings.mockRejectedValue(new Error('reset failed'))
    render(<AdminSearchSettings />)

    await user.click(
      await screen.findByRole('button', { name: /reset to defaults/i })
    )
    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Reset to defaults' })
    )

    expect(
      await screen.findByTestId('instance-settings-error')
    ).toHaveTextContent('reset failed')
    expect(screen.getByTestId('instance-settings-source')).toHaveTextContent(
      'Customized'
    )
  })

  it('warns when the refresh after a reset fails', async () => {
    const user = userEvent.setup()
    service.getSearchSettings.mockResolvedValue(customized)
    service.resetSearchSettings.mockResolvedValue(undefined)
    render(<AdminSearchSettings />)

    await user.click(
      await screen.findByRole('button', { name: /reset to defaults/i })
    )
    service.getSearchSettings.mockRejectedValue(new Error('refresh failed'))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Reset to defaults' })
    )

    expect(
      await screen.findByTestId('instance-settings-load-error')
    ).toHaveTextContent(/refresh failed.*may be out of date/)
  })
})

describe('AdminSearchSettings — history', () => {
  it('lists the changed fields, labels a delete as a reset, and pages', async () => {
    const user = userEvent.setup()
    service.listSearchSettingsAudit
      .mockResolvedValueOnce({
        entries: [
          {
            id: 'e1',
            setting: 'search',
            action: 'upsert',
            actor_user_id: 'u1',
            actor_name: 'Ada Admin',
            before: { ...builtIn, version: 1 },
            after: { ...builtIn, rank_candidate_cap: 800, version: 2 },
            created_at: '2026-09-20T10:00:00Z',
          },
        ],
        next_cursor: 'c2',
      })
      .mockResolvedValueOnce({
        entries: [
          {
            id: 'e2',
            setting: 'search',
            action: 'delete',
            actor_user_id: 'u1',
            actor_name: 'Ada Admin',
            before: builtIn,
            after: null,
            created_at: '2026-09-19T10:00:00Z',
          },
        ],
        next_cursor: null,
      })
    render(<AdminSearchSettings />)

    const first = await screen.findByTestId('instance-search-audit-entry')
    expect(first).toHaveTextContent('Saved')
    expect(first).toHaveTextContent(/Candidate cap\s*500\s*→\s*800/)
    expect(first).not.toHaveTextContent('version')

    await user.click(screen.getByRole('button', { name: 'Load more' }))

    const entries = await screen.findAllByTestId('instance-search-audit-entry')
    expect(entries).toHaveLength(2)
    expect(entries[1]).toHaveTextContent('Reset to defaults')
    expect(service.listSearchSettingsAudit).toHaveBeenLastCalledWith({
      limit: 20,
      cursor: 'c2',
    })
  })
})
