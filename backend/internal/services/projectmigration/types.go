package projectmigration

// ConflictPolicy defines how slug collisions are handled during migration.
type ConflictPolicy string

const (
	// ConflictPolicySkip excludes colliding resources from the migration.
	ConflictPolicySkip ConflictPolicy = "skip"
	// ConflictPolicyRename appends a "-moved" suffix to the source slug before moving.
	ConflictPolicyRename ConflictPolicy = "rename"
	// ConflictPolicyOverwrite deletes the destination resource and moves the source in its place.
	ConflictPolicyOverwrite ConflictPolicy = "overwrite"
)

// ResourceSelection specifies which resources of a given type to migrate.
type ResourceSelection struct {
	// All indicates every resource in the source project should be migrated.
	All bool `json:"all"`
	// IDs is an explicit list of resource IDs to migrate. Used when All is false.
	IDs []string `json:"ids,omitempty"`
}

// ResourceSelections groups per-resource-type selections for a migration request.
type ResourceSelections struct {
	Prompts    ResourceSelection `json:"prompts"`
	Artifacts  ResourceSelection `json:"artifacts"`
	Blueprints ResourceSelection `json:"blueprints"`
	FeedItems  ResourceSelection `json:"feed_items"`
}

// MigrationRequest is the body of the POST migration endpoint.
type MigrationRequest struct {
	DestinationProjectID string             `json:"destination_project_id"`
	Resources            ResourceSelections `json:"resources"`
	ConflictPolicy       ConflictPolicy     `json:"conflict_policy"`
}

// ResourceInventoryItem is a lightweight representation of a single resource.
type ResourceInventoryItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ResourceInventory holds a count and item list for a single resource type.
type ResourceInventory struct {
	Count int                     `json:"count"`
	Items []ResourceInventoryItem `json:"items,omitempty"`
}

// MigrationInventory is returned by GET migration/inventory.
type MigrationInventory struct {
	Prompts    ResourceInventory `json:"prompts"`
	Artifacts  ResourceInventory `json:"artifacts"`
	Blueprints ResourceInventory `json:"blueprints"`
	FeedItems  ResourceInventory `json:"feed_items"`
}

// ResourceOutcome records why a specific resource was skipped or failed.
type ResourceOutcome struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// ResourceMigrationCounts holds the count of successfully migrated resources per type.
type ResourceMigrationCounts struct {
	Prompts    int `json:"prompts"`
	Artifacts  int `json:"artifacts"`
	Blueprints int `json:"blueprints"`
	FeedItems  int `json:"feed_items"`
}

// ResourceMigrationOutcomes holds the list of resources that were skipped or failed per type.
type ResourceMigrationOutcomes struct {
	Prompts    []ResourceOutcome `json:"prompts,omitempty"`
	Artifacts  []ResourceOutcome `json:"artifacts,omitempty"`
	Blueprints []ResourceOutcome `json:"blueprints,omitempty"`
	FeedItems  []ResourceOutcome `json:"feed_items,omitempty"`
}

// MigrationResult is returned by POST migration.
type MigrationResult struct {
	Migrated               ResourceMigrationCounts   `json:"migrated"`
	Skipped                ResourceMigrationOutcomes `json:"skipped"`
	Failed                 ResourceMigrationOutcomes `json:"failed"`
	SourceProjectName      string                    `json:"source_project_name"`
	DestinationProjectName string                    `json:"destination_project_name"`
}
