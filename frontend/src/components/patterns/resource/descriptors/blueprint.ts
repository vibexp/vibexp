import { defineResource } from '../defineResource'

/**
 * Blueprint — addressed like an artifact. Its status enum is `active | expired`
 * (not `active | inactive`), and `path` is the repo-relative file it
 * materializes to; `source` carries import provenance for imported blueprints.
 */
export const blueprintDescriptor = defineResource({
  kind: 'blueprint',
  singular: 'blueprint',
  plural: 'blueprints',
  address: ['project', 'slug'],
  fields: [
    { key: 'title', role: 'name', label: 'Title' },
    { key: 'project_id', role: 'address', label: 'Project' },
    { key: 'slug', role: 'address', label: 'Slug' },
    {
      key: 'description',
      role: 'summary',
      label: 'Description',
      optional: true,
    },
    { key: 'content', role: 'body', label: 'Content' },
    {
      key: 'status',
      role: 'status',
      label: 'Status',
      statusValues: ['active', 'expired'],
      tone: { active: 'success', expired: 'neutral' },
    },
    { key: 'type', role: 'type', label: 'Type' },
    { key: 'subtype', role: 'taxonomy', label: 'Subtype', optional: true },
    { key: 'path', role: 'meta', label: 'Path' },
    { key: 'source', role: 'meta', label: 'Source', optional: true },
    { key: 'metadata', role: 'meta', label: 'Metadata', optional: true },
  ],
  capabilities: {
    attachments: true,
    versions: true,
    comments: true,
    relations: true,
    mcp: false,
  },
} as const)
