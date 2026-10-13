import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { usePromptRenderedTab } from '../usePromptRenderedTab'

type Options = Parameters<typeof usePromptRenderedTab>[0]

const team = { id: 'team-1' }

function setup(overrides: Partial<Options> = {}) {
  const fetchPlaceholders = vi
    .fn<Options['fetchPlaceholders']>()
    .mockResolvedValue(undefined)
  const renderPrompt = vi
    .fn<Options['renderPrompt']>()
    .mockResolvedValue(undefined)
  const initialProps: Options = {
    slug: 'greeting',
    tab: 'rendered',
    currentTeam: team,
    fetchPlaceholders,
    renderPrompt,
    isLoadingPlaceholders: false,
    placeholderValues: {},
    placeholderCount: 0,
    ...overrides,
  }
  const view = renderHook(
    (props: Options) => {
      usePromptRenderedTab(props)
    },
    { initialProps }
  )
  const rerender = (next: Partial<Options>) => {
    Object.assign(initialProps, next)
    view.rerender({ ...initialProps })
  }
  return { fetchPlaceholders, renderPrompt, rerender, unmount: view.unmount }
}

/** Lets the `fetchPlaceholders().then(...)` continuation run. */
async function flushFetch() {
  await act(async () => {
    await Promise.resolve()
  })
}

/**
 * Takes a hook past its first load with one placeholder on screen, so the
 * debounce effect is the only thing left that can call `renderPrompt`.
 *
 * The 500 ms is not padding: the placeholder values arriving count as a
 * change, so a first load is followed by one debounced render of its own.
 */
async function setupLoaded() {
  const hook = setup({ placeholderValues: { name: '' }, placeholderCount: 1 })
  await flushFetch()
  act(() => {
    vi.advanceTimersByTime(500)
  })
  hook.renderPrompt.mockClear()
  return hook
}

describe('usePromptRenderedTab', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  describe('first load', () => {
    it('fetches the placeholders, then renders 100 ms later', async () => {
      const { fetchPlaceholders, renderPrompt } = setup()

      expect(fetchPlaceholders).toHaveBeenCalledExactlyOnceWith(
        'greeting',
        'team-1'
      )
      await flushFetch()
      expect(renderPrompt).not.toHaveBeenCalled()

      act(() => {
        vi.advanceTimersByTime(99)
      })
      expect(renderPrompt).not.toHaveBeenCalled()

      act(() => {
        vi.advanceTimersByTime(1)
      })
      expect(renderPrompt).toHaveBeenCalledExactlyOnceWith('greeting', 'team-1')
    })

    it.each<[string, Partial<Options>]>([
      ['the Raw tab is showing', { tab: 'raw' }],
      ['the prompt has not loaded', { slug: undefined }],
      ['no team is selected', { currentTeam: null }],
    ])('does nothing while %s', async (_name, overrides) => {
      const { fetchPlaceholders, renderPrompt } = setup(overrides)

      await flushFetch()
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      expect(fetchPlaceholders).not.toHaveBeenCalled()
      expect(renderPrompt).not.toHaveBeenCalled()
    })

    it('starts loading when the reader switches to the Rendered tab', () => {
      const { fetchPlaceholders, rerender } = setup({ tab: 'raw' })
      expect(fetchPlaceholders).not.toHaveBeenCalled()

      rerender({ tab: 'rendered' })

      expect(fetchPlaceholders).toHaveBeenCalledExactlyOnceWith(
        'greeting',
        'team-1'
      )
    })

    it('fetches again for a different prompt', () => {
      const { fetchPlaceholders, rerender } = setup()

      rerender({ slug: 'farewell' })

      expect(fetchPlaceholders).toHaveBeenCalledTimes(2)
      expect(fetchPlaceholders).toHaveBeenLastCalledWith('farewell', 'team-1')
    })
  })

  describe('an already-loaded prompt', () => {
    it('renders straight away when the effect re-fires', () => {
      const { fetchPlaceholders, renderPrompt, rerender } = setup()

      // Coming back from Raw re-fires the effect for the same slug.
      rerender({ tab: 'raw' })
      rerender({ tab: 'rendered' })

      expect(fetchPlaceholders).toHaveBeenCalledTimes(1)
      expect(renderPrompt).toHaveBeenCalledExactlyOnceWith('greeting', 'team-1')
    })

    it('waits for the placeholders to finish loading', () => {
      const { renderPrompt, rerender } = setup()

      rerender({ isLoadingPlaceholders: true })
      expect(renderPrompt).not.toHaveBeenCalled()

      rerender({ isLoadingPlaceholders: false })
      expect(renderPrompt).toHaveBeenCalledExactlyOnceWith('greeting', 'team-1')
    })
  })

  describe('placeholder values', () => {
    it('renders once more, 500 ms in, when the first values arrive', async () => {
      const { renderPrompt } = setup({
        placeholderValues: { name: '' },
        placeholderCount: 1,
      })
      await flushFetch()

      act(() => {
        vi.advanceTimersByTime(100)
      })
      expect(renderPrompt).toHaveBeenCalledTimes(1)

      act(() => {
        vi.advanceTimersByTime(400)
      })
      expect(renderPrompt).toHaveBeenCalledTimes(2)
    })

    it('re-renders 500 ms after a value changes', async () => {
      const { renderPrompt, rerender } = await setupLoaded()

      rerender({ placeholderValues: { name: 'Ada' } })
      act(() => {
        vi.advanceTimersByTime(499)
      })
      expect(renderPrompt).not.toHaveBeenCalled()

      act(() => {
        vi.advanceTimersByTime(1)
      })
      expect(renderPrompt).toHaveBeenCalledExactlyOnceWith('greeting', 'team-1')
    })

    it('debounces: a second change restarts the 500 ms wait', async () => {
      const { renderPrompt, rerender } = await setupLoaded()

      rerender({ placeholderValues: { name: 'A' } })
      act(() => {
        vi.advanceTimersByTime(400)
      })
      rerender({ placeholderValues: { name: 'Ad' } })
      act(() => {
        vi.advanceTimersByTime(400)
      })
      expect(renderPrompt).not.toHaveBeenCalled()

      act(() => {
        vi.advanceTimersByTime(100)
      })
      expect(renderPrompt).toHaveBeenCalledTimes(1)
    })

    it('cancels the pending render on unmount', async () => {
      const { renderPrompt, rerender, unmount } = await setupLoaded()

      rerender({ placeholderValues: { name: 'Ada' } })
      unmount()
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      expect(renderPrompt).not.toHaveBeenCalled()
    })

    it('does not re-render when the values are a new object with the same contents', async () => {
      const { renderPrompt, rerender } = await setupLoaded()

      rerender({ placeholderValues: { name: '' } })
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      expect(renderPrompt).not.toHaveBeenCalled()
    })

    it('does not re-render on the Raw tab, and treats that value as seen', async () => {
      const { renderPrompt, rerender } = await setupLoaded()

      rerender({ tab: 'raw' })
      rerender({ placeholderValues: { name: 'Ada' } })
      act(() => {
        vi.advanceTimersByTime(1000)
      })
      expect(renderPrompt).not.toHaveBeenCalled()

      // Back on Rendered the first effect renders once, immediately; the
      // debounce has nothing new to add because the value was recorded.
      rerender({ tab: 'rendered' })
      expect(renderPrompt).toHaveBeenCalledTimes(1)
      act(() => {
        vi.advanceTimersByTime(1000)
      })
      expect(renderPrompt).toHaveBeenCalledTimes(1)
    })

    it.each<[string, Partial<Options>]>([
      ['the placeholders are still loading', { isLoadingPlaceholders: true }],
      ['the prompt has no placeholders', { placeholderCount: 0 }],
      ['no team is selected', { currentTeam: null }],
      ['the prompt is gone', { slug: undefined }],
    ])('does not re-render while %s', async (_name, overrides) => {
      const { renderPrompt, rerender } = await setupLoaded()

      rerender({ ...overrides, placeholderValues: { name: 'Ada' } })
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      expect(renderPrompt).not.toHaveBeenCalled()
    })

    it('does not re-render for a prompt whose first load has not started', () => {
      // On Raw the first-load effect never ran, so no slug is recorded as
      // loaded; a value change must not render ahead of that first load.
      const { renderPrompt, rerender } = setup({
        tab: 'raw',
        placeholderValues: { name: '' },
        placeholderCount: 1,
      })

      rerender({ placeholderValues: { name: 'Ada' } })
      act(() => {
        vi.advanceTimersByTime(1000)
      })

      expect(renderPrompt).not.toHaveBeenCalled()
    })
  })
})
