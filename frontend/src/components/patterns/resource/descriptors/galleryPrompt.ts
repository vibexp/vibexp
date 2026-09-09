import { defineResource } from '../defineResource'

/**
 * Gallery prompt — the public prompt gallery (`/prompt-gallery/:category/:id`).
 * It is served by the public gallery API, not the team resource API, so it has
 * no team-scoped resource id: every shared side panel is unavailable and the
 * kind is read-only. It has no status either.
 */
export const galleryPromptDescriptor = defineResource({
  kind: 'gallery-prompt',
  singular: 'gallery prompt',
  plural: 'gallery prompts',
  address: ['id'],
  fields: [
    { key: 'title', role: 'name', label: 'Title' },
    { key: 'id', role: 'address', label: 'ID' },
    {
      key: 'description',
      role: 'summary',
      label: 'Description',
      optional: true,
    },
    { key: 'content', role: 'body', label: 'Content' },
    { key: 'category', role: 'taxonomy', label: 'Category' },
    { key: 'tags', role: 'taxonomy', label: 'Tags', optional: true },
  ],
  capabilities: {
    attachments: false,
    versions: false,
    comments: false,
    relations: false,
    mcp: false,
  },
  readOnly: true,
} as const)
