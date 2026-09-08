import { defineResource } from '../defineResource'
import {
  freshnessFilter,
  metadataFilter,
  searchFilter,
  statusFilter,
} from '../filterSpecs'
import {
  bodyFormField,
  labelsFormField,
  metadataFormField,
  projectFormField,
  statusFormField,
} from '../formSpecs'

/**
 * Memory — the one kind whose title is OPTIONAL (#911). `title` carries the
 * `name` role so the form, the header subtitle and the list all read one field;
 * `text` is the body alone, and every reader falls back to an excerpt of it
 * when the title is null (which every memory written before #911 is). Memory is
 * also the one built-in that does not take attachments (`MemoryView` passes
 * `attachments={false}`).
 */
export const memoryDescriptor = defineResource({
  kind: 'memory',
  singular: 'memory',
  plural: 'memories',
  address: ['id'],
  fields: [
    { key: 'title', role: 'name', label: 'Title', optional: true },
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
    { key: 'labels', role: 'taxonomy', label: 'Labels', optional: true },
    // The free-form blob: object-valued, so the metadata section renders no
    // row for it — `ResourceTaxonomySection` owns it (and lifts its `tags`).
    { key: 'metadata', role: 'meta', label: 'Metadata', optional: true },
  ],
  list: {
    filters: [
      searchFilter('memories'),
      statusFilter('memory'),
      freshnessFilter('memory', 'memories'),
      metadataFilter('memories'),
    ],
    // The list's primary column is keyed on `text`, not on the `title` name
    // field: the endpoint's `sort_by` enum is [text, updated_at, created_at]
    // (backend/paths/memories.yaml) and #911 did not add `title` to it.
    sortable: ['text', 'updated_at'],
  },
  form: {
    fields: [
      // Not `nameFormField`: a memory's title is the one name field the API
      // leaves optional, so it must not carry that builder's `required: true`.
      {
        key: 'title',
        control: 'text',
        section: 'details',
        maxLength: 255,
        placeholder: 'Optional short title…',
        testId: 'memory-title-input',
      },
      // The test id is the one `e2e/memories.spec.ts` has always driven this
      // textarea by; the descriptor's own `memory-text-textarea` never reached
      // a rendered page, so the e2e selector is what survives (#915).
      {
        ...bodyFormField('text', 'memory-content-textarea'),
        placeholder:
          'Enter your memory content here…\n\nShare insights, learnings, code snippets, or any valuable information you want to remember.',
      },
      projectFormField('memory-project-select'),
      statusFormField(
        'memory',
        'Drafts are hidden from search; archived memories are hidden from default lists and search.'
      ),
      labelsFormField('labels', 'memory-labels-input'),
      metadataFormField(),
    ],
    // Memory has no `tags` FIELD — the chips edit `metadata.tags`, which the
    // metadata control deliberately does not own (`MemoryForm` reserves the
    // key). That lift is a display choice, so it stays a page-supplied slot.
    extensions: ['tags'],
  },
  capabilities: {
    attachments: false,
    versions: true,
    comments: true,
    relations: true,
    mcp: false,
  },
} as const)
