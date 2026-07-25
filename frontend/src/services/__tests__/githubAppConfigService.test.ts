import { generatedClient } from '@/lib/apiClientGenerated'
import {
  githubAppConfigService,
  isGitHubAppNotConfigured,
} from '@/services/githubAppConfigService'
import { ApiError } from '@/types/errors'

jest.mock('@/lib/apiClientGenerated', () => ({
  generatedClient: {
    GET: jest.fn(),
    POST: jest.fn(),
    PUT: jest.fn(),
    DELETE: jest.fn(),
  },
  unwrap: jest.fn(async (p: Promise<{ data: unknown }>) => (await p).data),
}))

const client = generatedClient as unknown as Record<string, jest.Mock>

const TEAM_ID = 'team-1'
const ok = (data: unknown) => Promise.resolve({ data })

beforeEach(() => jest.clearAllMocks())

describe('githubAppConfigService', () => {
  it('reads the config from the settings prefix, scoped to the team', async () => {
    client.GET.mockReturnValue(ok({ app_slug: 'acme' }))

    await githubAppConfigService.getAppConfig(TEAM_ID)

    expect(client.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/settings/github-app',
      { params: { path: { team_id: TEAM_ID } } }
    )
  })

  it('passes the create payload through untouched', async () => {
    client.POST.mockReturnValue(ok({ webhook_secret: 's' }))
    const body = {
      app_id: '1',
      app_slug: 'a',
      client_id: 'c',
      client_secret: 'cs',
      private_key: 'pk',
    }

    await githubAppConfigService.createAppConfig(TEAM_ID, body)

    expect(client.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/settings/github-app',
      { params: { path: { team_id: TEAM_ID } }, body }
    )
  })

  it('sends an update with omitted secrets as-is', async () => {
    client.PUT.mockReturnValue(ok({}))

    await githubAppConfigService.updateAppConfig(TEAM_ID, {
      app_slug: 'renamed',
      client_secret: undefined,
      private_key: undefined,
    })

    const [, options] = client.PUT.mock.calls[0] as [
      string,
      { body: Record<string, unknown> },
    ]
    expect(options.body.client_secret).toBeUndefined()
    expect(options.body.private_key).toBeUndefined()
  })

  it('targets the validate and rotate sub-routes', async () => {
    client.POST.mockReturnValue(ok({ is_valid: true, message: 'ok' }))

    await githubAppConfigService.validateAppConfig(TEAM_ID)
    await githubAppConfigService.rotateWebhookToken(TEAM_ID)

    expect(client.POST).toHaveBeenNthCalledWith(
      1,
      '/api/v1/{team_id}/settings/github-app/validate',
      { params: { path: { team_id: TEAM_ID } } }
    )
    expect(client.POST).toHaveBeenNthCalledWith(
      2,
      '/api/v1/{team_id}/settings/github-app/rotate-webhook-token',
      { params: { path: { team_id: TEAM_ID } } }
    )
  })
})

describe('isGitHubAppNotConfigured', () => {
  const apiError = (status: number, code: string) =>
    new ApiError({
      status,
      code,
      title: 't',
      detail: 'd',
      request_id: 'r',
      type: 'about:blank',
    } as ConstructorParameters<typeof ApiError>[0])

  it('recognises the documented not-configured signal', () => {
    expect(
      isGitHubAppNotConfigured(apiError(409, 'GITHUB_APP_NOT_CONFIGURED'))
    ).toBe(true)
  })

  it('does not swallow other conflicts', () => {
    // A 409 that means something else entirely must still surface as an error;
    // treating every 409 as "no App yet" would hide a real conflict behind an
    // empty state.
    expect(
      isGitHubAppNotConfigured(apiError(409, 'GITHUB_APP_ALREADY_REGISTERED'))
    ).toBe(false)
    expect(isGitHubAppNotConfigured(apiError(403, 'FORBIDDEN'))).toBe(false)
    expect(isGitHubAppNotConfigured(new Error('network'))).toBe(false)
    expect(isGitHubAppNotConfigured(undefined)).toBe(false)
  })
})
