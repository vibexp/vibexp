package models

// ClearEmbeddingsResponse is returned when a team's stored embeddings are cleared
// (truncated). DeletedCount is how many embedding rows were removed. Clearing does
// not regenerate anything — the team's content stays unembedded (and semantic
// search returns nothing for it) until a provider reprocess/re-embed runs.
type ClearEmbeddingsResponse struct {
	DeletedCount int64 `json:"deleted_count"`
}
