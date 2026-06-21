// Resource-type-agnostic, team-customizable categories ("types"). System
// defaults are global and read-only (is_system true, no team_id); custom types
// belong to a team. Mirrors the backend `types` table (#1846).
export interface Type {
  id: string
  team_id?: string
  resource_type: string
  slug: string
  name: string
  is_system: boolean
  created_at: string
}

export interface CreateTypeRequest {
  resource_type: string
  slug: string
  name: string
}

export interface TypeListResponse {
  types: Type[]
  total_count: number
}
