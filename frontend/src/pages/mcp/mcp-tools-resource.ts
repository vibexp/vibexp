import type { MCPTool } from './mcp-tool-shared'
import { TEAM_ID_DESCRIPTION } from './mcp-tool-shared'

/**
 * The keyed resource tools -- the three that dispatch on `resource_type` rather
 * than belonging to one domain -- kept in their own module so the main catalog
 * stays under the 600-line cap the lint config enforces, the same split
 * mcp-tools-memory.ts made for the memory write tools.
 */
export const resourceTools: MCPTool[] = [
  {
    name: 'vibexp_io_get_resource',
    description:
      'Fetch a single resource by type and identifier, returning its full content. Supported resource_type values: "memory" (identified by id), "artifact" and "blueprint" (identified by project_id and slug). This replaces the former per-type get tools.',
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
            'The resource type to fetch: one of "memory", "artifact", or "blueprint".',
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
            'Resource slug. Required when resource_type is "artifact" or "blueprint"; ignored otherwise.',
        },
      },
      required: ['team_id', 'resource_type'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_list_resources',
    description:
      'List the resources of a given type in a project with filtering and pagination. Returns slim items without full content — call vibexp_io_get_resource for the full content of a single resource. Supported resource_type values: "memory", "artifact", "blueprint". This replaces the former per-type search/list tools.',
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
            'The resource type to list: one of "memory", "artifact", or "blueprint".',
        },
        project_id: {
          type: 'string',
          description: 'Project UUID to list resources within.',
        },
        page: {
          type: 'integer',
          description: 'Page number for pagination (default: 1).',
        },
        limit: {
          type: 'integer',
          description: 'Number of items per page (default: 10, max: 10).',
        },
        search: {
          type: 'string',
          description:
            'Search term matched against memory text, or artifact/blueprint title and description.',
        },
        status: {
          type: 'string',
          description: 'Filter by status. Leave empty for the default view.',
        },
        type: {
          type: 'string',
          description: 'Filter by type (artifact and blueprint only).',
        },
        metadata: {
          type: 'object',
          description:
            'Filter by metadata: an object of key to array of string values, e.g. {"env":["prod","staging"],"scope":["backend"]}. Keys are combined with AND, values within a key with OR. An empty array means "the key exists". Array-valued metadata matches element-wise, and numeric or boolean values match their string form. At most 10 keys, 25 values per key, key length 255, value length 512.',
        },
      },
      required: ['team_id', 'resource_type', 'project_id'],
      additionalProperties: false,
    },
  },
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
