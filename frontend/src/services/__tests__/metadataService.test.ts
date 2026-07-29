// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PUT: vi.fn(),
  DELETE: vi.fn(),
}))

vi.mock('../../lib/apiClientGenerated', async () => {
  const actual = await vi.importActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return { ...actual, generatedClient: mockGeneratedClient }
})

import { metadataService, serializeMetadataFilter } from '../metadataService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const TEAM_ID = 'team-1'

describe('metadataService', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  test('listKeys requests the keys catalog with path and query params', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({ keys: ['env'], truncated: false })
    )

    const result = await metadataService.listKeys(TEAM_ID, {
      resource_type: 'blueprints',
      limit: 100,
    })

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/metadata/keys',
      {
        params: {
          path: { team_id: TEAM_ID },
          query: { resource_type: 'blueprints', limit: 100 },
        },
      }
    )
    expect(result).toEqual({ keys: ['env'], truncated: false })
  })

  test('listValues forwards the key, typeahead and project narrowing', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({ values: ['prod'], truncated: true })
    )

    const result = await metadataService.listValues(TEAM_ID, {
      resource_type: 'artifacts',
      key: 'env',
      q: 'pro',
      project_id: 'project-1',
      limit: 50,
    })

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/metadata/values',
      {
        params: {
          path: { team_id: TEAM_ID },
          query: {
            resource_type: 'artifacts',
            key: 'env',
            q: 'pro',
            project_id: 'project-1',
            limit: 50,
          },
        },
      }
    )
    expect(result.truncated).toBe(true)
  })

  test('a failed response rejects', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      Promise.resolve({
        error: { detail: 'nope' },
        response: {
          ok: false,
          status: 400,
          statusText: 'Bad Request',
        } as Response,
      })
    )

    await expect(
      metadataService.listKeys(TEAM_ID, { resource_type: 'memories' })
    ).rejects.toBeDefined()
  })
})

describe('serializeMetadataFilter', () => {
  test('encodes the filter as the JSON object the metadata param takes', () => {
    expect(
      serializeMetadataFilter({ env: ['prod', 'staging'], team: ['core'] })
    ).toBe('{"env":["prod","staging"],"team":["core"]}')
  })

  test('an empty filter serializes to undefined so callers can omit the param', () => {
    // Sending `metadata={}` would be a pointless round-trip; the caller spreads
    // undefined away instead.
    expect(serializeMetadataFilter({})).toBeUndefined()
  })

  test('an empty value array is preserved as the key-exists form', () => {
    expect(serializeMetadataFilter({ env: [] })).toBe('{"env":[]}')
  })
})
