import { defineResource } from '../defineResource'
import type { FieldSpec, ResourceDescriptor, ResourceFormSpec } from '../types'

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
  describe('list spec', () => {
    const STATUS: FieldSpec = {
      key: 'status',
      role: 'status',
      label: 'Status',
      statusValues: ['active', 'archived'],
    }
    const LABELS: FieldSpec = {
      key: 'labels',
      role: 'taxonomy',
      label: 'Labels',
    }

    function withList(list: ResourceDescriptor['list']): ResourceDescriptor {
      return descriptor([NAME, SLUG, STATUS, LABELS], { list })
    }

    it('accepts a descriptor with no list section at all', () => {
      const valid = descriptor([NAME, SLUG])
      expect(defineResource(valid)).toBe(valid)
    })

    it('accepts a list section naming declared fields', () => {
      const valid = withList({
        filters: [
          { key: 'search', control: 'search', label: 'Search widgets' },
          {
            key: 'status',
            control: 'select',
            label: 'Filter by status',
            optionsFrom: 'field',
          },
          {
            key: 'labels',
            control: 'taxonomy',
            label: 'Filter by labels',
            optionsFrom: 'prompt-labels',
          },
        ],
        sortable: ['title', 'updated_at'],
      })
      expect(defineResource(valid)).toBe(valid)
    })

    it('throws when a select filter names no declared field', () => {
      expect(() =>
        defineResource(
          withList({
            filters: [
              {
                key: 'severity',
                control: 'select',
                label: 'Filter by severity',
                optionsFrom: 'field',
              },
            ],
            sortable: ['title'],
          })
        )
      ).toThrow(/filter 'severity' names no declared field/)
    })

    it('throws when a select filter declares no options source', () => {
      expect(() =>
        defineResource(
          withList({
            filters: [
              { key: 'status', control: 'select', label: 'Filter by status' },
            ],
            sortable: ['title'],
          })
        )
      ).toThrow(/needs optionsFrom \["field","types"\], found null/)
    })

    it('throws when a taxonomy filter reads options from the field', () => {
      expect(() =>
        defineResource(
          withList({
            filters: [
              {
                key: 'labels',
                control: 'taxonomy',
                label: 'Filter by labels',
                optionsFrom: 'field',
              },
            ],
            sortable: ['title'],
          })
        )
      ).toThrow(/needs optionsFrom \["prompt-labels"\], found "field"/)
    })

    it('rejects valueLabels as an option set — it is partial by design', () => {
      expect(() =>
        defineResource(
          descriptor(
            [
              NAME,
              SLUG,
              {
                key: 'kind',
                role: 'type',
                label: 'Kind',
                // Labelled but not enumerated: the artifact `type` shape.
                valueLabels: { work_reports: 'Work reports' },
              },
            ],
            {
              list: {
                filters: [
                  {
                    key: 'kind',
                    control: 'select',
                    label: 'Filter by kind',
                    optionsFrom: 'field',
                  },
                ],
                sortable: ['title'],
              },
            }
          )
        )
      ).toThrow(/reads options from its field, which enumerates none/)
    })

    it('throws when a list-level control declares an options source', () => {
      expect(() =>
        defineResource(
          withList({
            filters: [
              {
                key: 'search',
                control: 'search',
                label: 'Search widgets',
                optionsFrom: 'types',
              },
            ],
            sortable: ['title'],
          })
        )
      ).toThrow(/has control 'search' but declares 'optionsFrom'/)
    })

    it('throws when a metadata filter names a plural the API has no catalog for', () => {
      // The bar addresses the catalog by `plural`, which is a plain string —
      // `resource_type` is an enum of exactly artifacts/blueprints/memories.
      expect(() =>
        defineResource(
          descriptor([NAME, SLUG], {
            plural: 'widgets',
            list: {
              filters: [
                { key: 'metadata', control: 'metadata', label: 'Filter' },
              ],
              sortable: ['title'],
            },
          })
        )
      ).toThrow(
        /metadata filter needs a plural the metadata API knows, found 'widgets'/
      )
    })

    it('throws on a duplicate filter key', () => {
      expect(() =>
        defineResource(
          withList({
            filters: [
              {
                key: 'status',
                control: 'select',
                label: 'Filter by status',
                optionsFrom: 'field',
              },
              {
                key: 'status',
                control: 'select',
                label: 'Filter by status again',
                optionsFrom: 'field',
              },
            ],
            sortable: ['title'],
          })
        )
      ).toThrow(/duplicate filter key 'status'/)
    })

    it('throws when a sortable key names no declared field', () => {
      expect(() =>
        defineResource(withList({ filters: [], sortable: ['headline'] }))
      ).toThrow(/sortable key 'headline' names no declared field/)
    })

    it('throws on a duplicate sortable key', () => {
      expect(() =>
        defineResource(withList({ filters: [], sortable: ['title', 'title'] }))
      ).toThrow(/duplicate sortable key 'title'/)
    })

    it('accepts the timestamps no descriptor declares as fields', () => {
      const valid = withList({
        filters: [],
        sortable: ['created_at', 'updated_at'],
      })
      expect(defineResource(valid)).toBe(valid)
    })
  })

  describe('form spec', () => {
    const STATUS: FieldSpec = {
      key: 'status',
      role: 'status',
      label: 'Status',
      statusValues: ['active', 'archived'],
    }
    const OPEN_TYPE: FieldSpec = { key: 'type', role: 'type', label: 'Type' }
    const BODY: FieldSpec = { key: 'content', role: 'body', label: 'Content' }
    const PROJECT: FieldSpec = {
      key: 'project_id',
      role: 'meta',
      label: 'Project',
    }
    const LABELS: FieldSpec = {
      key: 'labels',
      role: 'taxonomy',
      label: 'Labels',
    }
    const FIELDS = [NAME, SLUG, STATUS, OPEN_TYPE, BODY, PROJECT, LABELS]

    function withForm(form: ResourceFormSpec, fields = FIELDS) {
      return descriptor(fields, { form })
    }

    it('accepts a well-formed spec covering every control', () => {
      const valid = withForm({
        fields: [
          { key: 'title', control: 'text', section: 'details', required: true },
          {
            key: 'slug',
            control: 'text',
            section: 'details',
            required: true,
            pattern: 'slug',
          },
          {
            key: 'status',
            control: 'select',
            section: 'details',
            optionsFrom: 'field',
          },
          {
            key: 'type',
            control: 'select',
            section: 'details',
            optionsFrom: 'types',
          },
          { key: 'project_id', control: 'project', section: 'details' },
          { key: 'content', control: 'body', section: 'body' },
          { key: 'labels', control: 'taxonomy', section: 'taxonomy' },
        ],
        extensions: ['settings'],
      })
      expect(defineResource(valid)).toBe(valid)
    })

    it('throws when a form field names no declared field', () => {
      expect(() =>
        defineResource(
          withForm({
            fields: [{ key: 'headline', control: 'text', section: 'details' }],
          })
        )
      ).toThrow(/form field 'headline' names no declared field/)
    })

    it('throws on a duplicate form field key', () => {
      expect(() =>
        defineResource(
          withForm({
            fields: [
              { key: 'title', control: 'text', section: 'details' },
              { key: 'title', control: 'textarea', section: 'details' },
            ],
          })
        )
      ).toThrow(/duplicate form field 'title'/)
    })

    it('throws when a control cannot serve the field’s role', () => {
      expect(() =>
        defineResource(
          withForm({
            fields: [{ key: 'title', control: 'select', section: 'details' }],
          })
        )
      ).toThrow(/form field 'title' has control 'select', which serves/)
    })

    it('accepts a body control on a key that carries the body role among others', () => {
      const dual: FieldSpec[] = [
        { key: 'text', role: 'name', label: 'Memory' },
        { key: 'text', role: 'body', label: 'Memory' },
        SLUG,
      ]
      const valid = withForm(
        { fields: [{ key: 'text', control: 'body', section: 'body' }] },
        dual
      )
      expect(defineResource(valid)).toBe(valid)
    })

    it('throws when a control sits in the wrong section', () => {
      expect(() =>
        defineResource(
          withForm({
            fields: [{ key: 'content', control: 'body', section: 'details' }],
          })
        )
      ).toThrow(/belongs in the 'body' section, found 'details'/)
    })

    it('throws when a select declares no options source', () => {
      expect(() =>
        defineResource(
          withForm({
            fields: [{ key: 'status', control: 'select', section: 'details' }],
          })
        )
      ).toThrow(/form field 'status' with control 'select' needs 'optionsFrom'/)
    })

    it('throws when a select reads options from a field that enumerates none', () => {
      expect(() =>
        defineResource(
          withForm({
            fields: [
              {
                key: 'type',
                control: 'select',
                section: 'details',
                optionsFrom: 'field',
              },
            ],
          })
        )
      ).toThrow(
        /form field 'type' reads options from its field, which enumerates none/
      )
    })

    it('throws when a control that has no options declares an options source', () => {
      expect(() =>
        defineResource(
          withForm({
            fields: [
              {
                key: 'title',
                control: 'text',
                section: 'details',
                optionsFrom: 'field',
              },
            ],
          })
        )
      ).toThrow(/has control 'text' but declares 'optionsFrom'/)
    })

    it('throws when a required field uses a control that captures no required value', () => {
      expect(() =>
        defineResource(
          withForm({
            fields: [
              {
                key: 'labels',
                control: 'taxonomy',
                section: 'taxonomy',
                required: true,
              },
            ],
          })
        )
      ).toThrow(/is required but control 'taxonomy' captures no required value/)
    })

    it('throws when a patterned field is not required', () => {
      expect(() =>
        defineResource(
          withForm({
            fields: [
              {
                key: 'slug',
                control: 'text',
                section: 'details',
                pattern: 'slug',
              },
            ],
          })
        )
      ).toThrow(/declares pattern 'slug' but is not required/)
    })

    it('throws on more than one project control', () => {
      const twoProjects: FieldSpec[] = [
        ...FIELDS,
        { key: 'owner_id', role: 'meta', label: 'Owner' },
      ]
      expect(() =>
        defineResource(
          withForm(
            {
              fields: [
                { key: 'project_id', control: 'project', section: 'details' },
                { key: 'owner_id', control: 'project', section: 'details' },
              ],
            },
            twoProjects
          )
        )
      ).toThrow(/expected at most one 'project' form control, found 2/)
    })

    it('throws on a duplicate extension name', () => {
      expect(() =>
        defineResource(
          withForm({ fields: [], extensions: ['settings', 'settings'] })
        )
      ).toThrow(/duplicate form extension 'settings'/)
    })

    it('throws on an empty extension name', () => {
      expect(() =>
        defineResource(withForm({ fields: [], extensions: ['  '] }))
      ).toThrow(/form declares an empty extension name/)
    })

    it('throws when a read-only kind declares a form', () => {
      expect(() =>
        defineResource(
          descriptor(FIELDS, {
            readOnly: true,
            form: {
              fields: [{ key: 'title', control: 'text', section: 'details' }],
            },
          })
        )
      ).toThrow(/a read-only resource declares a form spec/)
    })

    it('accepts a descriptor with no form at all', () => {
      const valid = descriptor([NAME, SLUG])
      expect(defineResource(valid)).toBe(valid)
    })
  })
})
