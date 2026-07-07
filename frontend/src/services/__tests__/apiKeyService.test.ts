import type {
  APIKey,
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
} from '../apiKeyService'

const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  DELETE: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return { ...actual, generatedClient: mockGeneratedClient }
})

import { apiKeyService } from '../apiKeyService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContent = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response
const success = <T>(data: T, response: Response = okResponse) =>
  Promise.resolve({ data, response })
const problem = (status: number, detail: string, code: string) =>
  Promise.resolve({
    error: {
      type: `https://api.vibexp.io/errors/${code}`,
      title: code,
      status,
      detail,
      code,
      request_id: 'req-1',
      timestamp: '2026-01-01T00:00:00Z',
    },
    response: { ok: false, status, statusText: code } as Response,
  })

describe('APIKeyService', () => {
  const mockAPIKey: APIKey = {
    id: 'test-api-key-id',
    user_id: 'test-user-id',
    name: 'Test API Key',
    key_prefix: 'vxk_test_',
    integrations: ['ai_tools', 'cli'],
    is_legacy: false,
    last_used_at: null,
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('createAPIKey posts the request and returns the created key', async () => {
    const request: CreateAPIKeyRequest = {
      name: 'Test Key',
      integration_codes: ['ai_tools', 'cli', 'mcp_server'],
    }
    const response: CreateAPIKeyResponse = {
      api_key: mockAPIKey,
      full_key: 'vxk_test_full_key_value',
      key_prefix: 'vxk_test_',
    }
    mockGeneratedClient.POST.mockReturnValue(success(response))

    const result = await apiKeyService.createAPIKey(request)

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/settings/api-keys',
      { body: request }
    )
    expect(result).toEqual(response)
  })

  it('getAPIKeys returns the bare array the backend actually sends', async () => {
    const keys = [mockAPIKey, { ...mockAPIKey, id: 'key-2' }]
    // Backend returns a bare APIKey[] despite the spec's envelope (see service).
    mockGeneratedClient.GET.mockReturnValue(success(keys))

    const result = await apiKeyService.getAPIKeys()

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/settings/api-keys',
      {}
    )
    expect(result).toEqual(keys)
  })

  it('deleteAPIKey deletes by id and resolves void', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(success(undefined, noContent))

    await expect(apiKeyService.deleteAPIKey('key-1')).resolves.toBeUndefined()
    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
      '/api/v1/settings/api-keys/{id}',
      { params: { path: { id: 'key-1' } } }
    )
  })

  it('throws ApiError with the backend detail on a validation failure', async () => {
    mockGeneratedClient.POST.mockReturnValue(
      problem(400, 'Invalid integration code', 'VALIDATION_FAILED')
    )

    await expect(
      apiKeyService.createAPIKey({ name: 'x', integration_codes: ['ai_tools'] })
    ).rejects.toThrow('Invalid integration code')
  })
})
