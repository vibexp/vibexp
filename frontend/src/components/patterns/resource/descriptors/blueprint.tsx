import { formatDate } from '@/lib/time'

import { defineResource } from '../defineResource'

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
      valueLabels: {
        general: 'General',
        'claude-code': 'Claude Code',
        claude: 'Claude',
        cursor: 'Cursor',
        codex: 'Codex',
      },
    },
    { key: 'subtype', role: 'taxonomy', label: 'Subtype', optional: true },
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
    // row for it — `AdditionalDataCard` beneath the panel owns it.
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
