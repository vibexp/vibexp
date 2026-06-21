import { apiClient } from '../lib/apiClient'
import type {
  MigrateRequest,
  MigrationInventory,
  MigrationResult,
} from '../types/projectMigration'

class ProjectMigrationService {
  /** Fetch resource inventory counts for a project before initiating migration. */
  async getInventory(
    teamId: string,
    projectId: string
  ): Promise<MigrationInventory> {
    return apiClient.get<MigrationInventory>(
      `/${encodeURIComponent(teamId)}/projects/${encodeURIComponent(projectId)}/migration/inventory`
    )
  }

  /** Execute the migration and return the outcome. */
  async migrate(
    teamId: string,
    projectId: string,
    request: MigrateRequest
  ): Promise<MigrationResult> {
    return apiClient.post<MigrationResult>(
      `/${encodeURIComponent(teamId)}/projects/${encodeURIComponent(projectId)}/migration`,
      request
    )
  }
}

export const projectMigrationService = new ProjectMigrationService()
