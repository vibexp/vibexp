/**
 * Shape of an entry in the curated MCP tool catalog, and the one description
 * every team-scoped tool reuses.
 *
 * Split out of `mcp-tools.ts` so the catalog can be assembled from more than one
 * module without an import cycle (the main catalog spreads in per-domain
 * modules, which need these two).
 */
export interface MCPTool {
  name: string
  description: string
  inputSchema: {
    type: string
    properties: Record<
      string,
      {
        type: string
        description: string
      }
    >
    required: string[]
    additionalProperties: boolean
  }
}

export const TEAM_ID_DESCRIPTION =
  'The team (UUID or slug) to operate within. Call vibexp_io_list_teams to discover valid identifiers.'
