import { defineResource } from '../defineResource'

/**
 * Prompt — the only kind addressed by slug alone (`/prompts/:slug`), and the
 * only one exposable over MCP.
 */
export const promptDescriptor = defineResource({
  kind: 'prompt',
  singular: 'prompt',
  plural: 'prompts',
  address: ['slug'],
  fields: [
    { key: 'name', role: 'name', label: 'Name' },
    { key: 'slug', role: 'address', label: 'Slug' },
    { key: 'description', role: 'summary', label: 'Description' },
    { key: 'body', role: 'body', label: 'Body' },
    {
      key: 'status',
      role: 'status',
      label: 'Status',
      statusValues: ['draft', 'published'],
      tone: { draft: 'warning', published: 'success' },
      valueLabels: { draft: 'Draft', published: 'Published' },
    },
    { key: 'labels', role: 'taxonomy', label: 'Labels', optional: true },
    {
      key: 'mcp_expose',
      role: 'meta',
      label: 'MCP',
      render: value => (value === true ? 'Exposed' : 'Not exposed'),
    },
    {
      key: 'is_shared',
      role: 'meta',
      label: 'Shared',
      // Only worth a row when it is true: an unshared prompt is the norm, and
      // the reading header already badges a shared one.
      render: value => (value === true ? 'Shared' : null),
    },
    { key: 'project_id', role: 'meta', label: 'Project' },
  ],
  capabilities: {
    attachments: true,
    versions: true,
    comments: true,
    relations: true,
    mcp: true,
  },
} as const)
