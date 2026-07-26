import { ApiError } from '../../types/errors'
import type {
  TeamEmailProviderResponse,
  TeamEmailProviderTestResponse,
  UpsertTeamEmailProviderRequest,
} from '../emailProviderService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  PUT: jest.fn(),
  POST: jest.fn(),
  DELETE: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { emailProviderService } from '../emailProviderService'

const teamId = 'team-1'
const path = '/api/v1/{team_id}/settings/email-provider'
const testPath = '/api/v1/{team_id}/settings/email-provider/test'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContent = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response

const success = <T>(data: T, response: Response = okResponse) =>
  Promise.resolve({ data, response })

// An RFC 9457 problem-details error body as openapi-fetch surfaces it.
const problem = (status: number, detail: string, code: string) =>
  Promise.resolve({
    error: {
      type: `https://api.vibexp.io/errors/${code}`,
      title: code,
      status,
      detail,
      code,
      request_id: 'req-1',
      timestamp: '2024-01-01T00:00:00Z',
    },
    response: { ok: false, status, statusText: code } as Response,
  })

const inheriting: TeamEmailProviderResponse = {
  configured: false,
  source: 'instance',
  effective_from_address: 'noreply@instance.test',
  provider_type: null,
  has_credential: false,
}

const configured: TeamEmailProviderResponse = {
  configured: true,
  source: 'team',
  effective_from_address: 'hello@acme.test',
  provider_type: 'mailgun',
  has_credential: true,
  from_address: 'hello@acme.test',
  settings: { mailgun: { domain: 'mg.acme.test' } },
  is_healthy: true,
}

describe('emailProviderService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getEmailProvider', () => {
    it('requests the team-scoped settings variant and resolves the payload', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(configured))

      await expect(
        emailProviderService.getEmailProvider(teamId)
      ).resolves.toEqual(configured)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(path, {
        params: { path: { team_id: teamId } },
      })
    })

    it('resolves the inheriting state rather than treating it as missing', async () => {
      // The endpoint never 404s: a team with no provider of its own is
      // inheriting the instance one, which is a state the page must render.
      mockGeneratedClient.GET.mockReturnValue(success(inheriting))

      await expect(
        emailProviderService.getEmailProvider(teamId)
      ).resolves.toEqual(inheriting)
    })

    it('rejects with an ApiError when unauthenticated', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        problem(401, 'Unauthorized', 'unauthorized')
      )

      await expect(
        emailProviderService.getEmailProvider(teamId)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })

  describe('upsertEmailProvider', () => {
    const request: UpsertTeamEmailProviderRequest = {
      provider_type: 'mailgun',
      from_address: 'hello@acme.test',
      secret: 'key-abc123',
      settings: { mailgun: { domain: 'mg.acme.test' } },
    }

    it('PUTs the configuration and resolves the stored provider', async () => {
      mockGeneratedClient.PUT.mockReturnValue(success(configured))

      await expect(
        emailProviderService.upsertEmailProvider(teamId, request)
      ).resolves.toEqual(configured)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(path, {
        params: { path: { team_id: teamId } },
        body: request,
      })
    })

    it('passes a secret-less body through untouched so the stored one is kept', async () => {
      // Omission is the "keep current credential" signal; the service must not
      // helpfully add an empty string, which the backend rejects.
      const withoutSecret: UpsertTeamEmailProviderRequest = {
        provider_type: 'mailgun',
        from_address: 'hello@acme.test',
        settings: { mailgun: { domain: 'mg.acme.test' } },
      }
      mockGeneratedClient.PUT.mockReturnValue(success(configured))

      await emailProviderService.upsertEmailProvider(teamId, withoutSecret)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(path, {
        params: { path: { team_id: teamId } },
        body: withoutSecret,
      })
      expect(mockGeneratedClient.PUT.mock.calls[0][1].body).not.toHaveProperty(
        'secret'
      )
    })

    it('rejects with an ApiError when the configuration is invalid', async () => {
      mockGeneratedClient.PUT.mockReturnValue(
        problem(400, 'from_address is not a valid address', 'invalid_request')
      )

      await expect(
        emailProviderService.upsertEmailProvider(teamId, request)
      ).rejects.toBeInstanceOf(ApiError)
    })

    it('rejects with an ApiError when the caller is not an owner or admin', async () => {
      mockGeneratedClient.PUT.mockReturnValue(
        problem(403, 'Forbidden', 'forbidden')
      )

      await expect(
        emailProviderService.upsertEmailProvider(teamId, request)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })

  describe('deleteEmailProvider', () => {
    it('DELETEs the provider and resolves on 204', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(success(undefined, noContent))

      await expect(
        emailProviderService.deleteEmailProvider(teamId)
      ).resolves.toBeUndefined()

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(path, {
        params: { path: { team_id: teamId } },
      })
    })

    it('rejects with an ApiError when the team has no provider of its own', async () => {
      // 409, not 404 — the endpoint exists, the team just is not in a state
      // that can serve the request.
      mockGeneratedClient.DELETE.mockReturnValue(
        problem(
          409,
          'team has no email provider',
          'team_email_provider_not_configured'
        )
      )

      await expect(
        emailProviderService.deleteEmailProvider(teamId)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })

  describe('testEmailProvider', () => {
    const request: UpsertTeamEmailProviderRequest = {
      provider_type: 'smtp',
      from_address: 'hello@acme.test',
      secret: 'hunter2',
      settings: { smtp: { host: 'smtp.acme.test', port: '587' } },
    }

    it('POSTs to the test sub-resource with the candidate configuration', async () => {
      const result: TeamEmailProviderTestResponse = {
        is_valid: true,
        message: 'Test message accepted',
        recipient: 'admin@acme.test',
        details: {},
      }
      mockGeneratedClient.POST.mockReturnValue(success(result))

      await expect(
        emailProviderService.testEmailProvider(teamId, request)
      ).resolves.toEqual(result)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(testPath, {
        params: { path: { team_id: teamId } },
        body: request,
      })
    })

    it('resolves a rejected send rather than rejecting', async () => {
      // A failed send is a successful answer to "does this configuration
      // work?" — it comes back 200 with is_valid:false, so callers must read
      // the body instead of relying on a thrown error.
      const result: TeamEmailProviderTestResponse = {
        is_valid: false,
        message: 'Sending failed: connection refused',
        recipient: 'admin@acme.test',
        details: { error_details: 'send_failed' },
      }
      mockGeneratedClient.POST.mockReturnValue(success(result))

      await expect(
        emailProviderService.testEmailProvider(teamId, request)
      ).resolves.toEqual(result)
    })

    it('rejects with an ApiError when the body omits the required secret', async () => {
      mockGeneratedClient.POST.mockReturnValue(
        problem(400, 'secret is required', 'invalid_request')
      )

      await expect(
        emailProviderService.testEmailProvider(teamId, request)
      ).rejects.toBeInstanceOf(ApiError)
    })
  })
})
