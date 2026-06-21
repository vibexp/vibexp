import { apiClient } from '../lib/apiClient'
import type {
  Blueprint,
  BlueprintFilters,
  BlueprintListResponse,
  BlueprintStatsResponse,
  BlueprintVersion,
  BlueprintVersionListResponse,
  CreateBlueprintRequest,
  UpdateBlueprintRequest,
} from '../types'

class BlueprintService {
  async getBlueprints(
    teamId: string,
    filters: BlueprintFilters = {}
  ): Promise<BlueprintListResponse> {
    const params = new URLSearchParams()

    // Remove team_id from query params - it's now in the URL path
    // Handle standard filters
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== '') {
        params.append(key, String(value))
      }
    })

    const queryString = params.toString()
    const endpoint = `/${teamId}/blueprints${queryString ? `?${queryString}` : ''}`

    return apiClient.get<BlueprintListResponse>(endpoint)
  }

  async getBlueprintsByProject(
    teamId: string,
    projectId: string,
    filters: Omit<BlueprintFilters, 'project_id'> = {}
  ): Promise<BlueprintListResponse> {
    const params = new URLSearchParams()
    // Remove team_id from query params - it's now in the URL path
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined && value !== '') {
        params.append(key, String(value))
      }
    })

    const queryString = params.toString()
    const endpoint = `/${teamId}/blueprints/${encodeURIComponent(projectId)}${queryString ? `?${queryString}` : ''}`

    return apiClient.get<BlueprintListResponse>(endpoint)
  }

  async getBlueprint(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<Blueprint> {
    return apiClient.get<Blueprint>(
      `/${teamId}/blueprints/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}`
    )
  }

  async getBlueprintVersions(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<BlueprintVersionListResponse> {
    return apiClient.get<BlueprintVersionListResponse>(
      `/${encodeURIComponent(teamId)}/blueprints/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}/versions`
    )
  }

  async getBlueprintVersion(
    teamId: string,
    projectId: string,
    slug: string,
    versionNumber: number
  ): Promise<BlueprintVersion> {
    return apiClient.get<BlueprintVersion>(
      `/${encodeURIComponent(teamId)}/blueprints/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}/versions/${encodeURIComponent(versionNumber)}`
    )
  }

  async restoreBlueprintVersion(
    teamId: string,
    projectId: string,
    slug: string,
    versionNumber: number
  ): Promise<Blueprint> {
    return apiClient.post<Blueprint>(
      `/${encodeURIComponent(teamId)}/blueprints/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}/versions/${encodeURIComponent(versionNumber)}/restore`
    )
  }

  async createBlueprint(
    teamId: string,
    data: CreateBlueprintRequest
  ): Promise<Blueprint> {
    return apiClient.post<Blueprint>(`/${teamId}/blueprints`, data)
  }

  async updateBlueprint(
    teamId: string,
    projectId: string,
    slug: string,
    data: UpdateBlueprintRequest
  ): Promise<Blueprint> {
    return apiClient.put<Blueprint>(
      `/${teamId}/blueprints/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}`,
      data
    )
  }

  async deleteBlueprint(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<void> {
    await apiClient.delete(
      `/${teamId}/blueprints/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}`
    )
  }

  async getBlueprintStats(teamId: string): Promise<BlueprintStatsResponse> {
    try {
      // Remove team_id from query params - it's now in the URL path
      const response = await apiClient.get<BlueprintStatsResponse>(
        `/${teamId}/blueprints/stats`
      )
      return response
    } catch (error) {
      console.error('Failed to fetch blueprint stats:', error)
      return {
        total_projects: 0,
        total_blueprints: 0,
        added_this_week: 0,
        total_by_type: {},
        total_by_status: {},
      }
    }
  }
}

export const blueprintService = new BlueprintService()
