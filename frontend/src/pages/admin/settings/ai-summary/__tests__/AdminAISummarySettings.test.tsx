import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import {
  type AdminInstanceAISummarySettings,
  adminSettingsService,
} from '@/services/adminSettingsService'
import { ApiError } from '@/types/errors'

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/adminSettingsService', () => ({
  adminSettingsService: {
    getAISummarySettings: vi.fn(),
    updateAISummarySettings: vi.fn(),
    resetAISummarySettings: vi.fn(),
    listAISummarySettingsAudit: vi.fn(),
  },
}))

import { AdminAISummarySettings } from '../AdminAISummarySettings'

const service = vi.mocked(adminSettingsService)

const builtIn: AdminInstanceAISummarySettings['values'] = {
  enabled: true,
  top_n: 5,
  style: 'balanced',
  max_output_tokens: 800,
  per_document_chars: 8000,
  total_context_chars: 32000,
  request_timeout_ms: 60000,
}

const customized: AdminInstanceAISummarySettings = {
  values: { ...builtIn, style: 'detailed', top_n: 8 },
  source: 'instance',
  built_in_defaults: builtIn,
  limits: {
    top_n_min: 1,
    top_n_max: 10,
    max_output_tokens_min: 1,
    max_output_tokens_max: 32768,
    chars_min: 1,
    chars_max: 2147483647,
    request_timeout_ms_min: 1,
    request_timeout_ms_max: 2147483647,
  },
  teams_with_override: 3,
  updated_at: '2026-09-20T10:00:00Z',
  updated_by_user_id: null,
  updated_by_name: null,
  version: 2,
}

const saveButton = () => screen.getByRole('button', { name: 'Save changes' })

beforeEach(() => {
  vi.clearAllMocks()
  service.getAISummarySettings.mockResolvedValue(customized)
  service.listAISummarySettingsAudit.mockResolvedValue({
    entries: [],
    next_cursor: null,
  })
})

it('renders the team defaults and the server budgets group', async () => {
  render(<AdminAISummarySettings />)

  expect(await screen.findByLabelText('Style')).toHaveValue('detailed')
  expect(screen.getByLabelText('Results to read')).toHaveValue(8)
  expect(screen.getByLabelText('Enable AI Summary')).toBeChecked()
  expect(screen.getByText('Advanced / server budgets')).toBeInTheDocument()
  expect(
    screen.getByText(/including teams with their own AI\s+summary settings/)
  ).toBeInTheDocument()
  expect(screen.getByLabelText('Request timeout (ms)')).toHaveValue(60000)
  // No actor on the row (e.g. the boot-time import): time only.
  expect(screen.getByTestId('instance-settings-status')).toHaveTextContent(
    /Last changed at/
  )
  expect(screen.getByTestId('instance-settings-overrides')).toHaveTextContent(
    /3 teams override these settings\. .*server budgets still do/
  )
})

it('takes the input bounds from the response limits', async () => {
  render(<AdminAISummarySettings />)

  const topN = await screen.findByLabelText('Results to read')
  expect(topN).toHaveAttribute('max', '10')
  expect(screen.getByLabelText('Response length (tokens)')).toHaveAttribute(
    'max',
    '32768'
  )
})

it('saves every value with the last-read version', async () => {
  const user = userEvent.setup()
  service.updateAISummarySettings.mockResolvedValue({
    ...customized,
    values: { ...customized.values, style: 'concise' },
    version: 3,
  })
  render(<AdminAISummarySettings />)

  await user.selectOptions(await screen.findByLabelText('Style'), 'concise')
  const timeout = screen.getByLabelText('Request timeout (ms)')
  await user.clear(timeout)
  await user.type(timeout, '30000')
  await user.click(saveButton())

  expect(service.updateAISummarySettings).toHaveBeenCalledWith({
    ...customized.values,
    style: 'concise',
    request_timeout_ms: 30000,
    expected_version: 2,
  })
})

it('requires the total context to hold one document', async () => {
  const user = userEvent.setup()
  render(<AdminAISummarySettings />)

  const total = await screen.findByLabelText('Total context budget (chars)')
  await user.clear(total)
  await user.type(total, '100')

  expect(total).toHaveAccessibleDescription(
    /Must be at least the per-document budget/
  )
  expect(saveButton()).toBeDisabled()
})

it('puts a server error on the style select', async () => {
  const user = userEvent.setup()
  service.updateAISummarySettings.mockRejectedValue(
    new ApiError({
      type: 'about:blank',
      title: 'Bad Request',
      status: 400,
      detail: 'invalid',
      code: 'INSTANCE_SETTINGS_VALIDATION_FAILED',
      request_id: 'r1',
      timestamp: '2026-09-27T00:00:00Z',
      validation_errors: [{ field: 'style', message: 'style is unknown' }],
    } as ConstructorParameters<typeof ApiError>[0])
  )
  render(<AdminAISummarySettings />)

  await user.selectOptions(await screen.findByLabelText('Style'), 'concise')
  await user.click(saveButton())

  const style = screen.getByLabelText('Style')
  await waitFor(() => {
    expect(style).toHaveAttribute('aria-invalid', 'true')
  })
  expect(style).toHaveAccessibleDescription(/style is unknown/)
})

it('resets to the built-in defaults after previewing them', async () => {
  const user = userEvent.setup()
  service.resetAISummarySettings.mockResolvedValue(undefined)
  render(<AdminAISummarySettings />)

  await user.click(
    await screen.findByRole('button', { name: /reset to defaults/i })
  )
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog).toHaveTextContent(
    'enabled, 5 results, balanced, 800 tokens, 8000 chars per document, 32000 chars in total, 60000 ms timeout'
  )
  service.getAISummarySettings.mockResolvedValue({
    ...customized,
    source: 'default',
    values: builtIn,
    updated_at: null,
    version: null,
  })
  await user.click(
    within(dialog).getByRole('button', { name: 'Reset to defaults' })
  )

  await waitFor(() => {
    expect(screen.getByTestId('instance-settings-source')).toHaveTextContent(
      'Using built-in defaults'
    )
  })
  expect(screen.getByLabelText('Style')).toHaveValue('balanced')
})

it('lists AI summary changes in the history', async () => {
  service.listAISummarySettingsAudit.mockResolvedValue({
    entries: [
      {
        id: 'e1',
        setting: 'ai_summary',
        action: 'import',
        actor_user_id: null,
        actor_name: null,
        before: null,
        after: builtIn,
        created_at: '2026-09-20T10:00:00Z',
      },
    ],
    next_cursor: null,
  })
  render(<AdminAISummarySettings />)

  const entry = await screen.findByTestId('instance-ai-summary-audit-entry')
  expect(entry).toHaveTextContent('Imported from config.yaml')
  expect(entry).toHaveTextContent(/Request timeout \(ms\)\s*—\s*→\s*60000/)
})
