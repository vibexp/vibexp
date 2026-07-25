import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { githubAppConfigService } from '@/services/githubAppConfigService'

import { GitHubAppPostSaveDialog } from './GitHubAppPostSaveDialog'

jest.mock('@/services/githubAppConfigService', () => ({
  ...jest.requireActual('@/services/githubAppConfigService'),
  githubAppConfigService: { validateAppConfig: jest.fn() },
}))

jest.mock('@/lib/toast', () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}))

const mockedValidate = githubAppConfigService.validateAppConfig as jest.Mock

const WEBHOOK_URL = 'https://vibexp.example.com/api/v1/webhooks/github/tok3n'
const SECRET = 'whsec-generated-value'

beforeEach(() => {
  jest.clearAllMocks()
})

const renderDialog = (
  props: Partial<React.ComponentProps<typeof GitHubAppPostSaveDialog>> = {}
) =>
  render(
    <GitHubAppPostSaveDialog
      open
      onOpenChange={jest.fn()}
      teamId="team-1"
      webhookUrl={WEBHOOK_URL}
      webhookSecret={SECRET}
      {...props}
    />
  )

describe('GitHubAppPostSaveDialog', () => {
  it('hands over both values an admin must carry to GitHub', () => {
    renderDialog()

    expect(screen.getByText(WEBHOOK_URL)).toBeVisible()
    expect(screen.getByText(SECRET)).toBeVisible()
  })

  it('says plainly that the secret will not be shown again', () => {
    renderDialog()

    expect(screen.getByText('This secret is shown once')).toBeVisible()
  })

  it('omits the secret block when there is no secret to disclose', () => {
    // The rotation path re-issues only the URL — showing an empty "secret"
    // section would imply a secret was reissued when it was not.
    renderDialog({ webhookSecret: undefined })

    expect(screen.getByText(WEBHOOK_URL)).toBeVisible()
    expect(screen.queryByText('This secret is shown once')).toBeNull()
  })

  it('copies the webhook URL to the clipboard', async () => {
    // userEvent installs its own clipboard stub, so read it back rather than
    // replacing navigator.clipboard — which it defines as getter-only.
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Copy Webhook URL' }))

    await waitFor(async () => {
      expect(await navigator.clipboard.readText()).toBe(WEBHOOK_URL)
    })
  })

  it('reports a verified App', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'All good' })
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Verify' }))

    expect(await screen.findByText('Verified')).toBeVisible()
    expect(screen.getByText('All good')).toBeVisible()
  })

  it('turns a failed probe into an actionable instruction', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({
      is_valid: false,
      message: 'validation failed',
      details: { error_details: 'insufficient_permissions' },
    })
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Verify' }))

    // Not the server's generic wording — the category mapped to what to do.
    expect(
      await screen.findByText('The App is missing a required permission')
    ).toBeVisible()
    expect(screen.getByText(/Grant Contents \(read-only\)/)).toBeVisible()
  })

  it('surfaces a failed probe call itself', async () => {
    const user = userEvent.setup()
    mockedValidate.mockRejectedValue(new Error('network'))
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Verify' }))

    const { toast } = jest.requireMock('@/lib/toast')
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        'Could not verify the GitHub App'
      )
    })
  })
})
