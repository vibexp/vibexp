// Project types - matching backend API schema

export interface Project {
  id: string
  user_id: string
  team_id: string
  name: string
  slug: string
  description: string
  git_url: string
  homepage: string
  created_at: string
  updated_at: string
  version: number
  github_connected?: boolean
}

export interface CreateProjectRequest {
  name: string
  slug: string
  description?: string
  git_url?: string
  homepage?: string
}

export interface UpdateProjectRequest {
  name?: string
  slug?: string
  description?: string
  git_url?: string
  homepage?: string
}

export interface ProjectFilters {
  search?: string
  page?: number
  limit?: number
  sort_by?: 'created_at' | 'updated_at' | 'name' | 'slug'
  sort_order?: 'asc' | 'desc'
}

export interface ProjectListResponse {
  projects: Project[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}

export interface ProjectStats {
  total_prompts: number
  total_artifacts: number
  total_blueprints: number
  total_memories: number
  total_feed_items: number
}
