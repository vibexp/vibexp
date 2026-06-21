/** Conflict resolution strategy when destination has a resource with the same name. */
export type ConflictPolicy = 'skip' | 'rename' | 'overwrite'

export interface ResourceInventoryItem {
  id: string
  name: string
}

export interface ResourceInventory {
  count: number
  items?: ResourceInventoryItem[]
}

/** Counts per resource type returned by the inventory endpoint. */
export interface MigrationInventory {
  prompts: ResourceInventory
  artifacts: ResourceInventory
  blueprints: ResourceInventory
  feed_items: ResourceInventory
}

/** Selector for a resource type — either move all or a specific subset. */
export interface ResourceSelection {
  all: boolean
  ids?: string[]
}

/** Per-type selection payload sent to the migration endpoint. */
export interface MigrationResources {
  prompts: ResourceSelection
  artifacts: ResourceSelection
  blueprints: ResourceSelection
  feed_items: ResourceSelection
}

export interface MigrateRequest {
  destination_project_id: string
  resources: MigrationResources
  conflict_policy: ConflictPolicy
}

export interface ResourceOutcome {
  id: string
  reason: string
}

/** Aggregate migrated counts returned by the migration endpoint. */
export interface MigrationCounts {
  prompts: number
  artifacts: number
  blueprints: number
  feed_items: number
}

export interface MigrationOutcomes {
  prompts?: ResourceOutcome[]
  artifacts?: ResourceOutcome[]
  blueprints?: ResourceOutcome[]
  feed_items?: ResourceOutcome[]
}

/** Full result returned after a migration completes. */
export interface MigrationResult {
  migrated: MigrationCounts
  skipped: MigrationOutcomes
  failed: MigrationOutcomes
}
