package models

// SimilarResource is one embedding-similarity neighbor of a resource, computed
// live at read time (issue #427) — never a stored edge. It is surfaced in the
// optional `similar` array on resource reads, kept strictly distinct from the
// stored typed `related` edges. Score is 1 - cosine_distance (higher = closer).
type SimilarResource struct {
	Type  string  `json:"type"`
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}
