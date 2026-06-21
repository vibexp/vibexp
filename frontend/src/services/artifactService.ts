import { apiClient } from '../lib/apiClient'
import type {
  Artifact,
  ArtifactFilters,
  ArtifactListResponse,
  ArtifactStatsResponse,
  ArtifactVersion,
  ArtifactVersionListResponse,
  CreateArtifactRequest,
  UpdateArtifactRequest,
} from '../types'

class ArtifactService {
  async getArtifacts(
    teamId: string,
    filters: ArtifactFilters = {}
  ): Promise<ArtifactListResponse> {
    const params = new URLSearchParams()

    // Handle standard filters
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined) {
        params.append(key, String(value))
      }
    })

    const queryString = params.toString()
    const endpoint = `/${encodeURIComponent(teamId)}/artifacts${queryString ? `?${queryString}` : ''}`

    return apiClient.get<ArtifactListResponse>(endpoint)
  }

  async getArtifactsByProject(
    teamId: string,
    projectId: string,
    filters: Omit<ArtifactFilters, 'project_id'> = {}
  ): Promise<ArtifactListResponse> {
    const params = new URLSearchParams()
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined) {
        params.append(key, String(value))
      }
    })

    const queryString = params.toString()
    const endpoint = `/${encodeURIComponent(teamId)}/artifacts/${encodeURIComponent(projectId)}${queryString ? `?${queryString}` : ''}`

    return apiClient.get<ArtifactListResponse>(endpoint)
  }

  async getArtifact(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<Artifact> {
    return apiClient.get<Artifact>(
      `/${encodeURIComponent(teamId)}/artifacts/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}`
    )
  }

  async createArtifact(
    teamId: string,
    data: CreateArtifactRequest
  ): Promise<Artifact> {
    return apiClient.post<Artifact>(
      `/${encodeURIComponent(teamId)}/artifacts`,
      data
    )
  }

  async updateArtifact(
    teamId: string,
    projectId: string,
    slug: string,
    data: UpdateArtifactRequest
  ): Promise<Artifact> {
    return apiClient.put<Artifact>(
      `/${encodeURIComponent(teamId)}/artifacts/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}`,
      data
    )
  }

  async deleteArtifact(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<void> {
    await apiClient.delete(
      `/${encodeURIComponent(teamId)}/artifacts/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}`
    )
  }

  async getArtifactVersions(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<ArtifactVersionListResponse> {
    return apiClient.get<ArtifactVersionListResponse>(
      `/${encodeURIComponent(teamId)}/artifacts/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}/versions`
    )
  }

  async getArtifactVersion(
    teamId: string,
    projectId: string,
    slug: string,
    versionNumber: number
  ): Promise<ArtifactVersion> {
    return apiClient.get<ArtifactVersion>(
      `/${encodeURIComponent(teamId)}/artifacts/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}/versions/${encodeURIComponent(versionNumber)}`
    )
  }

  async restoreArtifactVersion(
    teamId: string,
    projectId: string,
    slug: string,
    versionNumber: number
  ): Promise<Artifact> {
    return apiClient.post<Artifact>(
      `/${encodeURIComponent(teamId)}/artifacts/${encodeURIComponent(projectId)}/${encodeURIComponent(slug)}/versions/${encodeURIComponent(versionNumber)}/restore`
    )
  }

  async getArtifactStats(teamId: string): Promise<ArtifactStatsResponse> {
    try {
      return await apiClient.get<ArtifactStatsResponse>(
        `/${encodeURIComponent(teamId)}/artifacts/stats`
      )
    } catch (error) {
      console.error('Failed to fetch artifact stats:', error)
      return {
        total_projects: 0,
        total_artifacts: 0,
        added_this_week: 0,
        total_by_type: {},
        total_by_status: {},
      }
    }
  }
}

export const artifactService = new ArtifactService()
