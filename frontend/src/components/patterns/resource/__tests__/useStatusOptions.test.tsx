import { renderHook } from '@testing-library/react'

import type { StatusDefinition } from '@/services/statusService'

import { agentDescriptor } from '../descriptors/agent'
import { artifactDescriptor } from '../descriptors/artifact'
import { blueprintDescriptor } from '../descriptors/blueprint'
import { memoryDescriptor } from '../descriptors/memory'
import { promptDescriptor } from '../descriptors/prompt'
import { statusResourceTypeOf, useStatusOptions } from '../useStatusOptions'

const catalog = vi.hoisted(() => ({
  statuses: [] as unknown[],
  isLoading: false,
  asked: [] as unknown[],
}))

vi.mock('@/hooks/useStatusCatalog', () => ({
  useStatusCatalog: (resourceType: unknown) => {
    catalog.asked.push(resourceType)
    return { statuses: catalog.statuses, isLoading: catalog.isLoading }
  },
}))

function status(
  slug: string,
  name: string,
  isSystem = false
): StatusDefinition {
  return {
    id: isSystem ? undefined : `id-${slug}`,
    resource_type: 'artifact',
    slug,
    name,
    is_system: isSystem,
    is_live: true,
    usage_count: 0,
  }
}

/** What the API returns for an artifact: the system statuses, then the team's. */
const ARTIFACT_CATALOG = [
  status('active', 'active', true),
  status('draft', 'draft', true),
  status('archived', 'archived', true),
  status('in-review', 'In review'),
  status('shipped', 'Shipped'),
]

describe('useStatusOptions', () => {
  beforeEach(() => {
    catalog.statuses = []
    catalog.isLoading = false
    catalog.asked = []
  })

  it('lists the system statuses in descriptor order, then the custom ones', () => {
    catalog.statuses = ARTIFACT_CATALOG

    const { result } = renderHook(() => useStatusOptions(artifactDescriptor))

    expect(result.current.options).toEqual([
      { value: 'active', label: 'Active' },
      { value: 'draft', label: 'Draft' },
      { value: 'archived', label: 'Archived' },
      { value: 'in-review', label: 'In review' },
      { value: 'shipped', label: 'Shipped' },
    ])
  })

  it('labels a system status from the descriptor, not from the catalog', () => {
    catalog.statuses = [status('active', 'ACTIVE (server wording)', true)]

    const { result } = renderHook(() => useStatusOptions(artifactDescriptor))

    expect(result.current.labelOf('active')).toBe('Active')
  })

  it('labels a custom status by its display name and an unknown value by itself', () => {
    catalog.statuses = ARTIFACT_CATALOG

    const { result } = renderHook(() => useStatusOptions(artifactDescriptor))

    expect(result.current.labelOf('in-review')).toBe('In review')
    expect(result.current.labelOf('deleted-elsewhere')).toBe(
      'deleted-elsewhere'
    )
  })

  it('tones a system status from the descriptor and everything else neutral', () => {
    catalog.statuses = ARTIFACT_CATALOG

    const { result } = renderHook(() => useStatusOptions(artifactDescriptor))

    expect(result.current.toneOf('active')).toBe('success')
    expect(result.current.toneOf('in-review')).toBe('neutral')
    expect(result.current.toneOf('deleted-elsewhere')).toBe('neutral')
  })

  it('knows system and custom values, and nothing else, once the catalog answered', () => {
    catalog.statuses = ARTIFACT_CATALOG

    const { result } = renderHook(() => useStatusOptions(artifactDescriptor))

    for (const value of ['active', 'in-review']) {
      expect(result.current.has(value)).toBe(true)
      expect(result.current.isPending(value)).toBe(false)
    }
    expect(result.current.has('bogus')).toBe(false)
    // Answered: an unknown value is dropped, not waited on.
    expect(result.current.isPending('bogus')).toBe(false)
  })

  // A reloaded `?status=in-review` can be neither sent (it may be junk, and
  // the API 400s) nor dropped (it may be a custom status) until the catalog
  // has had the chance to recognise it.
  it('offers the system statuses only while loading, and leaves any other value pending', () => {
    catalog.isLoading = true

    const { result } = renderHook(() => useStatusOptions(artifactDescriptor))

    expect(result.current.isLoading).toBe(true)
    expect(result.current.options.map(option => option.value)).toEqual([
      'active',
      'draft',
      'archived',
    ])
    expect(result.current.has('in-review')).toBe(false)
    expect(result.current.isPending('in-review')).toBe(true)
    // A system status needs no catalog to be recognised.
    expect(result.current.isPending('draft')).toBe(false)
  })

  it('falls back to the system statuses when the catalog has no answer', () => {
    // A failed request: nothing known, and no longer loading.
    const { result } = renderHook(() => useStatusOptions(artifactDescriptor))

    expect(result.current.options.map(option => option.value)).toEqual([
      'active',
      'draft',
      'archived',
    ])
    expect(result.current.has('in-review')).toBe(false)
    expect(result.current.isPending('in-review')).toBe(false)
    expect(result.current.labelOf('in-review')).toBe('in-review')
  })

  it('ignores a catalog row that reuses a system slug', () => {
    catalog.statuses = [status('active', 'Shadow')]

    const { result } = renderHook(() => useStatusOptions(artifactDescriptor))

    expect(result.current.options).toHaveLength(3)
    expect(result.current.labelOf('active')).toBe('Active')
  })

  it.each([
    ['prompt', promptDescriptor],
    ['artifact', artifactDescriptor],
    ['blueprint', blueprintDescriptor],
    ['memory', memoryDescriptor],
  ] as const)(
    'reads the %s catalog for the %s descriptor',
    (type, descriptor) => {
      renderHook(() => useStatusOptions(descriptor))

      expect(catalog.asked).toEqual([type])
      expect(statusResourceTypeOf(descriptor.kind)).toBe(type)
    }
  )

  it('reads no catalog for a kind whose status is closed', () => {
    const { result } = renderHook(() => useStatusOptions(agentDescriptor))

    expect(catalog.asked).toEqual([undefined])
    expect(result.current.options.map(option => option.value)).toEqual([
      'active',
      'paused',
      'error',
    ])
  })

  it('returns the same vocabulary object while nothing changed', () => {
    catalog.statuses = ARTIFACT_CATALOG

    const { result, rerender } = renderHook(() =>
      useStatusOptions(artifactDescriptor)
    )
    const first = result.current
    rerender()

    expect(result.current).toBe(first)
  })
})
