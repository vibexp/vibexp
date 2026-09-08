import { formatDate } from '@/lib/time'

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
  nameFormField,
  projectFormField,
  slugFormField,
  statusFormField,
  summaryFormField,
} from '../formSpecs'

/**
 * Blueprint — addressed like an artifact. Its status enum is `active | expired`
 * (not `active | inactive`), and `path` is the repo-relative file it
 * materializes to; `source` carries import provenance for imported blueprints.
 *
 * The three `source.*` fields are declared with dotted keys rather than as one
 * `source` object: the metadata section renders one row per field, and a nested
 * object would otherwise need a renderer that emits several rows — exactly the
 * bespoke, page-shaped escape hatch #903 removes.
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
      valueLabels: { active: 'Active', expired: 'Expired' },
    },
    {
      key: 'type',
      role: 'type',
      label: 'Type',
      // Closed enum (backend/paths/blueprints.yaml), unlike the artifact type.
      // Declared separately from `valueLabels`, which is partial by design and
      // must never be mistaken for the complete value set.
      typeValues: ['general', 'claude-code', 'claude', 'cursor', 'codex'],
      valueLabels: {
        general: 'General',
        'claude-code': 'Claude Code',
        claude: 'Claude',
        cursor: 'Cursor',
        codex: 'Codex',
      },
    },
    { key: 'subtype', role: 'taxonomy', label: 'Subtype', optional: true },
    { key: 'labels', role: 'taxonomy', label: 'Labels', optional: true },
    {
      key: 'path',
      role: 'meta',
      label: 'Path',
      render: value =>
        typeof value === 'string' && value.length > 0 ? (
          <code
            className="text-foreground/90 min-w-0 truncate font-mono text-xs"
            title={value}
          >
            {value}
          </code>
        ) : null,
    },
    {
      key: 'source.repo',
      role: 'meta',
      label: 'Source',
      optional: true,
      render: value =>
        typeof value === 'string' && value.length > 0 ? (
          <a
            href={value}
            target="_blank"
            rel="noreferrer"
            className="text-primary min-w-0 truncate hover:underline"
            title={value}
          >
            {value.replace(/^https?:\/\//, '')}
          </a>
        ) : null,
    },
    {
      key: 'source.commit_sha',
      role: 'meta',
      label: 'Commit',
      optional: true,
      render: value =>
        typeof value === 'string' && value.length > 0 ? (
          <code className="text-muted-foreground font-mono text-xs">
            {value.slice(0, 7)}
          </code>
        ) : null,
    },
    {
      key: 'source.imported_at',
      role: 'meta',
      label: 'Imported',
      optional: true,
      render: value =>
        typeof value === 'string' && value.length > 0 ? (
          <span className="text-muted-foreground">{formatDate(value)}</span>
        ) : null,
    },
    // The free-form blob: object-valued, so the metadata section renders no
    // row for it — `ResourceTaxonomySection` owns it.
    { key: 'metadata', role: 'meta', label: 'Metadata', optional: true },
  ],
  list: {
    filters: [
      searchFilter('blueprints'),
      {
        key: 'type',
        control: 'select',
        label: 'Filter by type',
        allLabel: 'All types',
        optionsFrom: 'field',
      },
      statusFilter('blueprint'),
      freshnessFilter('blueprint', 'blueprints'),
      metadataFilter('blueprints'),
    ],
    // The list endpoint's `sort_by` enum is [created_at, updated_at, title]
    // (backend/paths/blueprints.yaml), so status is not sortable here.
    sortable: ['title', 'updated_at'],
  },
  form: {
    // `subtype` is read-only — it comes from the import — so it has no control
    // even though it is a declared taxonomy field.
    fields: [
      nameFormField('title', 255, 'blueprint-title-input'),
      slugFormField('blueprint-slug-input', true),
      summaryFormField('description', 500, 'blueprint-description-input'),
      projectFormField('blueprint-project-select'),
      {
        key: 'type',
        control: 'select',
        section: 'details',
        required: true,
        optionsFrom: 'field',
        testId: 'blueprint-type-select',
      },
      // A blueprint's status IS editable — the API has accepted it since
      // before this epic (`UpdateBlueprintRequest.status`) and only the form
      // was missing (#915). The vocabulary is the two values #912 left it
      // with: widening a subset is a product decision, not a UI one, and
      // `BlueprintStatus` is still `[active, expired]` on the spec.
      statusFormField('blueprint'),
      {
        ...bodyFormField('content', 'blueprint-content-textarea'),
        placeholder: 'Enter blueprint content…',
      },
      labelsFormField('labels', 'blueprint-labels-input'),
      metadataFormField(),
    ],
    // A sub-agents blueprint must carry a `model` metadata key (enforced in
    // internal/services/blueprint.go). Which keys those are depends on the
    // blueprint being edited, not on the kind, so the page fills the slot.
    extensions: ['required-metadata-keys'],
  },
  capabilities: {
    attachments: true,
    versions: true,
    comments: true,
    relations: true,
    mcp: false,
  },
} as const)
