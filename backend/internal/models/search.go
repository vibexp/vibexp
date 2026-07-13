package models

import "time"

// SearchRequest is the request body for the team-scoped semantic search endpoint.
// Types use the plural resource names (prompts, artifacts, blueprints, memories);
// they are mapped to the singular embeddings entity_type values internally.
// ProjectID, when set, restricts results to a single project across every type.
type SearchRequest struct {
	Query     string   `json:"query" validate:"required,max=1000"`
	Types     []string `json:"types" validate:"omitempty,dive,oneof=prompts artifacts blueprints memories"`
	ProjectID string   `json:"project_id" validate:"omitempty,uuid"`
	Page      int      `json:"page"`
	PerPage   int      `json:"per_page"`
}

// SearchResultRow is a single semantic-search match returned by the repository.
// It carries the raw chunk content and cosine distance so the service can derive
// the excerpt and relevance score.
type SearchResultRow struct {
	EntityType   string    `db:"entity_type"`
	EntityID     string    `db:"entity_id"`
	ChunkID      string    `db:"chunk_id"`
	Title        string    `db:"title"`
	Slug         string    `db:"slug"`
	ProjectID    string    `db:"project_id"`
	ProjectName  string    `db:"project_name"`
	ChunkContent string    `db:"chunk_content"`
	SourceBody   string    `db:"source_body"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
	Distance     float64   `db:"distance"`
}

// SearchResultItem is a single relevance-ranked result in the search response.
// Slug carries the resource's own slug (” for memories, which route by id).
// ProjectID is the parent project's UUID, used to build the artifact and blueprint
// detail routes (which are keyed by project UUID, not slug); ProjectName is the
// human-readable project label shown alongside each result. Every entity type
// belongs to exactly one project, so both project fields are always populated.
type SearchResultItem struct {
	Type        string    `json:"type"`
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	ProjectID   string    `json:"project_id"`
	ProjectName string    `json:"project_name"`
	Excerpt     string    `json:"excerpt"`
	Score       float64   `json:"score"`
	ChunkID     string    `json:"chunk_id"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SearchResultsResponse is the paginated semantic-search response envelope.
type SearchResultsResponse struct {
	Results    JSONArray[SearchResultItem] `json:"results"`
	TotalCount int                         `json:"total_count"`
	Page       int                         `json:"page"`
	PerPage    int                         `json:"per_page"`
	TotalPages int                         `json:"total_pages"`
}
