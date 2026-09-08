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
      valueLabels: { active: 'Active', draft: 'Draft', archived: 'Archived' },
    },
    { key: 'project_id', role: 'meta', label: 'Project' },
    // The free-form blob: object-valued, so the metadata section renders no
    // row for it — `ResourceTaxonomySection` owns it (and lifts its `tags`).
    { key: 'metadata', role: 'meta', label: 'Metadata', optional: true },
  ],
  list: {
    filters: [
      { key: 'search', control: 'search', label: 'Search memories' },
      {
        key: 'status',
        control: 'select',
        label: 'Filter by status',
        allLabel: 'All statuses',
        optionsFrom: 'field',
        testId: 'memory-status-filter',
      },
      {
        key: 'freshness',
        control: 'freshness',
        label: 'Filter memories by freshness',
        testId: 'memory-freshness-filter',
      },
      {
        key: 'metadata',
        control: 'metadata',
        label: 'Filter memories by metadata',
      },
    ],
    // `text` is the memory's name field, and the list endpoint's `sort_by`
    // enum is [text, updated_at, created_at] (backend/paths/memories.yaml).
    sortable: ['text', 'updated_at'],
  },
  capabilities: {
    attachments: false,
    versions: true,
    comments: true,
    relations: true,
    mcp: false,
  },
} as const)
