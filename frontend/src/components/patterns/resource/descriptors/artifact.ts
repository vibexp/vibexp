import { defineResource } from '../defineResource'

/**
 * Artifact — addressed by project + slug (`/artifacts/:project/:slug`, built
 * from `project_id`). Its `type` is an open string validated server-side
 * against the team's registered types, so no values are enumerated here.
 */
export const artifactDescriptor = defineResource({
  kind: 'artifact',
  singular: 'artifact',
  plural: 'artifacts',
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
    { key: 'content', role: 'body', label: 'Content', optional: true },
    {
      key: 'status',
      role: 'status',
      label: 'Status',
      statusValues: ['active', 'draft', 'archived'],
      tone: { active: 'success', draft: 'warning', archived: 'neutral' },
    },
    { key: 'type', role: 'type', label: 'Type' },
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
