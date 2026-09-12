import type { MCPTool } from './mcp-tool-shared'
import { TEAM_ID_DESCRIPTION } from './mcp-tool-shared'

/**
 * The keyed resource delete tool, kept in its own module so the main catalog
 * stays under the 600-line cap the lint config enforces -- the same split
 * mcp-tools-memory.ts made for the memory write tools.
 */
export const resourceTools: MCPTool[] = [
  {
    name: 'vibexp_io_delete_resource',
    description:
      'Delete a single resource. Supported resource_type values and their required identifiers: "memory" needs id; "prompt" needs slug; "artifact" and "blueprint" need project_id and slug. The team is resolved and membership-checked per call; the resource\'s embeddings (and an artifact\'s attachments) are removed alongside it.',
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
            'The resource type to delete: one of "memory", "artifact", "blueprint", or "prompt".',
        },
        id: {
          type: 'string',
          description:
            'Resource UUID. Required when resource_type is "memory"; ignored otherwise.',
        },
        project_id: {
          type: 'string',
          description:
            'Project UUID. Required when resource_type is "artifact" or "blueprint"; ignored otherwise.',
        },
        slug: {
          type: 'string',
          description:
            'Resource slug. Required when resource_type is "prompt", "artifact", or "blueprint"; ignored otherwise.',
        },
      },
      required: ['team_id', 'resource_type'],
      additionalProperties: false,
    },
  },
]
