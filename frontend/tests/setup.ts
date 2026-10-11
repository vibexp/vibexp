import '@testing-library/jest-dom'

// Polyfill for TextEncoder/TextDecoder in Node.js test environment (jsdom
// does not provide them; react-router@8's server-runtime/crypto touches
// TextEncoder at module scope).
import { TextEncoder, TextDecoder } from 'util'

// import.meta.env is native under Vitest — values come from Vite env handling
// (mode 'test'), so the jest-era `globals['import.meta']` shim is gone. Tests
// that gate on VITE_GTM_ID etc. rely on those being unset/empty here.

// Add TextEncoder/TextDecoder for Node.js environment
if (typeof global.TextEncoder === 'undefined') {
  global.TextEncoder = TextEncoder
  global.TextDecoder = TextDecoder
}

// Note: JSDOM navigation errors will be ignored by allowing them to fail silently
// We've structured our tests to avoid relying on actual navigation behavior

// Polyfill ResizeObserver for jsdom (used by Radix UI primitives like Select).
if (typeof global.ResizeObserver === 'undefined') {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
}

// The team's status catalog (#1263) is read by every resource form, filter bar,
// list and detail page, through `useStatusCatalog` — which needs a
// `TeamProvider` and asks the API. A suite about something else gets the answer
// of a team with no custom statuses, so those pages render exactly as they do
// from the descriptor alone. A suite about custom statuses mocks this module
// itself (a file-level `vi.mock` wins), and the hook's own suite `vi.unmock`s it.
vi.mock('@/hooks/useStatusCatalog', () => {
  // One stable answer, as the real hook gives: readers memoise on it.
  const answered = { statuses: [], isLoading: false }
  return {
    FRESH_FOR_MS: 5000,
    resetStatusCatalog: () => {},
    useStatusCatalog: () => answered,
  }
})
