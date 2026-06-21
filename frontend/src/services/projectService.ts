import { apiClient } from '../lib/apiClient'
import type {
  CreateProjectRequest,
  Project,
  ProjectFilters,
  ProjectListResponse,
  ProjectStats,
  UpdateProjectRequest,
} from '../types/project'

class ProjectService {
  async getProjects(
    teamId: string,
    filters: ProjectFilters = {}
  ): Promise<ProjectListResponse> {
    const params = new URLSearchParams()

    // Remove team_id from query params - it's now in the URL path
    if (filters.search) params.append('search', filters.search)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())
    if (filters.sort_by) params.append('sort_by', filters.sort_by)
    if (filters.sort_order) params.append('sort_order', filters.sort_order)

    const queryString = params.toString()
    const endpoint = `/${encodeURIComponent(teamId)}/projects${queryString ? `?${queryString}` : ''}`

    return apiClient.get<ProjectListResponse>(endpoint)
  }

  async getProject(teamId: string, slug: string): Promise<Project> {
    return apiClient.get<Project>(
      `/${encodeURIComponent(teamId)}/projects/${encodeURIComponent(slug)}`
    )
  }

  async createProject(
    teamId: string,
    data: CreateProjectRequest
  ): Promise<Project> {
    return apiClient.post<Project>(
      `/${encodeURIComponent(teamId)}/projects`,
      data
    )
  }

  async updateProject(
    teamId: string,
    slug: string,
    data: UpdateProjectRequest
  ): Promise<Project> {
    return apiClient.put<Project>(
      `/${encodeURIComponent(teamId)}/projects/${encodeURIComponent(slug)}`,
      data
    )
  }

  async deleteProject(teamId: string, slug: string): Promise<void> {
    await apiClient.delete(
      `/${encodeURIComponent(teamId)}/projects/${encodeURIComponent(slug)}`
    )
  }

  async getProjectStats(teamId: string, slug: string): Promise<ProjectStats> {
    return apiClient.get<ProjectStats>(
      `/${encodeURIComponent(teamId)}/projects/${encodeURIComponent(slug)}/stats`
    )
  }
}

export const projectService = new ProjectService()
