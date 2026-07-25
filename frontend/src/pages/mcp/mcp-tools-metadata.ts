import type { MCPTool } from './mcp-tool-shared'
import { TEAM_ID_DESCRIPTION } from './mcp-tool-shared'

/**
 * The metadata discovery tool (#525), kept in its own module so the main
 * catalog stays under the 600-line cap the lint config enforces.
 */
export const metadataTools: MCPTool[] = [
  {
    name: 'vibexp_io_list_resource_metadata',
    description:
      "Discover the metadata a team actually uses, so a metadata filter can be built from real keys and values instead of guesses. Omit `key` to list the distinct metadata keys present on a resource type; supply `key` to list that key's distinct values, optionally narrowed by `q`. Values held in an array are flattened and non-string scalars are rendered in their text form, so every value returned can be used directly in the `metadata` filter of vibexp_io_list_resources. `truncated` reports that more exist than were returned. The team is resolved and membership-checked per call, and results never include another team's metadata.",
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        resource_type: {
          type: 'string',
          description:
            'The resource type whose metadata to enumerate: one of "memory", "artifact", or "blueprint".',
        },
        key: {
          type: 'string',
          description:
            "Omit to list the distinct metadata KEYS. Supply a key to list that key's distinct VALUES.",
        },
        project_id: {
          type: 'string',
          description:
            'Optional project UUID; narrows the catalog to a single project.',
        },
        q: {
          type: 'string',
          description:
            'Case-insensitive substring filter, applied when listing values.',
        },
        limit: {
          type: 'integer',
          description: 'Maximum entries to return (default: 100, max: 500).',
        },
      },
      required: ['team_id', 'resource_type'],
      additionalProperties: false,
    },
  },
]
