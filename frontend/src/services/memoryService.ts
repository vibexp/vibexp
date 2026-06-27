import { apiClient } from '../lib/apiClient'
import type {
  CreateMemoryRequest,
  MemoriesResponse,
  MemoryFilters,
  MemoryResponse,
  MemoryVersion,
  MemoryVersionListResponse,
  UpdateMemoryRequest,
} from '../types'

class MemoryService {
  async getMemories(
    teamId: string,
    filters: MemoryFilters = {}
  ): Promise<MemoriesResponse> {
    const params = new URLSearchParams()

    if (filters.project_id) params.append('project_id', filters.project_id)
    if (filters.search) params.append('search', filters.search)
    if (filters.status) params.append('status', filters.status)
    if (filters.metadata_key)
      params.append('metadata_key', filters.metadata_key)
    if (filters.metadata_value)
      params.append('metadata_value', filters.metadata_value)
    if (filters.sort_by) params.append('sort_by', filters.sort_by)
    if (filters.sort_order) params.append('sort_order', filters.sort_order)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())

    const queryString = params.toString()
    const endpoint = `/${teamId}/memories${queryString ? `?${queryString}` : ''}`

    return apiClient.get<MemoriesResponse>(endpoint)
  }

  async getMemory(teamId: string, id: string): Promise<MemoryResponse> {
    return apiClient.get<MemoryResponse>(`/${teamId}/memories/${id}`)
  }

  async createMemory(
    teamId: string,
    data: CreateMemoryRequest
  ): Promise<MemoryResponse> {
    return apiClient.post<MemoryResponse>(`/${teamId}/memories`, data)
  }

  async updateMemory(
    teamId: string,
    id: string,
    data: UpdateMemoryRequest
  ): Promise<MemoryResponse> {
    return apiClient.put<MemoryResponse>(`/${teamId}/memories/${id}`, data)
  }

  async deleteMemory(teamId: string, id: string): Promise<void> {
    await apiClient.delete(`/${teamId}/memories/${id}`)
  }

  async getMemoryVersions(
    teamId: string,
    id: string
  ): Promise<MemoryVersionListResponse> {
    return apiClient.get<MemoryVersionListResponse>(
      `/${encodeURIComponent(teamId)}/memories/${encodeURIComponent(id)}/versions`
    )
  }

  async getMemoryVersion(
    teamId: string,
    id: string,
    versionNumber: number
  ): Promise<MemoryVersion> {
    return apiClient.get<MemoryVersion>(
      `/${encodeURIComponent(teamId)}/memories/${encodeURIComponent(id)}/versions/${encodeURIComponent(versionNumber)}`
    )
  }

  async restoreMemoryVersion(
    teamId: string,
    id: string,
    versionNumber: number
  ): Promise<MemoryResponse> {
    return apiClient.post<MemoryResponse>(
      `/${encodeURIComponent(teamId)}/memories/${encodeURIComponent(id)}/versions/${encodeURIComponent(versionNumber)}/restore`
    )
  }

  async searchMemoriesByMetadata(
    teamId: string,
    filters: MemoryFilters
  ): Promise<MemoriesResponse> {
    if (!filters.metadata_key || !filters.metadata_value) {
      throw new Error(
        'metadata_key and metadata_value are required for metadata search'
      )
    }

    const params = new URLSearchParams()
    params.append('metadata_key', filters.metadata_key)
    params.append('metadata_value', filters.metadata_value)
    if (filters.search) params.append('search', filters.search)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())

    return apiClient.get<MemoriesResponse>(
      `/${teamId}/memories/search?${params.toString()}`
    )
  }
}

export const memoryService = new MemoryService()
