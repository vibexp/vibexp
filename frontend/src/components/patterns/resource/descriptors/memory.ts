import { defineResource } from '../defineResource'

/**
 * Memory — the kind with no title of its own. `text` carries both roles: lists
 * render a truncated excerpt of it as the row's identifier, and the reading
 * page renders the whole thing as the body. Memory is also the one built-in
 * that does not take attachments (`MemoryView` passes `attachments={false}`).
 */
export const memoryDescriptor = defineResource({
  kind: 'memory',
  singular: 'memory',
  plural: 'memories',
  address: ['id'],
  fields: [
    { key: 'text', role: 'name', label: 'Memory' },
    { key: 'text', role: 'body', label: 'Memory' },
    { key: 'id', role: 'address', label: 'ID' },
    {
      key: 'status',
      role: 'status',
      label: 'Status',
      statusValues: ['active', 'draft', 'archived'],
      tone: { active: 'success', draft: 'warning', archived: 'neutral' },
    },
    { key: 'project_id', role: 'meta', label: 'Project' },
    { key: 'metadata', role: 'meta', label: 'Metadata', optional: true },
  ],
  capabilities: {
    attachments: false,
    versions: true,
    comments: true,
    relations: true,
    mcp: false,
  },
} as const)
