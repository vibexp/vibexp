import { AdditionalDataRows } from '@/components/AdditionalDataRows'
import { TaxonomyChips } from '@/components/TaxonomyChips'
import {
  Panel,
  PanelBody,
  PanelHeader,
  PanelTitle,
} from '@/components/ui/panel'

import { valueOf } from './fieldValue'
import type { ResourceDescriptor } from './types'

/*
 * The details-column taxonomy section, generated from the descriptor.
 *
 * Free-form grouping used to render three ways: prompt labels as a "Labels"
 * card, memory tags as a "Tags" card fed by a page-local `extractTags()`, and
 * the metadata bag as an "Additional data" card at three call sites — one of
 * which also carried a "No metadata." fallback the other two did not. Blueprint
 * `subtype` is declared `taxonomy` and rendered nowhere at all. This walks the
 * descriptor instead, so every kind gets the same block in the same place under
 * one heading, and #910 (a real `labels[]` on every resource) becomes a
 * descriptor edit rather than four page rewrites.
 *
 * Fields are classified by the shape of their VALUE, not by their role: the
 * `meta` role holds both scalar rows (`mcp_expose`, `path`) and pair bags
 * (`metadata`), so classifying by role would make this section and #903's
 * `ResourceMetadataSection` fight over the same field. Scalars belong to the
 * metadata rows, lists and objects belong here — a partition neither side can
 * claim twice.
 */

/** The metadata key lifted out of a pair bag and shown as chips. */
const TAGS_KEY = 'tags'

/** One rendered block: a labelled chip row, or a bag of key/value pairs. */
type Group =
  | { kind: 'chips'; id: string; label: string; values: string[] }
  | { kind: 'pairs'; id: string; data: Record<string, unknown> }

/**
 * The chip values a `taxonomy` field carries, if any. Deduped: `metadata.tags`
 * is user-authored and may repeat a tag, which would both render twice and warn
 * on a duplicate React key.
 */
function chipValues(value: unknown): string[] {
  if (Array.isArray(value)) {
    const strings = value.filter(
      (v): v is string => typeof v === 'string' && v !== ''
    )
    return [...new Set(strings)]
  }
  if (typeof value === 'string' && value.length > 0) return [value]
  return []
}

/** A `tags` value the chip row can render without losing anything. */
function isChipShaped(value: unknown): boolean {
  return (
    typeof value === 'string' ||
    (Array.isArray(value) && value.every(entry => typeof entry === 'string'))
  )
}

/** A free-form pair bag — a plain object, which no scalar metadata row claims. */
function isPairBag(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/**
 * Splits a pair bag into its chip-shaped `tags` entry and the rest.
 *
 * Memory has no `labels` field of its own — it carries free-form tags inside
 * `metadata.tags`, which `MemoryView` used to lift with a page-local helper.
 * The lift lives here until #910 gives every resource a real `labels[]`, at
 * which point this becomes dead and the descriptor alone decides.
 */
function splitTags(bag: Record<string, unknown>) {
  const { [TAGS_KEY]: tags, ...rest } = bag
  // The lift is a display choice, never a filter: a `tags` the chip row cannot
  // represent (a number, an object, a mixed array) stays a pair rather than
  // vanishing from a bag that used to round-trip every key through `MetaValue`.
  if (!isChipShaped(tags)) return { tags: [], rest: bag }
  return { tags: chipValues(tags), rest }
}

/** The blocks a descriptor + payload produce, in descriptor field order. */
function groupsOf(
  descriptor: ResourceDescriptor,
  resource: Record<string, unknown>
): Group[] {
  const groups: Group[] = []
  const push = (group: Group) => {
    if (group.kind === 'chips' && group.values.length === 0) return
    if (group.kind === 'pairs' && Object.keys(group.data).length === 0) return
    groups.push(group)
  }

  for (const field of descriptor.fields) {
    const value = valueOf(resource, field.key)
    if (field.role === 'taxonomy') {
      push({
        kind: 'chips',
        id: `taxonomy:${field.key}`,
        label: field.label,
        values: chipValues(value),
      })
      continue
    }
    // A `meta` field with its own renderer is a metadata ROW by construction
    // (`FieldSpec.render` is confined to that role), so it is never a bag.
    if (field.role !== 'meta' || field.render || !isPairBag(value)) continue
    const { tags, rest } = splitTags(value)
    push({
      kind: 'chips',
      id: `tags:${field.key}`,
      label: 'Tags',
      values: tags,
    })
    push({ kind: 'pairs', id: `pairs:${field.key}`, data: rest })
  }

  return groups
}

export interface ResourceTaxonomySectionProps {
  descriptor: ResourceDescriptor
  /** The API payload, read by descriptor field key. */
  resource: Record<string, unknown>
  className?: string
}

export function ResourceTaxonomySection({
  descriptor,
  resource,
  className,
}: Readonly<ResourceTaxonomySectionProps>) {
  const groups = groupsOf(descriptor, resource)
  // The single emptiness rule: nothing to group by renders no section at all,
  // rather than memory's old "No metadata." note against the other kinds'
  // silence.
  if (groups.length === 0) return null

  return (
    <Panel className={className} data-testid="taxonomy-section">
      <PanelHeader>
        <PanelTitle>Labels &amp; metadata</PanelTitle>
      </PanelHeader>
      <PanelBody className="space-y-3 pb-4">
        {groups.map(group => (
          <div key={group.id} className="space-y-1.5">
            {group.kind === 'chips' ? (
              <>
                <span className="text-muted-foreground block text-xs">
                  {group.label}
                </span>
                <TaxonomyChips values={group.values} />
              </>
            ) : (
              <AdditionalDataRows data={group.data} />
            )}
          </div>
        ))}
      </PanelBody>
    </Panel>
  )
}
