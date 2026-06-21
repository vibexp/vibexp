package resourceaccess

// Source constants identify where a resource detail-access request originated.
const (
	// SourceWeb is the web application (cookie-authenticated).
	SourceWeb = "web"
	// SourceCLI is the VibeXP CLI (API-key authenticated, VibeXP-CLI user agent).
	SourceCLI = "cli"
	// SourceMCP is the MCP server (API-key authenticated, request path under /mcp/).
	SourceMCP = "mcp"
	// SourceAPI is any other programmatic API consumer (the catch-all default).
	SourceAPI = "api"
)

// SourceMetricPoint is a single source's count for a single day in a metrics series.
type SourceMetricPoint struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// DailyMetrics holds the per-source counts for one calendar day (UTC).
// The Sources slice is zero-filled: every source the series tracks has a point,
// even when its count is zero, so consumers never have to reason about gaps.
type DailyMetrics struct {
	Date    string              `json:"date"`
	Sources []SourceMetricPoint `json:"sources"`
}

// MetricsResult is the service-layer response for a resource's access metrics
// over a contiguous range of days. Days is ordered oldest-to-newest and contains
// one entry per day in the range with no gaps.
type MetricsResult struct {
	TeamID       string         `json:"team_id"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	RangeDays    int            `json:"range_days"`
	Days         []DailyMetrics `json:"days"`
}
