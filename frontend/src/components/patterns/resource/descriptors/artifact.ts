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
      valueLabels: { active: 'Active', draft: 'Draft', archived: 'Archived' },
    },
    {
      key: 'type',
      role: 'type',
      label: 'Type',
      // Partial on purpose: `type` is an open string, so a team type the SPA
      // has never heard of renders as itself rather than as `undefined`.
      valueLabels: {
        general: 'General',
        work_reports: 'Work reports',
        static_contexts: 'Static contexts',
      },
    },
    // The free-form blob: object-valued, so the metadata section renders no
    // row for it — `ResourceTaxonomySection` owns it.
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
