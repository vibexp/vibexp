import { FolderOpen } from 'lucide-react'
import type { ReactNode } from 'react'

import { type Channel, CHANNEL_ORDER } from '@/components/channelSeries'
import { LastWriteIndicator } from '@/components/LastWriteIndicator'
import {
  MetadataPanel,
  MetaLinkRow,
  MetaRow,
  MetaSlugRow,
  type VersionHistoryMeta,
} from '@/components/metadata/MetadataPanel'
import { LockedByRow } from '@/components/resource-lock/LockedByRow'
import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'
import type { ResourceLock } from '@/types/resourceLock'

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
  /**
   * Edit mode (#1180): a form control per field key, rendered in that field's
   * row in place of its read-only value — so the edit page shows the same rows
   * in the same order, with the editable ones as inputs. A field with a
   * control always gets its row, even while its value is empty; a control for
   * a key no row claims is appended as a row of its own, so nothing a
   * descriptor makes editable can go missing.
   */
  controls?: ReadonlyMap<string, ReactNode>
  /**
   * Display text for a status value — `useStatusOptions(descriptor).labelOf`,
   * so a team's custom status reads by its display name (#1263). Defaults to
   * the descriptor's own labels, which is all a closed status has.
   */
  statusLabel?: (value: string) => string
  /**
   * The lock held on the resource (epic #1285). When set, a "Locked by" row
   * names the locker, with the lock comment in a tooltip. Omitted by kinds
   * that cannot be locked (memories) and by the edit page.
   */
  lock?: ResourceLock | null
  /**
   * Leaves the Status row out. A reading page shows the status once, in its
   * header, so repeating it here is noise; the edit page keeps its status
   * control.
   */
  hideStatus?: boolean
  className?: string
}

/** The field key every kind uses for its owning project. */
const PROJECT_KEY = 'project_id'

/** The payload keys of the last write (#1452), the same on every kind. */
const LAST_WRITE_SOURCE_KEY = 'last_write_source'
const LAST_WRITE_ASSISTANT_KEY = 'last_write_assistant_name'

const CHANNELS: ReadonlySet<string> = new Set(CHANNEL_ORDER)

/**
 * The "Last written" row. It is keyed on the payload carrying the field at
 * all, not on its value: a prompt, artifact, blueprint or memory always has
 * the key, and `null` there is an answer ("Not recorded"), while a kind
 * without write channels (an agent) or a resource not yet created has no key
 * and gets no row.
 */
function lastWriteRow(resource: Record<string, unknown>): ReactNode {
  if (!(LAST_WRITE_SOURCE_KEY in resource)) return null
  const source = valueOf(resource, LAST_WRITE_SOURCE_KEY)
  const assistant = valueOf(resource, LAST_WRITE_ASSISTANT_KEY)
  return (
    <MetaRow label="Last written">
      <LastWriteIndicator
        variant="full"
        source={
          typeof source === 'string' && CHANNELS.has(source)
            ? (source as Channel)
            : null
        }
        assistantName={typeof assistant === 'string' ? assistant : null}
      />
    </MetaRow>
  )
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

/** The edit-mode controls, and which of them a row has already placed. */
interface Controls {
  nodes: ReadonlyMap<string, ReactNode>
  placed: Set<string>
}

/**
 * The row for a field that has a control: its label, with the control as the
 * value. Marks the key placed so the leftover pass does not add it twice.
 */
function controlRow(
  controls: Controls,
  key: string,
  label: string
): ReactNode | null {
  if (!controls.nodes.has(key)) return null
  controls.placed.add(key)
  return (
    <MetaRow key={`control:${key}`} label={label}>
      {controls.nodes.get(key)}
    </MetaRow>
  )
}

/**
 * One row per field whose value is a non-empty string, in field order. Only the
 * shared "look up, skip empty" walk lives here; each role supplies its own row,
 * because the rows genuinely differ (badge, status badge, copyable slug). A
 * field with a control gets the control row instead.
 */
function stringFieldRows(
  fields: FieldSpec[],
  resource: Record<string, unknown>,
  controls: Controls,
  render: (field: FieldSpec, value: string) => ReactNode
): ReactNode[] {
  const rows: ReactNode[] = []
  for (const field of fields) {
    const control = controlRow(controls, field.key, field.label)
    if (control) {
      rows.push(control)
      continue
    }
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
  resource: Record<string, unknown>,
  controls: Controls
): ReactNode[] {
  const rows: ReactNode[] = []
  for (const field of fields) {
    const control = controlRow(controls, field.key, field.label)
    if (control) {
      rows.push(control)
      continue
    }
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

/**
 * The display label of a resource's type ("Claude Code", "General"), or
 * `undefined` for a kind with no type field or a resource with no type set.
 * The same label the Type row shows, for the reading page's header meta line.
 */
export function resourceKindLabel(
  descriptor: ResourceDescriptor,
  resource: Record<string, unknown>
): string | undefined {
  for (const field of partition(descriptor).type) {
    const value = valueOf(resource, field.key)
    if (typeof value === 'string' && value.length > 0) {
      return fieldLabel(field, value)
    }
  }
  return undefined
}

/** The Project row, or none when the kind has no project or it cannot link. */
function projectRows(
  hasProject: boolean,
  project: ProjectRef | null | undefined,
  projectHref: ResourceMetadataSectionProps['projectHref'],
  controls: Controls
): ReactNode[] {
  const control = hasProject
    ? controlRow(controls, PROJECT_KEY, 'Project')
    : null
  if (control) return [control]
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
  controls: controlNodes,
  statusLabel,
  lock,
  hideStatus = false,
  className,
}: Readonly<ResourceMetadataSectionProps>) {
  const fields = partition(descriptor)
  const controls: Controls = {
    nodes: controlNodes ?? new Map(),
    placed: new Set(),
  }
  const rows: ReactNode[] = [
    ...stringFieldRows(fields.type, resource, controls, (field, value) => (
      <MetaRow key={`type:${field.key}`} label={field.label}>
        <Badge variant="secondary">{fieldLabel(field, value)}</Badge>
      </MetaRow>
    )),
    ...stringFieldRows(
      hideStatus ? [] : fields.status,
      resource,
      controls,
      (field, value) => (
        <MetaRow key={`status:${field.key}`} label={field.label}>
          <StatusBadge tone={fieldTone(field, value)}>
            {statusLabel ? statusLabel(value) : fieldLabel(field, value)}
          </StatusBadge>
        </MetaRow>
      )
    ),
    ...stringFieldRows(fields.address, resource, controls, (field, value) => (
      <MetaSlugRow
        key={`address:${field.key}`}
        label={field.label}
        value={value}
      />
    )),
    ...projectRows(fields.hasProject, project, projectHref, controls),
    ...metaRows(fields.meta, resource, controls),
  ]
  if (lock) rows.push(<LockedByRow key="lock" lock={lock} />)
  // A control no role claimed still gets a row, labelled as the descriptor
  // labels the field.
  for (const key of controls.nodes.keys()) {
    if (controls.placed.has(key)) continue
    const label =
      descriptor.fields.find(field => field.key === key)?.label ?? key
    rows.push(controlRow(controls, key, label))
  }

  const createdAt = valueOf(resource, 'created_at')
  const updatedAt = valueOf(resource, 'updated_at')

  return (
    <MetadataPanel
      className={className}
      createdAt={typeof createdAt === 'string' ? createdAt : undefined}
      updatedAt={typeof updatedAt === 'string' ? updatedAt : undefined}
      versionHistory={versionHistory}
      trailing={lastWriteRow(resource)}
    >
      {rows}
    </MetadataPanel>
  )
}
