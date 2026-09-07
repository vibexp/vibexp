import { FolderOpen } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'

import {
  MetadataPanel,
  MetaRow,
  MetaSlugRow,
  type VersionHistoryMeta,
} from '@/components/metadata/MetadataPanel'
import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'

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
   * page; this component only knows there is a link.
   */
  projectHref?: (project: ProjectRef) => string
  className?: string
}

/** The field key every kind uses for its owning project. */
const PROJECT_KEY = 'project_id'

/**
 * Reads a field's value off the payload, following a dotted key one level down
 * (`source.commit_sha`) so a nested provenance object can be described as
 * ordinary fields instead of needing a bespoke renderer per page.
 */
function valueOf(resource: Record<string, unknown>, key: string): unknown {
  const path = key.split('.')
  let current: unknown = resource
  for (const segment of path) {
    if (current === null || typeof current !== 'object') return undefined
    current = new Map(Object.entries(current)).get(segment)
  }
  return current
}

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

export function ResourceMetadataSection({
  descriptor,
  resource,
  versionHistory,
  project,
  projectHref,
  className,
}: Readonly<ResourceMetadataSectionProps>) {
  const fields = partition(descriptor)
  const rows: ReactNode[] = []

  for (const field of fields.type) {
    const value = valueOf(resource, field.key)
    if (typeof value !== 'string' || value.length === 0) continue
    rows.push(
      <MetaRow key={`type:${field.key}`} label={field.label}>
        <Badge variant="secondary">{fieldLabel(field, value)}</Badge>
      </MetaRow>
    )
  }

  for (const field of fields.status) {
    const value = valueOf(resource, field.key)
    if (typeof value !== 'string' || value.length === 0) continue
    rows.push(
      <MetaRow key={`status:${field.key}`} label={field.label}>
        <StatusBadge tone={fieldTone(field, value)}>
          {fieldLabel(field, value)}
        </StatusBadge>
      </MetaRow>
    )
  }

  for (const field of fields.address) {
    const value = valueOf(resource, field.key)
    if (typeof value !== 'string' || value.length === 0) continue
    rows.push(
      <MetaSlugRow
        key={`address:${field.key}`}
        label={field.label}
        value={value}
      />
    )
  }

  if (fields.hasProject && project && projectHref) {
    rows.push(
      <MetaRow key="project" label="Project">
        <Link
          to={projectHref(project)}
          className="flex items-center gap-1 hover:underline"
        >
          <FolderOpen className="size-3" />
          {project.name}
        </Link>
      </MetaRow>
    )
  }

  for (const field of fields.meta) {
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
