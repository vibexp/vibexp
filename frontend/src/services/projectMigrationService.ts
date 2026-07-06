import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the project-migration domain — the OpenAPI spec is
// the single source of truth; do not hand-write request/response shapes here.
export type ConflictPolicy = MigrationRequest['conflict_policy']
export type ResourceSelection = components['schemas']['ResourceSelection']
export type ResourceSelections = components['schemas']['ResourceSelections']
export type MigrationRequest = components['schemas']['MigrationRequest']
export type ResourceInventoryItem =
  components['schemas']['ResourceInventoryItem']
export type ResourceInventory = components['schemas']['ResourceInventory']
export type MigrationInventory = components['schemas']['MigrationInventory']
export type ResourceOutcome = components['schemas']['ResourceOutcome']
export type ResourceMigrationCounts =
  components['schemas']['ResourceMigrationCounts']
export type ResourceMigrationOutcomes =
  components['schemas']['ResourceMigrationOutcomes']
export type MigrationResult = components['schemas']['MigrationResult']

class ProjectMigrationService {
  /** Fetch resource inventory counts for a project before initiating migration. */
  async getInventory(
    teamId: string,
    projectId: string
  ): Promise<MigrationInventory> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/projects/{project_id}/migration/inventory',
        { params: { path: { team_id: teamId, project_id: projectId } } }
      )
    )
  }

  /** Execute the migration and return the outcome. */
  async migrate(
    teamId: string,
    projectId: string,
    request: MigrationRequest
  ): Promise<MigrationResult> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/projects/{project_id}/migration',
        {
          params: { path: { team_id: teamId, project_id: projectId } },
          body: request,
        }
      )
    )
  }
}

export const projectMigrationService = new ProjectMigrationService()
