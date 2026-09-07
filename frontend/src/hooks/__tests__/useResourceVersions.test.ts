import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useResourceVersions } from '@/hooks/useResourceVersions'
import type {
  ContentVersion,
  ResourceVersionListResponse,
} from '@/types/version'

/** A promise whose settlement this test controls, to order two in-flight loads. */
function deferred() {
  let resolve!: (value: ResourceVersionListResponse) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<ResourceVersionListResponse>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const version = (version_number: number): ContentVersion => ({
  id: `version-${String(version_number)}`,
  team_id: 'team-1',
  resource_type: 'prompt',
  resource_id: 'resource-1',
  version_number,
  content: 'snapshot content',
  change_summary: null,
  actor_type: 'human',
  created_by: null,
  author: null,
  created_at: '2026-09-01T09:00:00Z',
})

/** Lets a settled load's `.then`/`.catch` continuations run to completion. */
async function flushMicrotasks() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('useResourceVersions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('produces no affordance when the resource has no snapshots', async () => {
    const { result } = renderHook(() =>
      useResourceVersions({
        loadVersions: () => Promise.resolve({ versions: [] }),
        to: '/artifacts/proj/art/versions',
        deps: ['art'],
      })
    )

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.versions).toEqual([])
    expect(result.current.versionHistory).toBeUndefined()
  })

  it('derives the count, the current version, the edit time and the route', async () => {
    const { result } = renderHook(() =>
      useResourceVersions({
        // Deliberately out of order: the current version is one past the
        // HIGHEST retained snapshot, not past the last element.
        loadVersions: () =>
          Promise.resolve({ versions: [version(1), version(3), version(2)] }),
        to: '/artifacts/proj/art/versions',
        editedAt: '2026-09-01T10:00:00Z',
        deps: ['art'],
      })
    )

    await waitFor(() => {
      expect(result.current.versions).toHaveLength(3)
    })
    expect(result.current.versionHistory).toEqual({
      count: 3,
      currentVersion: 4,
      editedAt: '2026-09-01T10:00:00Z',
      to: '/artifacts/proj/art/versions',
    })
  })

  it('withholds the affordance until the route is known', async () => {
    const { result } = renderHook(() =>
      useResourceVersions({
        loadVersions: () => Promise.resolve({ versions: [version(1)] }),
        // The route is built from the resource payload, which has not arrived.
        to: undefined,
        deps: ['art'],
      })
    )

    await waitFor(() => {
      expect(result.current.versions).toHaveLength(1)
    })
    expect(result.current.versionHistory).toBeUndefined()
  })

  it('treats a failed load as "no history" rather than an error', async () => {
    const { result } = renderHook(() =>
      useResourceVersions({
        loadVersions: () => Promise.reject(new Error('versions boom')),
        to: '/memories/m1/versions',
        deps: ['m1'],
      })
    )

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })
    expect(result.current.versions).toEqual([])
    expect(result.current.versionHistory).toBeUndefined()
  })

  it('loads nothing until the caller is ready', async () => {
    const loadVersions = vi
      .fn<() => Promise<ResourceVersionListResponse>>()
      .mockResolvedValue({ versions: [version(1)] })

    const { result, rerender } = renderHook(
      ({ ready }: { ready: boolean }) =>
        useResourceVersions({
          loadVersions: ready ? loadVersions : null,
          to: ready ? '/memories/m1/versions' : undefined,
          deps: [ready],
        }),
      { initialProps: { ready: false } }
    )

    await flushMicrotasks()
    expect(loadVersions).not.toHaveBeenCalled()
    expect(result.current.loading).toBe(false)
    expect(result.current.versions).toEqual([])
    expect(result.current.versionHistory).toBeUndefined()

    rerender({ ready: true })
    await waitFor(() => {
      expect(result.current.versionHistory?.count).toBe(1)
    })
    expect(loadVersions).toHaveBeenCalledTimes(1)
  })

  it('drops the previous resource’s history the moment the identity changes', async () => {
    const first = deferred()
    const second = deferred()
    const pending = [first, second]
    let call = 0
    const loadVersions = vi.fn(() => pending[call++].promise)

    const { result, rerender } = renderHook(
      ({ slug }: { slug: string }) =>
        useResourceVersions({
          loadVersions,
          to: `/prompts/${slug}/versions`,
          deps: [slug],
        }),
      { initialProps: { slug: 'first-prompt' } }
    )

    first.resolve({ versions: [version(1), version(2), version(3)] })
    await waitFor(() => {
      expect(result.current.versionHistory?.count).toBe(3)
    })

    // `to` follows the new prompt immediately, so holding the old count would
    // pair the first prompt's history with the second prompt's link.
    rerender({ slug: 'second-prompt' })
    await flushMicrotasks()
    expect(result.current.versions).toEqual([])
    expect(result.current.versionHistory).toBeUndefined()

    second.resolve({ versions: [version(1)] })
    await waitFor(() => {
      expect(result.current.versionHistory?.count).toBe(1)
    })
  })

  it('discards a response that lands after the identity changed', async () => {
    const first = deferred()
    const second = deferred()
    const pending = [first, second]
    let call = 0
    const loadVersions = vi.fn(() => pending[call++].promise)

    const { result, rerender } = renderHook(
      ({ slug }: { slug: string }) =>
        useResourceVersions({
          loadVersions,
          to: `/prompts/${slug}/versions`,
          deps: [slug],
        }),
      { initialProps: { slug: 'first-prompt' } }
    )

    // Navigate to another prompt while the first request is still in flight.
    rerender({ slug: 'second-prompt' })
    expect(loadVersions).toHaveBeenCalledTimes(2)

    second.resolve({ versions: [version(1)] })
    await waitFor(() => {
      expect(result.current.versionHistory?.count).toBe(1)
    })

    // Only now does the abandoned first prompt's slower response arrive.
    first.resolve({ versions: [version(1), version(2), version(3)] })
    await flushMicrotasks()

    expect(result.current.versions).toHaveLength(1)
    expect(result.current.versionHistory).toEqual({
      count: 1,
      currentVersion: 2,
      editedAt: undefined,
      to: '/prompts/second-prompt/versions',
    })
  })

  it('discards a failure that lands after the identity changed', async () => {
    const first = deferred()
    const second = deferred()
    const pending = [first, second]
    let call = 0
    const loadVersions = vi.fn(() => pending[call++].promise)

    const { result, rerender } = renderHook(
      ({ slug }: { slug: string }) =>
        useResourceVersions({
          loadVersions,
          to: `/prompts/${slug}/versions`,
          deps: [slug],
        }),
      { initialProps: { slug: 'first-prompt' } }
    )

    rerender({ slug: 'second-prompt' })
    second.resolve({ versions: [version(4)] })
    await waitFor(() => {
      expect(result.current.versionHistory?.count).toBe(1)
    })

    first.reject(new Error('stale versions boom'))
    await flushMicrotasks()

    // The stale rejection must not clear the current prompt's history.
    expect(result.current.versionHistory).toEqual({
      count: 1,
      currentVersion: 5,
      editedAt: undefined,
      to: '/prompts/second-prompt/versions',
    })
  })

  it('reloads when the identity changes', async () => {
    const loadVersions = vi
      .fn<() => Promise<ResourceVersionListResponse>>()
      .mockResolvedValueOnce({ versions: [version(1)] })
      .mockResolvedValueOnce({ versions: [version(1), version(2)] })

    const { result, rerender } = renderHook(
      ({ slug }: { slug: string }) =>
        useResourceVersions({
          loadVersions,
          to: `/prompts/${slug}/versions`,
          deps: [slug],
        }),
      { initialProps: { slug: 'first-prompt' } }
    )

    await waitFor(() => {
      expect(result.current.versionHistory?.count).toBe(1)
    })

    rerender({ slug: 'second-prompt' })
    await waitFor(() => {
      expect(result.current.versionHistory?.count).toBe(2)
    })
    expect(result.current.versionHistory?.to).toBe(
      '/prompts/second-prompt/versions'
    )
    expect(loadVersions).toHaveBeenCalledTimes(2)
  })
})
