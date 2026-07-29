import '@testing-library/jest-dom'

// Polyfill for TextEncoder/TextDecoder in Node.js test environment (jsdom
// does not provide them; react-router@8's server-runtime/crypto touches
// TextEncoder at module scope).
import { TextEncoder, TextDecoder } from 'util'

// import.meta.env is native under Vitest — values come from Vite env handling
// (mode 'test'), so the jest-era `globals['import.meta']` shim is gone. Tests
// that gate on VITE_GTM_ENABLED etc. rely on those being unset/empty here.

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
