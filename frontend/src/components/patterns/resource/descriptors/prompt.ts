import { defineResource } from '../defineResource'
import { freshnessFilter, searchFilter, statusFilter } from '../filterSpecs'
import {
  bodyFormField,
  labelsFormField,
  nameFormField,
  projectFormField,
  slugFormField,
  statusFormField,
  summaryFormField,
} from '../formSpecs'

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
  list: {
    // The prompt-only "Shared" tri-state is not here: it is not a field of the
    // resource but a fact about an active share, so the page contributes it to
    // the bar as an extra control. Prompts have no metadata filter either —
    // the list endpoint has no such parameter.
    filters: [
      searchFilter('prompts'),
      statusFilter('prompt'),
      {
        key: 'labels',
        control: 'taxonomy',
        label: 'Filter by labels',
        optionsFrom: 'prompt-labels',
        testId: 'prompt-labels-filter',
      },
      freshnessFilter('prompt', 'prompts'),
    ],
    // Prompts are the one list whose endpoint accepts `status` as a sort field
    // (backend/paths/prompts.yaml: [name, status, updated_at, created_at]).
    sortable: ['name', 'status', 'updated_at'],
  },
  form: {
    fields: [
      nameFormField('name', 50, 'prompt-name-input'),
      // The only slug that stays editable after create: a prompt is addressed
      // by slug alone and the editor already resolves collisions for it.
      slugFormField('prompt-slug-input', false),
      summaryFormField('description', 200, 'prompt-description-input'),
      projectFormField('prompt-project-select'),
      statusFormField('prompt'),
      bodyFormField('body', 'prompt-body-textarea'),
      labelsFormField('labels', 'prompt-labels-input'),
    ],
    // Only prompts are MCP-exposable, and the toggle is a fact about the
    // share rather than a field of the prompt.
    extensions: ['mcp-exposure'],
  },
  capabilities: {
    attachments: true,
    versions: true,
    comments: true,
    relations: true,
    mcp: true,
  },
} as const)
