package resourceaccess

import "strings"

// Auth method values as stored on the request context by the auth middleware
// (see internal/server/middleware.go and mcp_oauth.go).
const (
	authMethodCookie = "cookie"
	authMethodAPIKey = "api_key"
)

// cliUserAgentPrefix marks requests originating from the VibeXP CLI.
const cliUserAgentPrefix = "VibeXP-CLI/"

// mcpPathSegment marks requests routed through the MCP server.
const mcpPathSegment = "/mcp/"

// DeriveSource classifies the origin of a resource detail-access request.
//
// The inputs are the primitives the auth middleware already resolves, so the
// function stays pure and unit-testable without an *http.Request:
//   - authMethod: the context auth_type value ("cookie", "api_key", or "oauth").
//   - path: the request URL path.
//   - userAgent: the request User-Agent header.
//
// Rules, in priority order:
//  1. cookie auth                                  -> SourceWeb
//  2. api_key auth AND path contains "/mcp/"       -> SourceMCP
//  3. api_key auth AND UA starts with "VibeXP-CLI/" -> SourceCLI
//  4. otherwise (incl. oauth — native API clients) -> SourceAPI
//
// "oauth" deliberately classifies as SourceAPI: AuthKit-JWT clients (mobile)
// are programmatic API consumers, and the MCP mount never routes through the
// resource-access middleware.
func DeriveSource(authMethod, path, userAgent string) string {
	if authMethod == authMethodCookie {
		return SourceWeb
	}

	if authMethod == authMethodAPIKey {
		if strings.Contains(path, mcpPathSegment) {
			return SourceMCP
		}
		if strings.HasPrefix(userAgent, cliUserAgentPrefix) {
			return SourceCLI
		}
	}

	return SourceAPI
}
