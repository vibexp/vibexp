import { act, renderHook, waitFor } from '@testing-library/react'

import { useRelations } from '@/hooks/useRelations'
import type {
  RelatedResource,
  RelationListResponse,
} from '@/services/relationService'
import { relationService } from '@/services/relationService'

jest.mock('@/services/relationService', () => ({
  relationService: {
    list: jest.fn(),
    create: jest.fn(),
    confirm: jest.fn(),
    remove: jest.fn(),
  },
}))

const mockList = relationService.list as jest.Mock
const mockCreate = relationService.create as jest.Mock
const mockConfirm = relationService.confirm as jest.Mock
const mockRemove = relationService.remove as jest.Mock

function makeRelation(
  overrides: Partial<RelatedResource> = {}
): RelatedResource {
  return {
    relation_id: 'r1',
    relation_type: 'governed-by',
    direction: 'outgoing',
    origin: 'ai',
    status: 'suggested',
    resource_type: 'blueprint',
    resource_id: 'b1',
    title: 'Go standards',
    created_at: '2026-07-21T09:00:00Z',
    ...overrides,
  }
}

function listResp(relations: RelatedResource[]): RelationListResponse {
  return {
    relations,
    total_count: relations.length,
    page: 1,
    per_page: 100,
    total_pages: 1,
  }
}

beforeEach(() => {
  jest.clearAllMocks()
})

async function renderLoaded(relations: RelatedResource[]) {
  mockList.mockResolvedValue(listResp(relations))
  const view = renderHook(() => useRelations('team-1', 'artifact', 'a1'))
  await waitFor(() => {
    expect(view.result.current.loading).toBe(false)
  })
  return view
}

test('loads both-directions relations on mount', async () => {
  const { result } = await renderLoaded([makeRelation()])
  expect(mockList).toHaveBeenCalledWith('team-1', 'artifact', 'a1', 1, 100)
  expect(result.current.relations).toHaveLength(1)
  expect(result.current.error).toBe(false)
})

test('addRelation creates a human edge then reloads', async () => {
  const { result } = await renderLoaded([])
  mockCreate.mockResolvedValue({ id: 'rel-new' })
  mockList.mockResolvedValue(
    listResp([makeRelation({ status: 'confirmed', origin: 'human' })])
  )

  await act(async () => {
    await result.current.addRelation('governed-by', 'blueprint', 'b1')
  })

  expect(mockCreate).toHaveBeenCalledWith('team-1', {
    from_type: 'artifact',
    from_id: 'a1',
    to_type: 'blueprint',
    to_id: 'b1',
    relation_type: 'governed-by',
    origin: 'human',
  })
  await waitFor(() => {
    expect(result.current.relations).toHaveLength(1)
  })
})

test('confirmRelation optimistically flips to confirmed', async () => {
  const { result } = await renderLoaded([makeRelation({ status: 'suggested' })])
  mockConfirm.mockResolvedValue({ id: 'r1' })

  await act(async () => {
    await result.current.confirmRelation('r1')
  })

  expect(mockConfirm).toHaveBeenCalledWith('team-1', 'r1')
  expect(result.current.relations[0].status).toBe('confirmed')
})

test('confirmRelation rolls back on error', async () => {
  const { result } = await renderLoaded([makeRelation({ status: 'suggested' })])
  mockConfirm.mockRejectedValue(new Error('boom'))

  await act(async () => {
    await expect(result.current.confirmRelation('r1')).rejects.toThrow('boom')
  })

  expect(result.current.relations[0].status).toBe('suggested')
})

test('removeRelation optimistically removes then rolls back on error', async () => {
  const { result } = await renderLoaded([makeRelation({ relation_id: 'r1' })])
  mockRemove.mockRejectedValue(new Error('nope'))

  await act(async () => {
    await expect(result.current.removeRelation('r1')).rejects.toThrow('nope')
  })

  // rolled back
  expect(result.current.relations).toHaveLength(1)
})

test('list failure surfaces error', async () => {
  mockList.mockRejectedValue(new Error('down'))
  const { result } = renderHook(() => useRelations('team-1', 'artifact', 'a1'))
  await waitFor(() => {
    expect(result.current.error).toBe(true)
  })
  expect(result.current.relations).toHaveLength(0)
})
