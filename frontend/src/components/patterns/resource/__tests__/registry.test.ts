import type { ResourceKind } from '@/components/resource-detail/ResourceReadingPage'

import {
  getResourceDescriptor,
  type ResourceKindKey,
  resourceRegistry,
} from '../registry'
import type { FieldRole, ResourceDescriptor } from '../types'

// The registry's keys must stay a superset of the kinds the shared side panels
// understand, so a descriptor can always back a `ResourceReadingPage`. Keying
// this map by `ResourceKind` makes it exhaustive: adding a member to that union
// fails `tsc -b` here until the registry gains a descriptor for it, and the
// `K & ResourceKindKey` value type forbids silencing that by mapping the new
// kind onto some other kind's descriptor.
const SIDE_PANEL_KINDS: { [K in ResourceKind]: K & ResourceKindKey } = {
  artifact: 'artifact',
  prompt: 'prompt',
  blueprint: 'blueprint',
  memory: 'memory',
}

function keysWithRole(descriptor: ResourceDescriptor, role: FieldRole) {
  return descriptor.fields.filter(f => f.role === role).map(f => f.key)
}

describe('resourceRegistry', () => {
  it('registers exactly the five built-in kinds', () => {
    expect(Object.keys(resourceRegistry)).toEqual([
      'prompt',
      'artifact',
      'blueprint',
      'memory',
      'gallery-prompt',
    ])
  })

  it('keys every descriptor by its own kind', () => {
    for (const [key, descriptor] of Object.entries(resourceRegistry)) {
      expect(descriptor.kind).toBe(key)
    }
  })

  // Object.freeze is shallow, so freezing the registry alone would leave every
  // descriptor — the object each page holds a reference to — writable. Walk
  // rather than enumerate levels, so the assertion pins "deep", not "4 deep".
  it('is frozen all the way down, so no consumer can mutate a descriptor', () => {
    const unfrozen: string[] = []
    const walk = (value: unknown, path: string) => {
      if (value === null || typeof value !== 'object') return
      if (!Object.isFrozen(value)) unfrozen.push(path)
      for (const [key, child] of Object.entries(value)) {
        walk(child, `${path}.${key}`)
      }
    }
    walk(resourceRegistry, 'resourceRegistry')
    expect(unfrozen).toEqual([])
  })

  it('looks a descriptor up by kind', () => {
    expect(getResourceDescriptor('blueprint')).toBe(resourceRegistry.blueprint)
  })

  it('covers every kind the shared side panels understand', () => {
    for (const kind of Object.values(SIDE_PANEL_KINDS)) {
      expect(getResourceDescriptor(kind).kind).toBe(kind)
    }
  })

  // Explicit per-kind assertions rather than a snapshot: a drift in the field
  // map should read as a named diff, not as snapshot churn.
  describe('field map', () => {
    it.each([
      ['prompt', 'name', 'body', ['slug']],
      ['artifact', 'title', 'content', ['project_id', 'slug']],
      ['blueprint', 'title', 'content', ['project_id', 'slug']],
      // #911 gave memory an optional `title`; it takes the `name` role and
      // `text` is the body alone, with every reader falling back to an excerpt.
      ['memory', 'title', 'text', ['id']],
      ['gallery-prompt', 'title', 'content', ['id']],
    ] as const)(
      '%s is named by `%s`, bodied by `%s` and addressed by %j',
      (kind, name, body, address) => {
        const descriptor = getResourceDescriptor(kind)
        expect(keysWithRole(descriptor, 'name')).toEqual([name])
        expect(keysWithRole(descriptor, 'body')).toEqual([body])
        expect(keysWithRole(descriptor, 'address')).toEqual(address)
      }
    )

    it('declares every field key the pages read today', () => {
      const keys = Object.fromEntries(
        Object.entries(resourceRegistry).map(([kind, descriptor]) => [
          kind,
          [...new Set(descriptor.fields.map(f => f.key))].sort((a, b) =>
            a.localeCompare(b)
          ),
        ])
      )
      expect(keys).toEqual({
        prompt: [
          'body',
          'description',
          'is_shared',
          'labels',
          'mcp_expose',
          'name',
          'project_id',
          'slug',
          'status',
        ],
        artifact: [
          'content',
          'description',
          'labels',
          'metadata',
          'project_id',
          'slug',
          'status',
          'title',
          'type',
        ],
        blueprint: [
          'content',
          'description',
          'labels',
          'metadata',
          'path',
          'project_id',
          'slug',
          'source.commit_sha',
          'source.imported_at',
          'source.repo',
          'status',
          'subtype',
          'title',
          'type',
        ],
        memory: [
          'id',
          'labels',
          'metadata',
          'project_id',
          'status',
          'text',
          'title',
        ],
        'gallery-prompt': [
          'category',
          'content',
          'description',
          'id',
          'tags',
          'title',
        ],
      })
    })
  })

  // Status values mirror each kind's OpenAPI enum until the shared
  // `ResourceStatus` enum lands (decision F). Blueprint is `active | expired`,
  // not `active | inactive`.
  it.each([
    ['prompt', ['draft', 'published']],
    ['artifact', ['active', 'draft', 'archived']],
    ['blueprint', ['active', 'expired']],
    ['memory', ['active', 'draft', 'archived']],
  ] as const)('declares %s statuses as %j', (kind, values) => {
    const status = getResourceDescriptor(kind).fields.find(
      f => f.role === 'status'
    )
    expect(status?.statusValues).toEqual(values)
  })

  it('gives the public gallery prompt no status', () => {
    expect(keysWithRole(resourceRegistry['gallery-prompt'], 'status')).toEqual(
      []
    )
  })

  describe('capabilities', () => {
    it.each([
      [
        'prompt',
        {
          attachments: true,
          versions: true,
          comments: true,
          relations: true,
          mcp: true,
        },
      ],
      [
        'artifact',
        {
          attachments: true,
          versions: true,
          comments: true,
          relations: true,
          mcp: false,
        },
      ],
      [
        'blueprint',
        {
          attachments: true,
          versions: true,
          comments: true,
          relations: true,
          mcp: false,
        },
      ],
      // MemoryView renders `attachments={false}`.
      [
        'memory',
        {
          attachments: false,
          versions: true,
          comments: true,
          relations: true,
          mcp: false,
        },
      ],
    ] as const)('%s', (kind, capabilities) => {
      expect(getResourceDescriptor(kind).capabilities).toEqual(capabilities)
    })

    // The gallery is served by the public API and has no team-scoped resource
    // id, so no shared side panel can address it.
    it('gallery-prompt is read-only with every capability off', () => {
      const descriptor = resourceRegistry['gallery-prompt']
      expect(descriptor.readOnly).toBe(true)
      expect(Object.values(descriptor.capabilities)).toEqual([
        false,
        false,
        false,
        false,
        false,
      ])
    })

    it('marks only the gallery prompt read-only', () => {
      const readOnly = Object.entries(resourceRegistry)
        .filter(([, d]) => d.readOnly)
        .map(([kind]) => kind)
      expect(readOnly).toEqual(['gallery-prompt'])
    })
  })
})
