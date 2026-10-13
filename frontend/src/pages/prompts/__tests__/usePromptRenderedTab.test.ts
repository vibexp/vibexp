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
      expect(renderPrompt).toHaveBeenCalledExactlyOnceWith(
        'greeting',
        'team-1',
        {}
      )
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
      expect(renderPrompt).toHaveBeenCalledExactlyOnceWith(
        'greeting',
        'team-1',
        {}
      )
    })

    it('waits for the placeholders to finish loading', () => {
      const { renderPrompt, rerender } = setup()

      rerender({ isLoadingPlaceholders: true })
      expect(renderPrompt).not.toHaveBeenCalled()

      rerender({ isLoadingPlaceholders: false })
      expect(renderPrompt).toHaveBeenCalledExactlyOnceWith(
        'greeting',
        'team-1',
        {}
      )
    })
  })

  describe('placeholder values', () => {
    it('always asks the server to render with no values', async () => {
      // The page substitutes values into the rendered body itself, so an
      // unfilled placeholder stays visible as a chip. A value sent to the
      // server (even an empty one) would erase it, so none ever is; and the
      // hook does not take the values at all, so typing one cannot re-render.
      const { renderPrompt, rerender } = setup()
      await flushFetch()
      act(() => {
        vi.advanceTimersByTime(100)
      })
      rerender({ tab: 'raw' })
      rerender({ tab: 'rendered' })

      expect(renderPrompt).toHaveBeenCalledTimes(2)
      for (const call of renderPrompt.mock.calls) {
        expect(call[2]).toEqual({})
      }
    })
  })
})
