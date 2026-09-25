/**
 * AdminExportButton (#1150): one download per export, a toast that says how
 * much arrived, and no second request while one is running.
 */
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
}))

const mockDownloadBlob = vi.hoisted(() => vi.fn())
vi.mock('@/utils/downloadBlob', () => ({ downloadBlob: mockDownloadBlob }))

import { toast } from 'sonner'

import type { AdminCsvExport } from '@/services/adminService'
import { ApiError } from '@/types/errors'

import { AdminExportButton } from '../AdminExportButton'

const blob = new Blob(['id\r\n'], { type: 'text/csv' })

function csvExport(overrides: Partial<AdminCsvExport> = {}): AdminCsvExport {
  return {
    blob,
    filename: 'admin-users-20260925.csv',
    totalCount: 1234,
    truncated: false,
    ...overrides,
  }
}

const button = () => screen.getByRole('button', { name: /export csv/i })

beforeEach(() => {
  vi.clearAllMocks()
})

it('downloads the file and reports the row count', async () => {
  const onExport = vi.fn(() => Promise.resolve(csvExport()))
  render(<AdminExportButton noun="users" onExport={onExport} />)

  await userEvent.click(button())

  await waitFor(() => {
    expect(toast.success).toHaveBeenCalledWith(
      `Exported ${(1234).toLocaleString()} users`
    )
  })
  expect(mockDownloadBlob).toHaveBeenCalledTimes(1)
  expect(mockDownloadBlob).toHaveBeenCalledWith(
    blob,
    'admin-users-20260925.csv'
  )
  expect(toast.warning).not.toHaveBeenCalled()
})

it('warns with both numbers when the export was capped', async () => {
  const onExport = vi.fn(() =>
    Promise.resolve(csvExport({ totalCount: 73210, truncated: true }))
  )
  render(<AdminExportButton noun="teams" onExport={onExport} />)

  await userEvent.click(button())

  await waitFor(() => {
    expect(toast.warning).toHaveBeenCalledTimes(1)
  })
  const [message] = vi.mocked(toast.warning).mock.calls[0]
  expect(message).toContain((50000).toLocaleString())
  expect(message).toContain((73210).toLocaleString())
  expect(message).toContain('narrow the filters')
  expect(toast.success).not.toHaveBeenCalled()
  expect(mockDownloadBlob).toHaveBeenCalledTimes(1)
})

it('shows the error and downloads nothing when the export fails', async () => {
  const onExport = vi.fn(() =>
    Promise.reject(
      new ApiError({
        type: 'about:blank',
        title: 'Bad Request',
        status: 400,
        detail: 'owner_email must be a bare email address',
        code: 'VALIDATION_ERROR',
        request_id: 'r1',
        timestamp: '2026-09-25T10:00:00Z',
      })
    )
  )
  render(<AdminExportButton noun="projects" onExport={onExport} />)

  await userEvent.click(button())

  await waitFor(() => {
    expect(toast.error).toHaveBeenCalledWith(
      'owner_email must be a bare email address'
    )
  })
  expect(mockDownloadBlob).not.toHaveBeenCalled()
  // Usable again after a failure.
  expect(button()).toBeEnabled()
})

it('is disabled while an export runs and ignores a second click', async () => {
  let resolve: (value: AdminCsvExport) => void = () => undefined
  const onExport = vi.fn(
    () =>
      new Promise<AdminCsvExport>(res => {
        resolve = res
      })
  )
  render(<AdminExportButton noun="users" onExport={onExport} />)

  const user = userEvent.setup()
  await user.click(button())
  expect(button()).toBeDisabled()
  await user.click(button())
  expect(onExport).toHaveBeenCalledTimes(1)

  resolve(csvExport())
  await waitFor(() => {
    expect(button()).toBeEnabled()
  })
  expect(mockDownloadBlob).toHaveBeenCalledTimes(1)
})

it('does not start a second export on a double click before re-render', () => {
  const onExport = vi.fn(() => new Promise<AdminCsvExport>(() => undefined))
  render(<AdminExportButton noun="users" onExport={onExport} />)

  // Two native clicks in the same task, before React re-renders the button.
  const el = button()
  el.click()
  el.click()

  expect(onExport).toHaveBeenCalledTimes(1)
})

it('is disabled when there is nothing to export', () => {
  const onExport = vi.fn(() => Promise.resolve(csvExport()))
  render(<AdminExportButton noun="users" onExport={onExport} disabled />)

  expect(button()).toBeDisabled()
})
