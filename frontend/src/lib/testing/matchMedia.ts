/**
 * Test-only `window.matchMedia` stand-in that answers `(min-width: …)`
 * queries for a chosen viewport width and lets a test resize the viewport
 * mid-run. jsdom ships no `matchMedia`, so `useMediaQuery` would otherwise
 * fall back to its desktop default.
 */
export function mockViewportWidth(initialWidth: number) {
  let width = initialWidth
  const listeners = new Set<() => void>()

  const matches = (query: string): boolean => {
    const m = /\(min-width:\s*([\d.]+)(px|rem)\)/.exec(query)
    if (!m) return false
    const threshold = parseFloat(m[1]) * (m[2] === 'rem' ? 16 : 1)
    return width >= threshold
  }

  // jsdom exposes the property but leaves it undefined; remember exactly what
  // was there so `restore` can put it back.
  const original =
    typeof window.matchMedia === 'function'
      ? window.matchMedia.bind(window)
      : undefined

  const fake = (query: string): MediaQueryList =>
    ({
      get matches() {
        return matches(query)
      },
      media: query,
      onchange: null,
      addEventListener: (_type: string, cb: () => void) => {
        listeners.add(cb)
      },
      removeEventListener: (_type: string, cb: () => void) => {
        listeners.delete(cb)
      },
      addListener: (cb: () => void) => {
        listeners.add(cb)
      },
      removeListener: (cb: () => void) => {
        listeners.delete(cb)
      },
      dispatchEvent: () => true,
    }) as unknown as MediaQueryList

  window.matchMedia = fake

  return {
    /** Change the viewport width and notify every subscribed query. */
    setWidth(next: number) {
      width = next
      for (const cb of listeners) cb()
    },
    restore() {
      if (original) window.matchMedia = original
      else {
        // @ts-expect-error -- jsdom has no matchMedia; put it back that way.
        delete window.matchMedia
      }
    },
  }
}
