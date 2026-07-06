import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import type {
  ResourceVersion,
  ResourceVersionListResponse,
} from '../types/version'

// Generated wire types for the prompts domain — the OpenAPI spec is the single
// source of truth; do not hand-write request/response shapes here.
export type Prompt = components['schemas']['Prompt']
export type CreatePromptRequest = components['schemas']['CreatePromptRequest']
export type UpdatePromptRequest = components['schemas']['UpdatePromptRequest']
export type PromptListResponse = components['schemas']['PromptListResponse']
export type RenderPromptRequest = components['schemas']['RenderPromptRequest']
export type RenderPromptResponse = components['schemas']['RenderPromptResponse']
export type PromptFilters = NonNullable<
  operations['listPrompts']['parameters']['query']
>

// Prompt versions are the generic resource version with `resource_type` ===
// "prompt". Kept as aliases so prompt call sites read naturally while the
// version-history module works against the resource-agnostic types.
export type PromptVersion = ResourceVersion
export type PromptVersionListResponse = ResourceVersionListResponse

// Prompt dependency types. The dependencies endpoint is documented in the spec
// as returning `{ dependencies }`, but the backend actually returns the
// `{ used_by, uses }` split the dependency UI consumes (and that the
// pre-migration client already read). We keep these local types plus a small
// adapter to preserve that contract.
export interface PromptDependencyInfo {
  id: string
  slug: string
  name: string
}

export interface PromptDependenciesResponse {
  used_by: PromptDependencyInfo[] // Prompts that reference this prompt
  uses: PromptDependencyInfo[] // Prompts that this prompt references
}

class PromptService {
  async getPrompts(
    teamId: string,
    filters: PromptFilters = {}
  ): Promise<PromptListResponse> {
    return (
      await unwrap(
        generatedClient.GET('/api/v1/{team_id}/prompts', {
          params: { path: { team_id: teamId }, query: filters },
        })
      )
    ).data
  }

  async getPrompt(teamId: string, slug: string): Promise<Prompt> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/prompts/{slug}', {
        params: { path: { team_id: teamId, slug } },
      })
    )
  }

  async createPrompt(
    teamId: string,
    data: CreatePromptRequest
  ): Promise<Prompt> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/prompts', {
        params: { path: { team_id: teamId } },
        body: data,
      })
    )
  }

  async updatePrompt(
    teamId: string,
    slug: string,
    data: UpdatePromptRequest
  ): Promise<Prompt> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/prompts/{slug}', {
        params: { path: { team_id: teamId, slug } },
        body: data,
      })
    )
  }

  async deletePrompt(teamId: string, slug: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/prompts/{slug}', {
        params: { path: { team_id: teamId, slug } },
      })
    )
  }

  async getPromptPlaceholders(teamId: string, slug: string): Promise<string[]> {
    return (
      await unwrap(
        generatedClient.GET('/api/v1/{team_id}/prompts/{slug}/placeholders', {
          params: { path: { team_id: teamId, slug } },
        })
      )
    ).placeholders
  }

  async renderPrompt(
    teamId: string,
    slug: string,
    placeholders: Record<string, string>
  ): Promise<RenderPromptResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/prompts/{slug}/render', {
        params: { path: { team_id: teamId, slug } },
        body: { placeholders },
      })
    )
  }

  async getPromptDependencies(
    teamId: string,
    slug: string
  ): Promise<PromptDependenciesResponse> {
    const data = (await unwrap(
      generatedClient.GET('/api/v1/{team_id}/prompts/{slug}/dependencies', {
        params: { path: { team_id: teamId, slug } },
      })
    )) as unknown as Partial<PromptDependenciesResponse> | undefined

    // Always return arrays, never null/undefined.
    return {
      used_by: data?.used_by ?? [],
      uses: data?.uses ?? [],
    }
  }

  // Content version history. Slug-addressed within a team (no project_id),
  // mirroring the prompt CRUD endpoints. The versioned content is the raw body
  // template.
  async getPromptVersions(
    teamId: string,
    slug: string
  ): Promise<PromptVersionListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/prompts/{slug}/versions', {
        params: { path: { team_id: teamId, slug } },
      })
    )
  }

  async getPromptVersion(
    teamId: string,
    slug: string,
    versionNumber: number
  ): Promise<PromptVersion> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/prompts/{slug}/versions/{version_number}',
        {
          params: {
            path: { team_id: teamId, slug, version_number: versionNumber },
          },
        }
      )
    )
  }

  async restorePromptVersion(
    teamId: string,
    slug: string,
    versionNumber: number
  ): Promise<Prompt> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/prompts/{slug}/versions/{version_number}/restore',
        {
          params: {
            path: { team_id: teamId, slug, version_number: versionNumber },
          },
        }
      )
    )
  }

  async getPromptLabels(teamId: string): Promise<string[]> {
    return (
      await unwrap(
        generatedClient.GET('/api/v1/{team_id}/prompts/labels', {
          params: { path: { team_id: teamId } },
        })
      )
    ).data.labels
  }
}

export const promptService = new PromptService()
