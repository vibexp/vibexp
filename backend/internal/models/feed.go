package models

import "time"

// FeedItemReply represents a threaded reply to a feed item
type FeedItemReply struct {
	ID              string    `json:"id"                         db:"id"`
	TeamID          string    `json:"team_id"                    db:"team_id"`
	FeedItemID      string    `json:"feed_item_id"               db:"feed_item_id"`
	Content         string    `json:"content"                    db:"content"`
	PostedByUserID  string    `json:"posted_by_user_id"          db:"posted_by_user_id"`
	AIAssistantName *string   `json:"ai_assistant_name,omitempty" db:"ai_assistant_name"`
	PostedAt        time.Time `json:"posted_at"                  db:"posted_at"`
}

// CreateFeedItemReplyRequest is the request body for posting a reply to a feed item
type CreateFeedItemReplyRequest struct {
	Content         string  `json:"content"                    validate:"required,min=1,max=10000"`
	AIAssistantName *string `json:"ai_assistant_name,omitempty" validate:"omitempty,max=30"`
}

// FeedItemReplyListResponse is the paginated response for listing feed item replies
type FeedItemReplyListResponse struct {
	Replies    JSONArray[FeedItemReply] `json:"replies"`
	TotalCount int                      `json:"total_count"`
	Page       int                      `json:"page"`
	PerPage    int                      `json:"per_page"`
	TotalPages int                      `json:"total_pages"`
}

// Feed represents a team-scoped AI feed channel
type Feed struct {
	ID              string    `json:"id" db:"id"`
	TeamID          string    `json:"team_id" db:"team_id"`
	Name            string    `json:"name" db:"name"`
	Description     *string   `json:"description,omitempty" db:"description"`
	CreatedByUserID string    `json:"created_by_user_id" db:"created_by_user_id"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// FeedItem represents a single item posted to a feed
type FeedItem struct {
	ID              string     `json:"id" db:"id"`
	TeamID          string     `json:"team_id" db:"team_id"`
	FeedID          string     `json:"feed_id" db:"feed_id"`
	ProjectID       *string    `json:"project_id,omitempty" db:"project_id"`
	Title           string     `json:"title" db:"title"`
	Content         string     `json:"content" db:"content"`
	Excerpt         string     `json:"excerpt" db:"excerpt"`
	AIAssistantName string     `json:"ai_assistant_name" db:"ai_assistant_name"`
	PostedByUserID  string     `json:"posted_by_user_id" db:"posted_by_user_id"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty" db:"archived_at"`
	PostedAt        time.Time  `json:"posted_at" db:"posted_at"`
	ReplyCount      int        `json:"reply_count" db:"-"`
}

// CreateFeedRequest is the request body for creating a new feed
type CreateFeedRequest struct {
	Name        string  `json:"name" validate:"required,min=1,max=255"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=1000"`
}

// UpdateFeedRequest is the request body for updating an existing feed
type UpdateFeedRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=1000"`
}

// CreateFeedItemRequest is the request body for posting an item to a feed
type CreateFeedItemRequest struct {
	Title           string  `json:"title" validate:"required,min=1,max=255"`
	Content         string  `json:"content" validate:"required,min=1"`
	AIAssistantName string  `json:"ai_assistant_name" validate:"required,min=1,max=30"`
	ProjectID       *string `json:"project_id,omitempty" validate:"omitempty,uuid"`
}

// FeedListResponse is the paginated response for listing feeds
type FeedListResponse struct {
	Feeds      JSONArray[Feed] `json:"feeds"`
	TotalCount int             `json:"total_count"`
	Page       int             `json:"page"`
	PerPage    int             `json:"per_page"`
	TotalPages int             `json:"total_pages"`
}

// FeedItemListResponse is the paginated response for listing feed items
type FeedItemListResponse struct {
	Items      JSONArray[FeedItem] `json:"items"`
	TotalCount int                 `json:"total_count"`
	Page       int                 `json:"page"`
	PerPage    int                 `json:"per_page"`
	TotalPages int                 `json:"total_pages"`
}

// FeedWithLastPost is a Feed enriched with the timestamp of the most recent post.
// It is returned by the MCP list-feeds tool and does NOT alter the REST API response shape.
type FeedWithLastPost struct {
	ID              string     `json:"id"`
	TeamID          string     `json:"team_id"`
	Name            string     `json:"name"`
	Description     *string    `json:"description,omitempty"`
	CreatedByUserID string     `json:"created_by_user_id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastPostAt      *time.Time `json:"last_post_at"`
}

// MCPFeedListResponse is the response returned by the vibexp_io_list_feeds MCP tool.
type MCPFeedListResponse struct {
	Feeds []FeedWithLastPost `json:"feeds"`
}
