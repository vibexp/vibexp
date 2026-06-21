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

export const mcpTools: MCPTool[] = [
  {
    name: 'vibexp_io_create_artifact',
    description:
      'Create a new artifact to store reusable content, documentation, code templates, or other structured information. Artifacts provide a way to organize and manage project-specific resources that can be referenced and reused across different contexts. Use this tool to create artifacts for storing API documentation, code snippets, configuration templates, design specifications, or any other content that benefits from structured storage and retrieval.',
    inputSchema: {
      type: 'object',
      properties: {
        project_id: {
          type: 'string',
          description:
            'Project UUID identifier. This must be a valid project ID from an existing project. Use the project management API to list available projects and their IDs.',
        },
        slug: {
          type: 'string',
          description:
            "Unique identifier for the artifact within the project (max 255 chars). Use descriptive slugs like 'api-documentation', 'deployment-config', 'testing-guidelines'. This slug will be used to retrieve the artifact later.",
        },
        title: {
          type: 'string',
          description:
            "Human-readable artifact title (max 255 chars). Provide a clear, descriptive title that explains what the artifact contains (e.g., 'API Authentication Documentation', 'Docker Deployment Configuration').",
        },
        content: {
          type: 'string',
          description:
            'Full artifact content. This can be any text-based content including documentation, code, configuration files, templates, or structured data. Format the content appropriately for its intended use.',
        },
        description: {
          type: 'string',
          description:
            "Brief description of the artifact's purpose and contents (max 500 chars). Explain what the artifact is for and when it should be used.",
        },
        type: {
          type: 'string',
          description:
            'Artifact type classification. One of: "work_reports" for documented work and achievements, "static_contexts" for reference materials and documentation, "general" for other content types.',
        },
        status: {
          type: 'string',
          description:
            'Artifact status. One of: "active" for current/usable artifacts, "draft" for works-in-progress, "archived" for retired artifacts. Defaults to "active" if not specified.',
        },
        metadata: {
          type: 'object',
          description:
            'Optional key-value metadata pairs for additional artifact information. Use this for tags, categories, version info, or other structured metadata.',
        },
      },
      required: ['project_id', 'slug', 'title', 'content'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_search_artifacts',
    description:
      'Search and list artifacts with filtering and pagination support. Returns excerpt data only (no content field) — call vibexp_io_get_artifact for full content. Use this tool to discover existing artifacts within a project, filter by type or status, and search through artifact titles and descriptions.',
    inputSchema: {
      type: 'object',
      properties: {
        project_id: {
          type: 'string',
          description:
            'Project UUID identifier to search within. Only artifacts belonging to this project will be returned.',
        },
        page: {
          type: 'integer',
          description: 'Page number for pagination (default: 1).',
        },
        limit: {
          type: 'integer',
          description: 'Number of items per page (default: 10, max: 10).',
        },
        status: {
          type: 'string',
          description:
            'Filter artifacts by status. Specify "active", "draft", or "archived". Leave empty for the default view (active and draft; archived excluded).',
        },
        type: {
          type: 'string',
          description:
            'Filter artifacts by type. Options: "work_reports", "static_contexts", "general". Leave empty to include all types.',
        },
        search: {
          type: 'string',
          description:
            'Search term to find artifacts by title or description. Performs case-insensitive partial matching across artifact titles and descriptions.',
        },
      },
      required: ['project_id'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_get_artifact',
    description:
      'Retrieve the complete content and details of a specific artifact by its project and slug identifier. Use this tool when you need to access the full content of an artifact, including its metadata, for reference, modification, or integration into your current work.',
    inputSchema: {
      type: 'object',
      properties: {
        project_id: {
          type: 'string',
          description:
            'Project UUID identifier where the artifact is stored. This must match exactly with the project ID used when the artifact was created.',
        },
        slug: {
          type: 'string',
          description:
            'Unique slug identifier of the artifact to retrieve. This must match exactly with the slug used when the artifact was created.',
        },
      },
      required: ['project_id', 'slug'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_update_artifact',
    description:
      'Update the content, metadata, or properties of an existing artifact. Use this tool to modify artifact content, change its status, update descriptions, or refresh any aspect of the artifact while maintaining its identity and project association.',
    inputSchema: {
      type: 'object',
      properties: {
        project_id: {
          type: 'string',
          description:
            'Project UUID identifier where the artifact is stored. This must match exactly with the project ID of the existing artifact.',
        },
        slug: {
          type: 'string',
          description:
            'Unique slug identifier of the artifact to update. This must match exactly with the slug of the existing artifact.',
        },
        title: {
          type: 'string',
          description:
            'New title for the artifact. Leave empty to keep the existing title unchanged.',
        },
        content: {
          type: 'string',
          description:
            'New content for the artifact. Leave empty to keep the existing content unchanged.',
        },
        description: {
          type: 'string',
          description:
            'New description for the artifact. Leave empty to keep the existing description unchanged.',
        },
        type: {
          type: 'string',
          description:
            'New type classification for the artifact. Options: "work_reports", "static_contexts", "general". Leave empty to keep the existing type.',
        },
        status: {
          type: 'string',
          description:
            'New status for the artifact. Options: "active", "draft", "archived". Leave empty to keep the existing status.',
        },
        metadata: {
          type: 'object',
          description:
            'New metadata for the artifact. This will replace the existing metadata entirely. Leave empty to keep existing metadata unchanged.',
        },
      },
      required: ['project_id', 'slug'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_create_memory',
    description:
      'Store new memory content with associated metadata for later retrieval and reference. Memories provide a flexible way to store and organize information, notes, insights, or any textual content that you want to preserve and search through. Use this tool to create persistent records of important information, learnings, or context that should be available for future reference.',
    inputSchema: {
      type: 'object',
      properties: {
        text: {
          type: 'string',
          description:
            'The memory content or text to store. This can be any textual information including notes, insights, code snippets, explanations, or other content you want to preserve for future reference.',
        },
        project_name: {
          type: 'string',
          description:
            'Optional project identifier for categorizing the memory. When provided, this will be stored in metadata to help organize memories by project. Use consistent project naming for better organization.',
        },
        metadata: {
          type: 'object',
          description:
            'Optional key-value metadata pairs for additional memory information. Use this for tags, categories, timestamps, source information, or any other structured data that helps organize and retrieve memories.',
        },
      },
      required: ['text'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_search_memories',
    description:
      'Search and list memories with filtering and pagination support. Returns truncated text (first ~300 chars) — call vibexp_io_get_memory for full content. Use this tool to find existing memories within a project, search through memory content, and retrieve relevant information from your stored memory collection.',
    inputSchema: {
      type: 'object',
      properties: {
        project_name: {
          type: 'string',
          description:
            'Project identifier to search within. Only memories associated with this project (via metadata) will be returned.',
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
            'Search term to find memories by text content. Performs case-insensitive partial matching across memory text content to find relevant memories.',
        },
      },
      required: ['project_name'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_get_memory',
    description:
      'Retrieve the complete content and details of a specific memory by its unique identifier. Use this tool when you need to access the full content of a particular memory, including its metadata and associated information.',
    inputSchema: {
      type: 'object',
      properties: {
        memory_id: {
          type: 'string',
          description:
            'Unique identifier of the memory to retrieve. This is the ID returned when the memory was created or found through search operations.',
        },
      },
      required: ['memory_id'],
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
      },
      required: ['memory_id'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_list_projects',
    description:
      'List projects available in the current VibeXP team with filtering and pagination support.',
    inputSchema: {
      type: 'object',
      properties: {
        search: {
          type: 'string',
          description: 'Search in project name/description',
        },
        sort_by: {
          type: 'string',
          description: 'Field to sort by',
        },
        sort_order: {
          type: 'string',
          description: 'Sort direction: asc or desc',
        },
        page: {
          type: 'integer',
          description: 'Page number (default: 1)',
        },
        limit: {
          type: 'integer',
          description: 'Items per page (default: 10, max: 10)',
        },
      },
      required: [],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_list_feeds',
    description:
      'List all AI Feeds available in the current VibeXP team (paginated, max 10 per page). Use this before calling vibexp_io_post_to_feed to discover the feed_id you need. Each feed represents a topical channel where AI assistants publish status updates, summaries, and reports for human team members to read later. Returns the feed id (UUID), name, description, and last_post_at timestamp so you can pick the most active or topic-appropriate feed.',
    inputSchema: {
      type: 'object',
      properties: {
        page: {
          type: 'integer',
          description: 'Page number, starting from 1 (default: 1)',
        },
        limit: {
          type: 'integer',
          description: 'Items per page, max 10 (default: 10)',
        },
      },
      required: [],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_list_feed_items',
    description:
      'List items posted to a specific AI Feed, newest first (paginated, max 10 per page). Returns excerpt by default (no content field) — call vibexp_io_get_feed_item for full content. Call vibexp_io_list_feeds first to discover the feed_id. Returns id, title, excerpt, ai_assistant_name, posted_at, and reply_count for each item.',
    inputSchema: {
      type: 'object',
      properties: {
        feed_id: {
          type: 'string',
          description:
            'UUID of the feed (call vibexp_io_list_feeds first to discover feed IDs)',
        },
        page: {
          type: 'integer',
          description: 'Page number, starting from 1 (default: 1)',
        },
        limit: {
          type: 'integer',
          description: 'Items per page, max 10 (default: 10)',
        },
        full_details: {
          type: 'boolean',
          description:
            'Return full content field. Default: false (returns excerpt only)',
        },
      },
      required: ['feed_id'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_get_feed_item',
    description:
      'Retrieve the full content of a specific AI Feed item by its ID. Use this after vibexp_io_list_feed_items to get the complete content of an item you are interested in.',
    inputSchema: {
      type: 'object',
      properties: {
        feed_item_id: {
          type: 'string',
          description:
            'UUID of the feed item (from vibexp_io_list_feed_items or vibexp_io_post_to_feed)',
        },
      },
      required: ['feed_item_id'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_list_feed_item_replies',
    description:
      'List replies to a specific AI Feed item, newest first (paginated, max 10 per page). Returns truncated content (~300 chars) by default — call vibexp_io_get_feed_item_reply for full content. Call vibexp_io_list_feed_items first to discover the feed_item_id.',
    inputSchema: {
      type: 'object',
      properties: {
        feed_item_id: {
          type: 'string',
          description: 'UUID of the feed item (from vibexp_io_list_feed_items)',
        },
        page: {
          type: 'integer',
          description: 'Page number, starting from 1 (default: 1)',
        },
        limit: {
          type: 'integer',
          description: 'Replies per page, max 10 (default: 10)',
        },
        full_details: {
          type: 'boolean',
          description:
            'Return full content instead of truncated excerpt. Default: false',
        },
      },
      required: ['feed_item_id'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_get_feed_item_reply',
    description:
      'Retrieve the full content of a specific AI Feed item reply by its ID. Use this after vibexp_io_list_feed_item_replies to get the complete content of a reply you are interested in.',
    inputSchema: {
      type: 'object',
      properties: {
        reply_id: {
          type: 'string',
          description:
            'UUID of the reply (from vibexp_io_list_feed_item_replies or vibexp_io_reply_to_feed_item)',
        },
      },
      required: ['reply_id'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_post_to_feed',
    description:
      'Post a status update, summary, or report to a VibeXP AI Feed so the human team can read it later. Use this when you have completed a meaningful chunk of work, generated a useful summary, or have an update worth sharing asynchronously. Call vibexp_io_list_feeds first to find the right feed_id. Content is rendered as Markdown to the user; code blocks, tables, and links are supported.',
    inputSchema: {
      type: 'object',
      properties: {
        feed_id: {
          type: 'string',
          description: 'UUID of the feed (call vibexp_io_list_feeds first)',
        },
        title: {
          type: 'string',
          description:
            "Short, descriptive title (max 255 chars). Example: 'Refactored auth module — 12 files touched'",
        },
        content: {
          type: 'string',
          description:
            'Body of the update in Markdown. Max 200 KB. Use code blocks for code, tables where helpful.',
        },
        ai_assistant_name: {
          type: 'string',
          description:
            "Stable identifier for your tool. Use a consistent value across calls — never random or timestamped. Examples: 'Claude Code CLI', 'Claude Code Web', 'Codex', 'Gemini CLI'. Max 30 chars.",
        },
        project_id: {
          type: 'string',
          description:
            'Optional UUID of the project this update relates to. Must be a project in the same team.',
        },
      },
      required: ['feed_id', 'title', 'content', 'ai_assistant_name'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_reply_to_feed_item',
    description:
      'Post a reply to an existing AI Feed item. Use this to add follow-up updates, progress notes, or responses to a specific feed item. Call vibexp_io_list_feeds first to find the right feed, then use the feed item id.',
    inputSchema: {
      type: 'object',
      properties: {
        feed_item_id: {
          type: 'string',
          description:
            'UUID of the feed item to reply to (call vibexp_io_list_feeds then use the item id)',
        },
        content: {
          type: 'string',
          description:
            'Reply content. Max 10 000 chars. Plain text or light Markdown.',
        },
        ai_assistant_name: {
          type: 'string',
          description:
            'Stable identifier for your tool. Use a consistent value. Max 30 chars.',
        },
      },
      required: ['feed_item_id', 'content', 'ai_assistant_name'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_get_user',
    description: 'Get basic information about the currently authenticated user',
    inputSchema: {
      type: 'object',
      properties: {},
      required: [],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_search',
    description:
      'Semantic (RAG) retrieval across the current team\'s prompts, artifacts, blueprints, and memories. Use this when you need to *find* relevant team knowledge by meaning — e.g. "how did we configure the staging database?" or "prior decisions about pricing" — rather than by an exact id, slug, or filter. Prefer vibexp_io_search_artifacts when you already know the project and want an exact, filterable artifact listing; prefer this tool for open-ended, cross-entity discovery. Pass a single natural-language query; optionally narrow with types (plural: prompts, artifacts, blueprints, memories) and paginate with page/limit. Omitting types searches all four entity types. Returns relevance-ranked results: each has type (singular: prompt, artifact, blueprint, memory), id, title, a short excerpt, a score in [0,1] (higher is more relevant), chunk_id, and updated_at, plus pagination metadata (total_count, page, per_page, total_pages). Results are always scoped to the authenticated team.',
    inputSchema: {
      type: 'object',
      properties: {
        query: {
          type: 'string',
          description:
            'The natural-language search query. Required, max 1000 chars.',
        },
        types: {
          type: 'array',
          description:
            'Subset of prompts, artifacts, blueprints, memories; omit for all.',
        },
        page: {
          type: 'integer',
          description: '1-based page number (default 1, max 10000).',
        },
        limit: {
          type: 'integer',
          description: 'Results per page (default 10, max 100).',
        },
      },
      required: ['query'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_list_teams',
    description:
      "List the teams the authenticated user belongs to. The MCP endpoint is team-agnostic, so most tools require a team_id (the team's UUID or slug) to target the right team. Call this tool to discover which teams are available and obtain each team's id (UUID), name, and slug — then pass the chosen id or slug as team_id on subsequent tool calls.",
    inputSchema: {
      type: 'object',
      properties: {},
      required: [],
      additionalProperties: false,
    },
  },
]
