import '@testing-library/jest-dom'

// Polyfill for TextEncoder/TextDecoder in Node.js test environment
import { TextEncoder, TextDecoder } from 'util'

// Mock import.meta.env for Vite environment variables
Object.defineProperty(globalThis, 'import', {
  value: {
    meta: {
      env: {
        DEV: false,
        PROD: true,
        MODE: 'test',
        BASE_URL: '/',
        SSR: false,
        // GTM/GA4 environment variables (disabled in tests)
        VITE_GTM_ENABLED: 'false',
        VITE_GTM_ID: '',
        VITE_GA4_MEASUREMENT_ID: '',
        VITE_API_BASE_URL: 'https://api.vibexp.io/api/v1',
      },
    },
  },
  writable: true,
})

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
