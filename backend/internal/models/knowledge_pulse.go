package models

import "time"

// Knowledge Pulse raw rows. The repository reads them from the
// tables that already record activity; the service turns them into the
// published event shape. Absent values are empty strings, never NULL, because
// the queries COALESCE every optional column.

// PulseResource identifies a resource and carries what the client needs to
// render and deep-link it. Title is empty when the resource no longer exists.
type PulseResource struct {
	Type      string
	ID        string
	Title     string
	Slug      string
	ProjectID string
}

// PulseActorRaw is everything the tables record about who caused an event.
// The service decides from it whether the actor is a person or an agent.
type PulseActorRaw struct {
	Source        string
	UserID        string
	UserName      string
	APIKeyID      string
	APIKeyName    string
	UserAgent     string
	AssistantName string
}

// PulseReadEvent is one resource_access_events row.
type PulseReadEvent struct {
	ID       string
	At       time.Time
	Resource PulseResource
	Actor    PulseActorRaw
}

// PulseWriteEvent is one resource_write_events row.
type PulseWriteEvent struct {
	ID       string
	At       time.Time
	Action   string
	Resource PulseResource
	Actor    PulseActorRaw
}

// PulseLinkEvent is one resource_relations row.
type PulseLinkEvent struct {
	ID            string
	At            time.Time
	RelationType  string
	Origin        string
	CreatedByID   string
	CreatedByName string
	From          PulseResource
	To            PulseResource
}

// PulseFeedEvent is one feed_items row.
type PulseFeedEvent struct {
	ID            string
	At            time.Time
	Title         string
	ProjectID     string
	AssistantName string
	UserID        string
	UserName      string
}

// PulseNode is a snapshot node: a resource and how many relations it is in.
type PulseNode struct {
	Resource PulseResource
	Degree   int
}

// PulseEdge is a snapshot edge between two node ids.
type PulseEdge struct {
	FromID       string
	ToID         string
	RelationType string
}

// PulseSnapshotRaw is the graph snapshot before actors are resolved.
type PulseSnapshotRaw struct {
	Nodes  []PulseNode
	Edges  []PulseEdge
	Actors []PulseActorRaw
}

// PulseDayStats are the raw counts behind stats_today.
type PulseDayStats struct {
	Reads         int
	Writes        int
	Links         int
	Supersedes    int
	LinkedUpdates int
}
