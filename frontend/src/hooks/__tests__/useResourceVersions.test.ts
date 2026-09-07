import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useResourceVersions } from '@/hooks/useResourceVersions'

interface TestVersion {
  version_number: number
}

interface VersionListResponse {
  versions: TestVersion[]
}

/** A promise whose settlement this test controls, to order two in-flight fetches. */
function deferred() {
  let resolve!: (value: VersionListResponse) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<VersionListResponse>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const version = (version_number: number): TestVersion => ({ version_number })

/** Lets a settled fetch's `.then`/`.catch` continuations run to completion. */
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
      useResourceVersions<TestVersion>({
        fetch: () => Promise.resolve({ versions: [] }),
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
      useResourceVersions<TestVersion>({
        // Deliberately out of order: the current version is one past the
        // HIGHEST retained snapshot, not past the last element.
        fetch: () =>
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
      useResourceVersions<TestVersion>({
        fetch: () => Promise.resolve({ versions: [version(1)] }),
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
      useResourceVersions<TestVersion>({
        fetch: () => Promise.reject(new Error('versions boom')),
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

  it('fetches nothing until the caller is ready', async () => {
    const fetchVersions = vi
      .fn<() => Promise<VersionListResponse>>()
      .mockResolvedValue({ versions: [version(1)] })

    const { result, rerender } = renderHook(
      ({ ready }: { ready: boolean }) =>
        useResourceVersions<TestVersion>({
          fetch: ready ? fetchVersions : null,
          to: ready ? '/memories/m1/versions' : undefined,
          deps: [ready],
        }),
      { initialProps: { ready: false } }
    )

    await flushMicrotasks()
    expect(fetchVersions).not.toHaveBeenCalled()
    expect(result.current.loading).toBe(false)
    expect(result.current.versions).toEqual([])
    expect(result.current.versionHistory).toBeUndefined()

    rerender({ ready: true })
    await waitFor(() => {
      expect(result.current.versionHistory?.count).toBe(1)
    })
    expect(fetchVersions).toHaveBeenCalledTimes(1)
  })

  it('discards a response that lands after the identity changed', async () => {
    const first = deferred()
    const second = deferred()
    const pending = [first, second]
    let call = 0
    const fetchVersions = vi.fn(() => pending[call++].promise)

    const { result, rerender } = renderHook(
      ({ slug }: { slug: string }) =>
        useResourceVersions<TestVersion>({
          fetch: fetchVersions,
          to: `/prompts/${slug}/versions`,
          deps: [slug],
        }),
      { initialProps: { slug: 'first-prompt' } }
    )

    // Navigate to another prompt while the first request is still in flight.
    rerender({ slug: 'second-prompt' })
    expect(fetchVersions).toHaveBeenCalledTimes(2)

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
    const fetchVersions = vi.fn(() => pending[call++].promise)

    const { result, rerender } = renderHook(
      ({ slug }: { slug: string }) =>
        useResourceVersions<TestVersion>({
          fetch: fetchVersions,
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

  it('refetches when the identity changes', async () => {
    const fetchVersions = vi
      .fn<() => Promise<VersionListResponse>>()
      .mockResolvedValueOnce({ versions: [version(1)] })
      .mockResolvedValueOnce({ versions: [version(1), version(2)] })

    const { result, rerender } = renderHook(
      ({ slug }: { slug: string }) =>
        useResourceVersions<TestVersion>({
          fetch: fetchVersions,
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
    expect(fetchVersions).toHaveBeenCalledTimes(2)
  })
})
