import type { MCPTool } from './mcp-tool-shared'
import { TEAM_ID_DESCRIPTION } from './mcp-tool-shared'

/**
 * The memory write tools, kept in their own module so the main catalog stays
 * under the 600-line cap the lint config enforces -- the same split
 * mcp-tools-metadata.ts made for the metadata discovery tool.
 */
export const memoryTools: MCPTool[] = [
  {
    name: 'vibexp_io_create_memory',
    description:
      'Store new memory content with associated metadata for later retrieval and reference. Memories provide a flexible way to store and organize information, notes, insights, or any textual content that you want to preserve and search through. Use this tool to create persistent records of important information, learnings, or context that should be available for future reference.',
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        text: {
          type: 'string',
          description:
            'The memory content or text to store. This can be any textual information including notes, insights, code snippets, explanations, or other content you want to preserve for future reference.',
        },
        project_id: {
          type: 'string',
          description:
            'Project UUID identifier — the project this memory belongs to.',
        },
        metadata: {
          type: 'object',
          description:
            'Optional key-value metadata pairs for additional memory information. Use this for tags, categories, timestamps, source information, or any other structured data that helps organize and retrieve memories.',
        },
        labels: {
          type: 'array',
          description:
            'Optional labels for categorising and filtering (max 10 labels, 50 characters each). Labels are the shared taxonomy across prompts, artifacts, blueprints and memories, so the same label groups related resources of every type.',
        },
      },
      required: ['team_id', 'project_id', 'text'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_update_memory',
    description:
      'Update the content or metadata of an existing memory. Use this tool to modify memory text, update associated metadata, or refresh any aspect of the stored memory while maintaining its unique identity.',
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        memory_id: {
          type: 'string',
          description:
            'Unique identifier of the memory to update. This must match exactly with the ID of the existing memory.',
        },
        text: {
          type: 'string',
          description:
            'New text content for the memory. Leave empty to keep the existing content unchanged.',
        },
        metadata: {
          type: 'object',
          description:
            'New metadata for the memory. This will replace the existing metadata entirely. Leave empty to keep existing metadata unchanged.',
        },
        labels: {
          type: 'array',
          description:
            'Optional labels for categorising and filtering (max 10 labels, 50 characters each). Labels are the shared taxonomy across prompts, artifacts, blueprints and memories, so the same label groups related resources of every type.',
        },
      },
      required: ['team_id', 'memory_id'],
      additionalProperties: false,
    },
  },
]
