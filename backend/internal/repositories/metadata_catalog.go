package repositories

// MetadataResourceType names one of the three metadata-bearing resource types
// the metadata catalog can enumerate. It is a closed set: the value selects a
// table from a fixed map in the repository, so an unrecognised value must be
// rejected before any SQL is built.
type MetadataResourceType string

// The recognised metadata resource types.
const (
	MetadataResourceArtifacts  MetadataResourceType = "artifacts"
	MetadataResourceBlueprints MetadataResourceType = "blueprints"
	MetadataResourceMemories   MetadataResourceType = "memories"
)

// ParseMetadataResourceType converts a raw request value into a
// MetadataResourceType, reporting whether it is one of the recognised values.
func ParseMetadataResourceType(raw string) (MetadataResourceType, bool) {
	switch MetadataResourceType(raw) {
	case MetadataResourceArtifacts:
		return MetadataResourceArtifacts, true
	case MetadataResourceBlueprints:
		return MetadataResourceBlueprints, true
	case MetadataResourceMemories:
		return MetadataResourceMemories, true
	default:
		return "", false
	}
}

// MetadataCatalogQuery is the common shape of both catalog lookups.
type MetadataCatalogQuery struct {
	// UserID is the caller, used for the tenancy predicate.
	UserID string
	// TeamID scopes the lookup to one team.
	TeamID string
	// ResourceType selects the table to enumerate.
	ResourceType MetadataResourceType
	// ProjectID optionally narrows the lookup to a single project.
	ProjectID *string
	// Key is the metadata key whose values to enumerate. Values lookups only.
	Key string
	// Search is an optional case-insensitive substring filter for typeahead.
	// Values lookups only.
	Search *string
	// Limit bounds the number of entries returned.
	Limit int
}

// MetadataCatalogResult is a page of catalog entries plus whether more exist.
type MetadataCatalogResult struct {
	Entries   []string
	Truncated bool
}
