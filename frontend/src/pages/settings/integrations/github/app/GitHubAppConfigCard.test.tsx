import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { GitHubAppConfigResponse } from '@/services/githubAppConfigService'
import { githubAppConfigService } from '@/services/githubAppConfigService'

import { GitHubAppConfigCard } from './GitHubAppConfigCard'

jest.mock('@/services/githubAppConfigService', () => ({
  ...jest.requireActual('@/services/githubAppConfigService'),
  githubAppConfigService: {
    validateAppConfig: jest.fn(),
    rotateWebhookToken: jest.fn(),
    deleteAppConfig: jest.fn(),
  },
}))

jest.mock('@/lib/toast', () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}))

const TEAM_ID = 'team-1'

const config = {
  id: 'cfg-1',
  team_id: TEAM_ID,
  app_id: '123456',
  app_slug: 'acme-app',
  client_id: 'Iv1.abc',
  has_private_key: true,
  has_client_secret: true,
  has_webhook_secret: true,
  webhook_url: 'https://vibexp.example.com/api/v1/webhooks/github/rout1ngtok3n',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  version: 1,
} as unknown as GitHubAppConfigResponse

const renderCard = (
  props: Partial<React.ComponentProps<typeof GitHubAppConfigCard>> = {}
) =>
  render(
    <GitHubAppConfigCard
      teamId={TEAM_ID}
      config={config}
      canManage
      onChanged={jest.fn()}
      {...props}
    />
  )

describe('GitHubAppConfigCard — unconfigured', () => {
  it('shows the setup guide rather than the configured state', () => {
    renderCard({ config: null })

    expect(screen.getByText('Connect a GitHub App')).toBeVisible()
    expect(screen.getByText(/Create a GitHub App/i)).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Register GitHub App' })
    ).toBeVisible()
  })

  it('spells out the permissions and events with their rationale', () => {
    renderCard({ config: null })

    expect(screen.getByText(/Contents: Read-only/)).toBeVisible()
    expect(screen.getByText(/Metadata: Read-only/)).toBeVisible()
    expect(screen.getByText('Installation')).toBeVisible()
    expect(screen.getByText('Installation repositories')).toBeVisible()
  })
})

describe('GitHubAppConfigCard — configured', () => {
  it('shows the public fields and the webhook URL', () => {
    renderCard()

    expect(screen.getByText('acme-app')).toBeVisible()
    expect(screen.getByText('123456')).toBeVisible()
    expect(screen.getByText(config.webhook_url)).toBeVisible()
  })

  it('reports secrets as Set/Not set and never renders a value', () => {
    const { container } = renderCard({
      config: { ...config, has_webhook_secret: false },
    })

    expect(screen.getByText('Private key: Set')).toBeVisible()
    expect(screen.getByText('Client secret: Set')).toBeVisible()
    expect(screen.getByText('Webhook secret: Not set')).toBeVisible()
    // A missing webhook secret is called out, because it silently breaks
    // delivery verification rather than failing loudly.
    expect(screen.getByText('No webhook secret set')).toBeVisible()

    // Nothing secret-shaped is in the DOM at all.
    expect(container.textContent).not.toMatch(/BEGIN .*PRIVATE KEY/)
  })
})

describe('GitHubAppConfigCard — permission gating', () => {
  it('offers no mutating affordance to a member', () => {
    renderCard({ canManage: false })

    for (const name of ['Edit', 'Verify', 'Rotate webhook URL', 'Remove']) {
      expect(screen.queryByRole('button', { name })).toBeNull()
    }
    expect(
      screen.getByText('Only a team owner or admin can change these settings.')
    ).toBeVisible()
  })

  it('offers no registration affordance to a member on an unconfigured team', () => {
    renderCard({ config: null, canManage: false })

    expect(
      screen.queryByRole('button', { name: 'Register GitHub App' })
    ).toBeNull()
    expect(screen.getByText('Ask an owner or admin')).toBeVisible()
    // The guide itself stays visible — reading how it works is not a mutation.
    expect(screen.getByText(/Create a GitHub App/i)).toBeVisible()
  })

  it('offers the full surface to an owner or admin', () => {
    renderCard({ canManage: true })

    for (const name of ['Edit', 'Verify', 'Rotate webhook URL', 'Remove']) {
      expect(screen.getByRole('button', { name })).toBeVisible()
    }
  })
})

describe('GitHubAppConfigCard — destructive confirmations', () => {
  // These two dialogs exist for what they SAY, not that they exist: rotating
  // without updating GitHub stops deliveries silently, and removing the App
  // disconnects installations. Asserting only that the buttons render would
  // pass with the warnings deleted.
  it('warns that GitHub must be updated after rotating', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Rotate webhook URL' }))

    expect(
      await screen.findByText(/deliveries stop until you do/i)
    ).toBeVisible()
  })

  it('warns that installations disconnect on remove', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Remove' }))

    expect(
      await screen.findByText(/installations are disconnected/i)
    ).toBeVisible()
  })
})

describe('GitHubAppSetupGuide — organization deep link', () => {
  it('targets the personal account by default and the org once given one', async () => {
    const user = userEvent.setup()
    renderCard({ config: null })

    expect(
      screen.getByRole('link', { name: /Create App on GitHub/i })
    ).toHaveAttribute('href', 'https://github.com/settings/apps/new')

    await user.type(
      screen.getByLabelText(/Organization \(optional\)/i),
      'acme-inc'
    )

    expect(
      screen.getByRole('link', { name: /Create App in acme-inc/i })
    ).toHaveAttribute(
      'href',
      'https://github.com/organizations/acme-inc/settings/apps/new'
    )
  })
})

describe('GitHubAppConfigCard — actions', () => {
  const service = githubAppConfigService as unknown as Record<string, jest.Mock>

  it('rotates, refreshes, and re-discloses the new webhook URL', async () => {
    const user = userEvent.setup()
    service.rotateWebhookToken.mockResolvedValue({
      ...config,
      webhook_url: 'https://vibexp.example.com/api/v1/webhooks/github/new-tok',
    })
    const onChanged = jest.fn()
    renderCard({ onChanged })

    await user.click(screen.getByRole('button', { name: 'Rotate webhook URL' }))
    const rotateDialog = await screen.findByRole('alertdialog')
    await user.click(
      within(rotateDialog).getByRole('button', { name: 'Rotate' })
    )

    await waitFor(() => {
      expect(service.rotateWebhookToken).toHaveBeenCalled()
    })
    expect(onChanged).toHaveBeenCalled()
    // The new URL is useless unless it is handed over — rotating without
    // showing it would stop deliveries with no way to fix them.
    expect(
      await screen.findByText(
        'https://vibexp.example.com/api/v1/webhooks/github/new-tok'
      )
    ).toBeVisible()
  })

  it('deletes and refreshes', async () => {
    const user = userEvent.setup()
    service.deleteAppConfig.mockResolvedValue(undefined)
    const onChanged = jest.fn()
    renderCard({ onChanged })

    await user.click(screen.getByRole('button', { name: 'Remove' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Remove' }))

    await waitFor(() => {
      expect(service.deleteAppConfig).toHaveBeenCalled()
    })
    expect(onChanged).toHaveBeenCalled()
  })

  it('reports a failed verify with the mapped instruction', async () => {
    const user = userEvent.setup()
    service.validateAppConfig.mockResolvedValue({
      is_valid: false,
      message: 'nope',
      details: { error_details: 'slug_mismatch' },
    })
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Verify' }))

    const { toast } = jest.requireMock('@/lib/toast')
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        'The App slug does not match',
        expect.objectContaining({
          description: expect.stringContaining('public URL'),
        })
      )
    })
  })

  it('reports a successful verify', async () => {
    const user = userEvent.setup()
    service.validateAppConfig.mockResolvedValue({
      is_valid: true,
      message: 'ok',
    })
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Verify' }))

    const { toast } = jest.requireMock('@/lib/toast')
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith('GitHub App verified')
    })
  })
})

describe('GitHubAppConfigCard — failure paths', () => {
  const service = githubAppConfigService as unknown as Record<string, jest.Mock>
  const toastOf = () => jest.requireMock('@/lib/toast').toast

  it.each([
    [
      'rotateWebhookToken',
      'Rotate webhook URL',
      'Rotate',
      'Could not rotate the webhook URL',
    ],
    ['deleteAppConfig', 'Remove', 'Remove', 'Could not remove the GitHub App'],
  ])(
    'reports a failed %s instead of appearing to succeed',
    async (method, trigger, confirm, message) => {
      const user = userEvent.setup()
      service[method].mockRejectedValue(new Error('boom'))
      const onChanged = jest.fn()
      renderCard({ onChanged })

      await user.click(screen.getByRole('button', { name: trigger }))
      // Scope to the dialog: the trigger and the confirm can share a label,
      // and the trigger is inert behind the modal.
      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: confirm }))

      await waitFor(() => {
        expect(toastOf().error).toHaveBeenCalledWith(message)
      })
      // A failed mutation must not trigger a refetch that would imply it worked.
      expect(onChanged).not.toHaveBeenCalled()
    }
  )

  it('reports a verify that could not run at all', async () => {
    const user = userEvent.setup()
    service.validateAppConfig.mockRejectedValue(new Error('network'))
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Verify' }))

    await waitFor(() => {
      expect(toastOf().error).toHaveBeenCalledWith(
        'Could not verify the GitHub App'
      )
    })
  })
})
