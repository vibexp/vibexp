import { render, screen } from '@testing-library/react'

const mockGet = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getTeamEmailProvider: (...args: unknown[]) => mockGet(...args),
  },
}))

import { TeamEmailConfigTab } from '../detail/TeamEmailConfigTab'
import {
  emailConfig,
  expectNoSentinel,
  expectReadOnly,
  withSentinels,
} from './teamConfigFixtures'

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a loading skeleton while the request is pending', () => {
  mockGet.mockReturnValue(new Promise(() => {}))
  render(<TeamEmailConfigTab teamId="t1" />)
  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})

it('shows an error alert when the request fails', async () => {
  mockGet.mockRejectedValue(new Error('mail down'))
  render(<TeamEmailConfigTab teamId="t1" />)
  expect(
    await screen.findByText('Failed to load email provider')
  ).toBeInTheDocument()
})

it('renders the team provider without secrets or error text', async () => {
  mockGet.mockResolvedValue(
    withSentinels(
      emailConfig({
        settings: withSentinels({
          smtp: withSentinels({ host: 'smtp.example.com', port: '587' }),
        }),
      })
    )
  )
  render(<TeamEmailConfigTab teamId="t1" />)

  expect(await screen.findByText('smtp.example.com')).toBeInTheDocument()
  expect(mockGet).toHaveBeenCalledWith('t1')
  expect(screen.getByTestId('config-source')).toHaveTextContent('Team')
  expect(screen.getByText('587')).toBeInTheDocument()
  expect(screen.getByText('reply@example.com')).toBeInTheDocument()
  expect(screen.getByText('Configured ✓')).toBeInTheDocument()
  expect(screen.getByText('Healthy')).toBeInTheDocument()
  expect(screen.queryByText(/username/i)).not.toBeInTheDocument()
  expectReadOnly()
  expectNoSentinel()
})

it('shows only the effective from address when inherited', async () => {
  mockGet.mockResolvedValue(
    emailConfig({
      configured: false,
      source: 'instance',
      effective_from_address: 'noreply@instance.example.com',
      provider_type: null,
      from_address: null,
      from_name: null,
      reply_to: null,
      settings: null,
      has_secret: false,
      last_success_at: null,
      status: 'unknown',
    })
  )
  render(<TeamEmailConfigTab teamId="t1" />)

  expect(await screen.findByTestId('config-source')).toHaveTextContent(
    'Inherited from instance'
  )
  expect(screen.getByText('noreply@instance.example.com')).toBeInTheDocument()
  expect(screen.getByText('Unknown')).toBeInTheDocument()
  expect(screen.queryByText('Credential')).not.toBeInTheDocument()
})

it('renders a failing status with its timestamp, and Mailgun settings', async () => {
  mockGet.mockResolvedValue(
    emailConfig({
      provider_type: 'mailgun',
      settings: {
        mailgun: { domain: 'mg.example.com', base_url: null },
      },
      last_error_at: '2026-09-21T08:00:00Z',
      status: 'failing',
    })
  )
  render(<TeamEmailConfigTab teamId="t1" />)

  expect(await screen.findByText('Failing')).toBeInTheDocument()
  expect(screen.getByText('mg.example.com')).toBeInTheDocument()
  expect(screen.getByText(/September 21, 2026/)).toBeInTheDocument()
})

it('renders Postmark settings', async () => {
  mockGet.mockResolvedValue(
    emailConfig({
      provider_type: 'postmark',
      settings: { postmark: { message_stream: 'outbound' } },
    })
  )
  render(<TeamEmailConfigTab teamId="t1" />)
  expect(await screen.findByText('outbound')).toBeInTheDocument()
})
