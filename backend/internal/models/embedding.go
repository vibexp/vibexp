package models

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

// Embedding represents a vector embedding for an entity
type Embedding struct {
	ID               string          `json:"id" db:"id"`
	EntityType       string          `json:"entity_type" db:"entity_type"`
	EntityID         string          `json:"entity_id" db:"entity_id"`
	VectorEmbeddings pgvector.Vector `json:"vector_embeddings" db:"vector_embeddings"`
	Content          string          `json:"content" db:"content"`
	ModelID          string          `json:"model_id" db:"model_id"`
	UserID           string          `json:"user_id" db:"user_id"`
	TeamID           string          `json:"team_id" db:"team_id"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

// EmbeddingSimilarity represents a similarity search result
type EmbeddingSimilarity struct {
	Embedding
	Distance float64 `json:"distance"`
}

// BackfillEntity carries the fields needed to reconstruct an entity's `.created`
// event during an embedding backfill. It is a union across all embeddable
// entity types: only the fields relevant to a given EntityType are populated by
// the repository, and the backfill service reads exactly those when rebuilding the
// event. ProjectName mirrors how the live services pass project_id into the
// `.created` event constructors. Title/Body/Type/Slug/Email/FeedID are empty for
// entity types that have no such concept.
type BackfillEntity struct {
	EntityType  string
	EntityID    string
	UserID      string
	TeamID      string
	ProjectName string
	FeedID      string
	Slug        string
	Title       string
	Description string
	Body        string
	Type        string
	Email       string
	// Excerpt is the feed item's pre-rendered excerpt; empty for other types.
	Excerpt   string
	CreatedAt time.Time
}
