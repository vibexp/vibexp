package models

import "time"

// Attachment is a generic, polymorphic file attachment for a resource. It is
// keyed by (OwnerType, OwnerID) so any resource type can own attachments
// without a schema change. The binary lives in GCS at GCSObjectKey; this row
// holds only metadata.
type Attachment struct {
	ID        string `json:"id" db:"id"`
	TeamID    string `json:"team_id" db:"team_id"`
	UserID    string `json:"user_id,omitempty" db:"user_id"`
	OwnerType string `json:"owner_type" db:"owner_type"`
	OwnerID   string `json:"owner_id" db:"owner_id"`
	FileName  string `json:"file_name" db:"file_name"`
	// RelativePath is the file's path relative to its owner's directory (e.g.
	// "scripts/helper.py" for a multi-file skill companion). Empty for a plain
	// attachment; stored verbatim after traversal validation. FileName stays the
	// basename regardless (#338).
	RelativePath string `json:"relative_path,omitempty" db:"relative_path"`
	ContentType  string `json:"content_type" db:"content_type"`
	SizeBytes    int64  `json:"size_bytes" db:"size_bytes"`
	// GCSObjectKey is the storage location; never exposed to API clients.
	GCSObjectKey string    `json:"-" db:"gcs_object_key"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// AttachmentListResponse is the response body for listing an owner's attachments.
type AttachmentListResponse struct {
	Attachments    JSONArray[Attachment] `json:"attachments"`
	TotalCount     int                   `json:"total_count"`
	TotalSizeBytes int64                 `json:"total_size_bytes"`
}
