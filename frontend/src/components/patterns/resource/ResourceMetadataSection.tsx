import { FolderOpen } from 'lucide-react'
import type { ReactNode } from 'react'

import {
  MetadataPanel,
  MetaLinkRow,
  MetaRow,
  MetaSlugRow,
  type VersionHistoryMeta,
} from '@/components/metadata/MetadataPanel'
import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'

import { valueOf } from './fieldValue'
import { fieldLabel, fieldTone } from './statusTone'
import type { FieldSpec, ResourceDescriptor } from './types'

/*
 * The details-column Metadata section, generated from the descriptor.
 *
 * Before #903 each of the four detail pages hand-composed `MetadataPanel` with
 * its own rows, so they drifted: only memory showed its project, only artifacts
 * and blueprints showed a slug, and the prompt showed nothing but MCP. This
 * component walks the descriptor instead, in one fixed role order, so every
 * kind — including a resource type added later — gets the same rows in the same
 * places. `MetadataPanel` is untouched: it stays the dumb rendering primitive
 * (and keeps its `metadata-panel` test id), this only stops pages hand-feeding
 * it.
 */

/** A project reference the Project row can render. */
export interface ProjectRef {
  name: string
  slug: string
}

export interface ResourceMetadataSectionProps {
  descriptor: ResourceDescriptor
  /** The API payload, read by descriptor field key. */
  resource: Record<string, unknown>
  /** Forwarded verbatim to `MetadataPanel`. */
  versionHistory?: VersionHistoryMeta
  /**
   * The resolved project for the resource's `project_id`. `null`/omitted while
   * it loads (or when the caller cannot resolve it) renders no Project row —
   * a row reading "Project —" is worse than no row.
   */
  project?: ProjectRef | null
  /**
   * Builds the Project row's link target. Routing knowledge stays with the
   * page; this component only knows there is a link. Returning `null` (no team
   * resolved yet, say) renders no row rather than a broken one.
   */
  projectHref?: (project: ProjectRef) => string | null
  className?: string
}

/** The field key every kind uses for its owning project. */
const PROJECT_KEY = 'project_id'

/** Fields carrying the metadata rows, bucketed by the role that renders them. */
function partition(descriptor: ResourceDescriptor) {
  const byRole = (role: FieldSpec['role']) =>
    descriptor.fields.filter(field => field.role === role)
  return {
    type: byRole('type'),
    status: byRole('status'),
    // The project segment of an address is rendered as the Project row, not as
    // a copyable chip: `/artifacts/:project/:slug` carries a UUID no reader
    // wants to see twice.
    address: byRole('address').filter(field => field.key !== PROJECT_KEY),
    meta: byRole('meta').filter(field => field.key !== PROJECT_KEY),
    hasProject: descriptor.fields.some(field => field.key === PROJECT_KEY),
  }
}

/** A `meta` row's value: the field's own renderer, or the scalar as text. */
function metaValue(field: FieldSpec, value: unknown): ReactNode {
  if (field.render) return field.render(value)
  if (typeof value === 'string' && value.length > 0) return value
  if (typeof value === 'number') return String(value)
  return null
}

/**
 * One row per field whose value is a non-empty string, in field order. Only the
 * shared "look up, skip empty" walk lives here; each role supplies its own row,
 * because the rows genuinely differ (badge, status badge, copyable slug).
 */
function stringFieldRows(
  fields: FieldSpec[],
  resource: Record<string, unknown>,
  render: (field: FieldSpec, value: string) => ReactNode
): ReactNode[] {
  const rows: ReactNode[] = []
  for (const field of fields) {
    const value = valueOf(resource, field.key)
    if (typeof value === 'string' && value.length > 0) {
      rows.push(render(field, value))
    }
  }
  return rows
}

/** The `meta` rows: any field whose `metaValue` renders something. */
function metaRows(
  fields: FieldSpec[],
  resource: Record<string, unknown>
): ReactNode[] {
  const rows: ReactNode[] = []
  for (const field of fields) {
    const rendered = metaValue(field, valueOf(resource, field.key))
    if (rendered === null || rendered === undefined || rendered === false) {
      continue
    }
    rows.push(
      <MetaRow key={`meta:${field.key}`} label={field.label}>
        {rendered}
      </MetaRow>
    )
  }
  return rows
}

/** The Project row, or none when the kind has no project or it cannot link. */
function projectRows(
  hasProject: boolean,
  project: ProjectRef | null | undefined,
  projectHref: ResourceMetadataSectionProps['projectHref']
): ReactNode[] {
  const projectTo =
    hasProject && project && projectHref ? projectHref(project) : null
  if (!project || !projectTo) return []
  return [
    <MetaLinkRow key="project" icon={FolderOpen} label="Project" to={projectTo}>
      {project.name}
    </MetaLinkRow>,
  ]
}

export function ResourceMetadataSection({
  descriptor,
  resource,
  versionHistory,
  project,
  projectHref,
  className,
}: Readonly<ResourceMetadataSectionProps>) {
  const fields = partition(descriptor)
  const rows: ReactNode[] = [
    ...stringFieldRows(fields.type, resource, (field, value) => (
      <MetaRow key={`type:${field.key}`} label={field.label}>
        <Badge variant="secondary">{fieldLabel(field, value)}</Badge>
      </MetaRow>
    )),
    ...stringFieldRows(fields.status, resource, (field, value) => (
      <MetaRow key={`status:${field.key}`} label={field.label}>
        <StatusBadge tone={fieldTone(field, value)}>
          {fieldLabel(field, value)}
        </StatusBadge>
      </MetaRow>
    )),
    ...stringFieldRows(fields.address, resource, (field, value) => (
      <MetaSlugRow
        key={`address:${field.key}`}
        label={field.label}
        value={value}
      />
    )),
    ...projectRows(fields.hasProject, project, projectHref),
    ...metaRows(fields.meta, resource),
  ]

  const createdAt = valueOf(resource, 'created_at')
  const updatedAt = valueOf(resource, 'updated_at')

  return (
    <MetadataPanel
      className={className}
      createdAt={typeof createdAt === 'string' ? createdAt : undefined}
      updatedAt={typeof updatedAt === 'string' ? updatedAt : undefined}
      versionHistory={versionHistory}
    >
      {rows}
    </MetadataPanel>
  )
}
