import { apiClient } from '../lib/apiClient'
import type { CreateTypeRequest, Type, TypeListResponse } from '../types'

// typeService talks to the resource-type-agnostic /types endpoints (#1846).
// Every call is team-scoped (teamId first), mirroring artifactService.
class TypeService {
  async getTypes(teamId: string, resourceType: string): Promise<Type[]> {
    const response = await apiClient.get<TypeListResponse>(
      `/${encodeURIComponent(teamId)}/types?resource_type=${encodeURIComponent(resourceType)}`
    )
    return response.types
  }

  async createType(teamId: string, data: CreateTypeRequest): Promise<Type> {
    return apiClient.post<Type>(`/${encodeURIComponent(teamId)}/types`, data)
  }

  async deleteType(teamId: string, id: string): Promise<void> {
    await apiClient.delete(
      `/${encodeURIComponent(teamId)}/types/${encodeURIComponent(id)}`
    )
  }
}

export const typeService = new TypeService()
