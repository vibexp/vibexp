import { defineResource } from '../defineResource'
import {
  freshnessFilter,
  metadataFilter,
  searchFilter,
  statusFilter,
} from '../filterSpecs'
import {
  bodyFormField,
  metadataFormField,
  nameFormField,
  projectFormField,
  slugFormField,
  statusFormField,
  summaryFormField,
} from '../formSpecs'

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
  list: {
    filters: [
      searchFilter('artifacts'),
      {
        key: 'type',
        control: 'select',
        label: 'Filter by type',
        allLabel: 'All types',
        // Open string validated against the team's registered types, so the
        // catalog is a runtime fetch rather than a list declared here.
        optionsFrom: 'types',
      },
      statusFilter('artifact'),
      freshnessFilter('artifact', 'artifacts'),
      metadataFilter('artifacts'),
    ],
    // `status` is absent because the list endpoint's `sort_by` enum is
    // [created_at, updated_at, title] — declaring it would 400 on the first
    // header click (backend/paths/artifacts.yaml).
    sortable: ['title', 'updated_at'],
  },
  form: {
    fields: [
      nameFormField('title', 255, 'artifact-title-input'),
      slugFormField('artifact-slug-input', true),
      summaryFormField('description', 500, 'artifact-description-input'),
      projectFormField('artifact-project-select'),
      {
        key: 'type',
        control: 'select',
        section: 'details',
        required: true,
        // The team's registered types, so the option list is a runtime fetch
        // rather than a vocabulary this descriptor can enumerate.
        optionsFrom: 'types',
        testId: 'artifact-type-select',
      },
      statusFormField('artifact'),
      bodyFormField('content', 'artifact-content-textarea'),
      metadataFormField(),
    ],
  },
  capabilities: {
    attachments: true,
    versions: true,
    comments: true,
    relations: true,
    mcp: false,
  },
} as const)
