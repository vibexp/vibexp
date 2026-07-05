import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the projects domain — the OpenAPI spec is the single
// source of truth; do not hand-write request/response shapes here.
//
// The widely-imported `Project` name maps to the generated `ProjectResponse`
// (`Project & { github_connected: boolean }`) so existing call sites keep the
// computed `github_connected` field they already rely on.
export type Project = components['schemas']['ProjectResponse']
export type ProjectResponse = components['schemas']['ProjectResponse']
export type CreateProjectRequest = components['schemas']['CreateProjectRequest']
export type UpdateProjectRequest = components['schemas']['UpdateProjectRequest']
export type ProjectListResponse = components['schemas']['ProjectListResponse']
export type ProjectStatsResponse = components['schemas']['ProjectStatsResponse']
export type ProjectFilters = NonNullable<
  operations['listProjects']['parameters']['query']
>

class ProjectService {
  async getProjects(
    teamId: string,
    filters: ProjectFilters = {}
  ): Promise<ProjectListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/projects', {
        params: { path: { team_id: teamId }, query: filters },
      })
    )
  }

  async getProject(teamId: string, slug: string): Promise<Project> {
    // The single-project endpoints are documented as returning the bare
    // `Project` schema, but the backend also populates `github_connected`;
    // surface it as `ProjectResponse` (the frontend `Project`) so consumers
    // keep reading the computed field.
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/projects/{slug}', {
        params: { path: { team_id: teamId, slug } },
      })
    ) as Promise<Project>
  }

  async createProject(
    teamId: string,
    data: CreateProjectRequest
  ): Promise<Project> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/projects', {
        params: { path: { team_id: teamId } },
        body: data,
      })
    ) as Promise<Project>
  }

  async updateProject(
    teamId: string,
    slug: string,
    data: UpdateProjectRequest
  ): Promise<Project> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/projects/{slug}', {
        params: { path: { team_id: teamId, slug } },
        body: data,
      })
    ) as Promise<Project>
  }

  async deleteProject(teamId: string, slug: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/projects/{slug}', {
        params: { path: { team_id: teamId, slug } },
      })
    )
  }

  async getProjectStats(
    teamId: string,
    slug: string
  ): Promise<ProjectStatsResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/projects/{slug}/stats', {
        params: { path: { team_id: teamId, slug } },
      })
    )
  }
}

export const projectService = new ProjectService()
