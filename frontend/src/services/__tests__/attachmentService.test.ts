const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
  delete: jest.fn(),
}

jest.mock('../../lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

// Import after mocking dependencies. getApiBaseUrl is mocked globally by the jest
// setup to return https://api.vibexp.io/api/v1.
import { attachmentService } from '../attachmentService'

const apiBaseUrl = 'https://api.vibexp.io/api/v1'

const teamId = 'team-1'
const ownerType = 'artifact'
const ownerId = 'artifact-1'
const attachmentId = 'att-1'

describe('attachmentService (universal endpoint)', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('list passes owner_type/owner_id as query params on the team-scoped path', async () => {
    mockApiClient.get.mockResolvedValue({
      attachments: [],
      total_count: 0,
      total_size_bytes: 0,
    })

    await attachmentService.list(teamId, ownerType, ownerId)

    const calledWith = mockApiClient.get.mock.calls[0][0] as string
    expect(calledWith).toBe(
      `/${teamId}/attachments?owner_type=${ownerType}&owner_id=${ownerId}`
    )
  })

  it('upload posts FormData with owner fields to the team-scoped path', async () => {
    mockApiClient.post.mockResolvedValue({ id: attachmentId })
    const file = new File(['hello'], 'notes.txt', { type: 'text/plain' })

    await attachmentService.upload(teamId, ownerType, ownerId, file)

    const [path, body] = mockApiClient.post.mock.calls[0]
    expect(path).toBe(`/${teamId}/attachments`)
    expect(body).toBeInstanceOf(FormData)
    const form = body as FormData
    expect(form.get('owner_type')).toBe(ownerType)
    expect(form.get('owner_id')).toBe(ownerId)
    expect(form.get('file')).toBeInstanceOf(File)
  })

  it('remove deletes by attachment id only (no owner in path)', async () => {
    mockApiClient.delete.mockResolvedValue(undefined)

    await attachmentService.remove(teamId, attachmentId)

    expect(mockApiClient.delete).toHaveBeenCalledWith(
      `/${teamId}/attachments/${attachmentId}`
    )
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
