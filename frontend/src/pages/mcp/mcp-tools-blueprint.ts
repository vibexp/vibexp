import type { MCPTool } from './mcp-tool-shared'
import { TEAM_ID_DESCRIPTION } from './mcp-tool-shared'

/**
 * The blueprint write tools, kept in their own module so the main catalog stays
 * under the 600-line cap the lint config enforces -- the same split
 * mcp-tools-memory.ts made for the memory write tools.
 */
export const blueprintTools: MCPTool[] = [
  {
    name: 'vibexp_io_create_blueprint',
    description:
      'Create a new blueprint — a standing rule set an AI tool reads before it works, such as coding conventions, review checklists or a sub-agent definition. Blueprints are the per-tool half of the shared workspace: a team writes the rule once and every connected tool picks it up. Use this tool to publish a rule that should apply to future work rather than a one-off note.',
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        project_id: {
          type: 'string',
          description:
            'Project UUID identifier — the project this blueprint belongs to.',
        },
        slug: {
          type: 'string',
          description:
            "Unique identifier for the blueprint within the project (max 255 chars). Use descriptive slugs like 'go-code-style', 'pr-review-checklist'.",
        },
        title: {
          type: 'string',
          description:
            'Human-readable blueprint title (max 255 chars). Make it clear what rule the blueprint states.',
        },
        content: {
          type: 'string',
          description:
            'Full blueprint content — the rules themselves, in Markdown.',
        },
        description: {
          type: 'string',
          description:
            "Brief description of the blueprint's purpose (max 500 chars). Explain when the rules apply.",
        },
        type: {
          type: 'string',
          description: 'Blueprint type classification. Defaults to "general".',
        },
        subtype: {
          type: 'string',
          description:
            'Optional subtype, e.g. "sub-agents". When it is "sub-agents", metadata.model is required.',
        },
        status: {
          type: 'string',
          description:
            'Blueprint status. One of: "active" for rules in force, "expired" for retired ones.',
        },
        metadata: {
          type: 'object',
          description:
            'Optional key-value metadata pairs, e.g. {"model": "..."} for a sub-agent blueprint.',
        },
        labels: {
          type: 'array',
          description:
            'Optional labels for categorising and filtering (max 10 labels, 50 characters each). Labels are the shared taxonomy across prompts, artifacts, blueprints and memories, so the same label groups related resources of every type.',
        },
      },
      required: ['team_id', 'project_id', 'slug', 'title', 'content'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_update_blueprint',
    description:
      'Update an existing blueprint, located by its project and slug. Use this tool to revise the rules a blueprint states, retire it by setting its status, or refresh its metadata, while keeping its identity and history.',
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        project_id: {
          type: 'string',
          description:
            'Project UUID the blueprint belongs to — required to locate the blueprint.',
        },
        slug: {
          type: 'string',
          description: 'Slug identifier of the blueprint to update.',
        },
        title: {
          type: 'string',
          description:
            'New title (max 255 chars). Leave empty to keep the existing title unchanged.',
        },
        content: {
          type: 'string',
          description:
            'New blueprint content. Leave empty to keep the existing content unchanged.',
        },
        description: {
          type: 'string',
          description:
            'New description (max 500 chars). Leave empty to keep the existing description unchanged.',
        },
        type: {
          type: 'string',
          description: 'New blueprint type classification.',
        },
        subtype: {
          type: 'string',
          description: 'New subtype, e.g. "sub-agents".',
        },
        status: {
          type: 'string',
          description: 'New status. One of: "active", "expired".',
        },
        metadata: {
          type: 'object',
          description:
            'New metadata. This replaces the existing metadata entirely when provided.',
        },
        labels: {
          type: 'array',
          description:
            'Optional labels for categorising and filtering (max 10 labels, 50 characters each). Labels are the shared taxonomy across prompts, artifacts, blueprints and memories, so the same label groups related resources of every type.',
        },
      },
      required: ['team_id', 'project_id', 'slug'],
      additionalProperties: false,
    },
  },
]
