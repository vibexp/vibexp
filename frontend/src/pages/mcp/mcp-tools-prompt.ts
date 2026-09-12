import type { MCPTool } from './mcp-tool-shared'
import { TEAM_ID_DESCRIPTION } from './mcp-tool-shared'

/**
 * The prompt tools, kept in their own module so the main catalog stays under
 * the 600-line cap the lint config enforces -- the same split
 * mcp-tools-memory.ts made for the memory write tools.
 */
export const promptTools: MCPTool[] = [
  {
    name: 'vibexp_io_create_prompt',
    description:
      "Create a new reusable prompt in the team's library. Prompts are templates with {{placeholders}} that any connected tool can render, so a wording that worked once becomes something the whole team can run. Publish it and expose it over MCP to make it available as a prompt primitive in every teammate's AI tool.",
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
            'Project UUID identifier — the project this prompt belongs to.',
        },
        name: {
          type: 'string',
          description: 'Human-readable prompt name (max 50 chars).',
        },
        slug: {
          type: 'string',
          description:
            "Unique identifier for the prompt within the team (max 255 chars). Use descriptive slugs like 'weekly-status-update'.",
        },
        body: {
          type: 'string',
          description:
            'Full prompt body. Use {{placeholder}} syntax for the values a caller supplies at render time.',
        },
        description: {
          type: 'string',
          description:
            'Brief description of what the prompt does and when to use it (max 200 chars).',
        },
        status: {
          type: 'string',
          description:
            'Prompt status. One of: "draft" for works-in-progress, "published" for prompts ready to be used. Only published prompts can be rendered over MCP.',
        },
        mcp_expose: {
          type: 'boolean',
          description:
            'Whether to expose this prompt as an MCP prompt primitive, so it appears directly in a connected tool rather than only through vibexp_io_render_prompt.',
        },
        labels: {
          type: 'array',
          description:
            'Optional labels for categorising and filtering (max 10 labels, 50 characters each). Labels are the shared taxonomy across prompts, artifacts, blueprints and memories, so the same label groups related resources of every type.',
        },
      },
      required: ['team_id', 'project_id', 'name', 'slug', 'body'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_update_prompt',
    description:
      'Update an existing prompt, located by its slug. Use this tool to revise the prompt body or its placeholders, publish a draft, move it to another project, or toggle whether it is exposed over MCP.',
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        slug: {
          type: 'string',
          description: 'Slug identifier of the prompt to update.',
        },
        name: {
          type: 'string',
          description:
            'New name (max 50 chars). Leave empty to keep the existing name unchanged.',
        },
        body: {
          type: 'string',
          description:
            'New prompt body. Leave empty to keep the existing body unchanged.',
        },
        description: {
          type: 'string',
          description:
            'New description (max 200 chars). Leave empty to keep the existing description unchanged.',
        },
        status: {
          type: 'string',
          description: 'New status. One of: "draft", "published".',
        },
        project_id: {
          type: 'string',
          description:
            'New project UUID — supply it to move the prompt to another project.',
        },
        mcp_expose: {
          type: 'boolean',
          description:
            'Whether to expose this prompt as an MCP prompt primitive.',
        },
        labels: {
          type: 'array',
          description:
            'Optional labels for categorising and filtering (max 10 labels, 50 characters each). Labels are the shared taxonomy across prompts, artifacts, blueprints and memories, so the same label groups related resources of every type.',
        },
      },
      required: ['team_id', 'slug'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_render_prompt',
    description:
      "Render a published, MCP-exposed prompt by slug, substituting values for its {{placeholders}}. Use this to run any of your team's prompts as a tool — including prompts not exposed as a slash-command primitive, since only the most recent are. Returns the rendered body.",
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        slug: {
          type: 'string',
          description: 'Slug of the prompt to render (unique within the team).',
        },
        arguments: {
          type: 'object',
          description:
            "Values for the prompt's {{placeholders}}, keyed by placeholder name.",
        },
      },
      required: ['team_id', 'slug'],
      additionalProperties: false,
    },
  },
]
