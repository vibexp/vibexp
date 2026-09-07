import { defineResource } from '../defineResource'
import type { FieldSpec, ResourceDescriptor } from '../types'

const NAME: FieldSpec = { key: 'title', role: 'name', label: 'Title' }
const SLUG: FieldSpec = { key: 'slug', role: 'address', label: 'Slug' }

function descriptor(
  fields: readonly FieldSpec[],
  overrides: Partial<ResourceDescriptor> = {}
): ResourceDescriptor {
  return {
    kind: 'widget',
    singular: 'widget',
    plural: 'widgets',
    address: ['slug'],
    fields,
    capabilities: {
      attachments: false,
      versions: false,
      comments: false,
      relations: false,
      mcp: false,
    },
    ...overrides,
  }
}

describe('defineResource', () => {
  it('returns the descriptor unchanged when it is valid', () => {
    const valid = descriptor([NAME, SLUG])
    expect(defineResource(valid)).toBe(valid)
  })

  it('names the kind in the error so a module-load failure is locatable', () => {
    expect(() => defineResource(descriptor([SLUG]))).toThrow(
      /^defineResource\(widget\):/
    )
  })

  describe('exactly one name field', () => {
    it('throws when there is none', () => {
      expect(() => defineResource(descriptor([SLUG]))).toThrow(
        /exactly one 'name' field, found 0/
      )
    })

    it('throws when there is more than one', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            { key: 'alias', role: 'name', label: 'Alias' },
            SLUG,
          ])
        )
      ).toThrow(/exactly one 'name' field, found 2/)
    })
  })

  describe('at most one body field', () => {
    it('throws when there is more than one', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            SLUG,
            { key: 'content', role: 'body', label: 'Content' },
            { key: 'text', role: 'body', label: 'Text' },
          ])
        )
      ).toThrow(/at most one 'body' field, found 2/)
    })

    it('accepts zero', () => {
      expect(() => defineResource(descriptor([NAME, SLUG]))).not.toThrow()
    })
  })

  describe('duplicate fields', () => {
    it('throws when the same key repeats in the same role', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            SLUG,
            { key: 'slug', role: 'address', label: 'Slug' },
          ])
        )
      ).toThrow(/duplicate field 'slug' with role 'address'/)
    })

    // Memory's `text` is both its name (lists show a truncated excerpt) and its
    // body, so uniqueness is on the (key, role) pair.
    it('accepts the same key under two different roles', () => {
      expect(() =>
        defineResource(
          descriptor([
            { key: 'text', role: 'name', label: 'Memory' },
            { key: 'text', role: 'body', label: 'Memory' },
            SLUG,
          ])
        )
      ).not.toThrow()
    })
  })

  describe('address fields match the route shape', () => {
    it('throws when there are too few', () => {
      expect(() =>
        defineResource(
          descriptor([NAME, SLUG], { address: ['project', 'slug'] })
        )
      ).toThrow(/needs 2 'address' field\(s\), found 1/)
    })

    it('throws when there are too many', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            { key: 'project_id', role: 'address', label: 'Project' },
            SLUG,
          ])
        )
      ).toThrow(/needs 1 'address' field\(s\), found 2/)
    })

    it('throws when a field does not name its segment', () => {
      expect(() =>
        defineResource(
          descriptor([NAME, { key: 'id', role: 'address', label: 'ID' }])
        )
      ).toThrow(
        /address segment 'slug' expects a field keyed 'slug' or 'slug_id', found 'id'/
      )
    })

    it('throws when the address fields are declared out of order', () => {
      expect(() =>
        defineResource(
          descriptor(
            [
              NAME,
              SLUG,
              { key: 'project_id', role: 'address', label: 'Project' },
            ],
            { address: ['project', 'slug'] }
          )
        )
      ).toThrow(/address segment 'project' expects a field keyed 'project'/)
    })

    // `/artifacts/:project/:slug` is built from `artifact.project_id`.
    it('accepts an `_id`-suffixed key for a segment', () => {
      expect(() =>
        defineResource(
          descriptor(
            [
              NAME,
              { key: 'project_id', role: 'address', label: 'Project' },
              SLUG,
            ],
            { address: ['project', 'slug'] }
          )
        )
      ).not.toThrow()
    })
  })

  describe('status values', () => {
    it('throws when a toned value is not a declared status', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            SLUG,
            {
              key: 'status',
              role: 'status',
              label: 'Status',
              statusValues: ['active', 'expired'],
              tone: { active: 'success', inactive: 'neutral' },
            },
          ])
        )
      ).toThrow(/status value 'inactive' is not one of \["active","expired"\]/)
    })

    it('throws when a status field declares an empty value list', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            SLUG,
            {
              key: 'status',
              role: 'status',
              label: 'Status',
              statusValues: [],
            },
          ])
        )
      ).toThrow(/status field 'status' declares no status values/)
    })

    it('throws when a status field declares no values', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            SLUG,
            { key: 'status', role: 'status', label: 'Status' },
          ])
        )
      ).toThrow(/status field 'status' declares no status values/)
    })

    it('throws when a non-status field declares status values', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            SLUG,
            {
              key: 'type',
              role: 'type',
              label: 'Type',
              statusValues: ['general'],
            },
          ])
        )
      ).toThrow(/field 'type' has role 'type' but declares status metadata/)
    })
  })

  // `render` is the escape hatch for the handful of non-scalar metadata rows.
  // Confining it to `meta` is what stops it becoming a general per-page layout
  // slot, so the guard needs its own coverage — without this, deleting it
  // leaves the suite green.
  describe('render is meta-only', () => {
    it('accepts a render on a meta field', () => {
      const valid = descriptor([
        NAME,
        SLUG,
        { key: 'path', role: 'meta', label: 'Path', render: () => null },
      ])
      expect(defineResource(valid)).toBe(valid)
    })

    it('throws when a non-meta field declares a render', () => {
      expect(() =>
        defineResource(
          descriptor([
            { key: 'title', role: 'name', label: 'Title', render: () => null },
            SLUG,
          ])
        )
      ).toThrow(
        /field 'title' declares 'render' but has role 'name' \(only 'meta' may\)/
      )
    })
  })

  describe('value labels', () => {
    it('throws when a role other than status or type declares them', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            SLUG,
            {
              key: 'path',
              role: 'meta',
              label: 'Path',
              valueLabels: { a: 'A' },
            },
          ])
        )
      ).toThrow(
        /field 'path' declares 'valueLabels' but has role 'meta' \(only 'status' or 'type' may\)/
      )
    })

    it('throws when a status label names a value the resource cannot be in', () => {
      expect(() =>
        defineResource(
          descriptor([
            NAME,
            SLUG,
            {
              key: 'status',
              role: 'status',
              label: 'Status',
              statusValues: ['active'],
              valueLabels: { retired: 'Retired' },
            },
          ])
        )
      ).toThrow(/status value 'retired' is not one of \["active"\]/)
    })

    it('accepts open-ended labels on a type field', () => {
      const valid = descriptor([
        NAME,
        SLUG,
        {
          key: 'type',
          role: 'type',
          label: 'Type',
          valueLabels: { work_reports: 'Work reports' },
        },
      ])
      expect(defineResource(valid)).toBe(valid)
    })
  })
})
