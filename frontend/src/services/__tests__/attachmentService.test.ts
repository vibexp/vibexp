// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
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

// Import after mocking dependencies. getApiBaseUrl is mocked globally by the jest
// setup to return https://api.vibexp.io/api/v1.
import { attachmentService } from '../attachmentService'

const apiBaseUrl = 'https://api.vibexp.io/api/v1'

const teamId = 'team-1'
const ownerType = 'artifact'
const ownerId = 'artifact-1'
const attachmentId = 'att-1'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContentResponse = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

describe('attachmentService (universal endpoint)', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('list passes owner_type/owner_id as query params on the team-scoped path', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({ attachments: [], total_count: 0, total_size_bytes: 0 })
    )

    await attachmentService.list(teamId, ownerType, ownerId)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/attachments',
      {
        params: {
          path: { team_id: teamId },
          query: { owner_type: ownerType, owner_id: ownerId },
        },
      }
    )
  })

  it('upload posts a multipart body carrying the owner fields and file', async () => {
    mockGeneratedClient.POST.mockReturnValue(success({ id: attachmentId }))
    const file = new File(['hello'], 'notes.txt', { type: 'text/plain' })

    await attachmentService.upload(teamId, ownerType, ownerId, file)

    const [path, init] = mockGeneratedClient.POST.mock.calls[0] as [
      string,
      {
        params: unknown
        body: Record<string, string>
        bodySerializer: (body: Record<string, string>) => FormData
      },
    ]
    expect(path).toBe('/api/v1/{team_id}/attachments')
    expect(init.params).toEqual({ path: { team_id: teamId } })

    // The serializer must produce the multipart FormData the endpoint expects.
    const form = init.bodySerializer(init.body)
    expect(form).toBeInstanceOf(FormData)
    expect(form.get('owner_type')).toBe(ownerType)
    expect(form.get('owner_id')).toBe(ownerId)
    expect(form.get('file')).toBeInstanceOf(File)
  })

  it('remove deletes by attachment id only (no owner in path)', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(
      Promise.resolve({ data: undefined, response: noContentResponse })
    )

    await attachmentService.remove(teamId, attachmentId)

    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
      '/api/v1/{team_id}/attachments/{id}',
      { params: { path: { team_id: teamId, id: attachmentId } } }
    )
  })

  it('throws ApiError with backend detail on RFC 9457 error', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      Promise.resolve({
        error: {
          type: 'https://api.vibexp.io/errors/NOT_FOUND',
          title: 'Not Found',
          status: 404,
          detail: 'owner not found',
          code: 'RESOURCE_NOT_FOUND',
          request_id: 'req-1',
          timestamp: '2024-01-01T10:00:00Z',
        },
        response: { ok: false, status: 404, statusText: 'Not Found' },
      })
    )

    await expect(
      attachmentService.list(teamId, ownerType, ownerId)
    ).rejects.toThrow('owner not found')
  })

  it('download fetches the item URL with credentials and returns a Blob', async () => {
    const blob = new Blob(['data'])
    const fetchMock = jest.fn().mockResolvedValue({
      ok: true,
      blob: jest.fn().mockResolvedValue(blob),
    })
    global.fetch = fetchMock

    const result = await attachmentService.download(teamId, attachmentId)

    expect(fetchMock).toHaveBeenCalledWith(
      `${apiBaseUrl}/${teamId}/attachments/${attachmentId}`,
      { credentials: 'include' }
    )
    expect(result).toBe(blob)
  })

  it('download throws on a non-ok response', async () => {
    global.fetch = jest.fn().mockResolvedValue({ ok: false, status: 404 })

    await expect(
      attachmentService.download(teamId, attachmentId)
    ).rejects.toThrow('Failed to download attachment (HTTP 404)')
  })
})
