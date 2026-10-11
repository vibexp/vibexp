/**
 * @jest-environment jsdom
 */
import { getEnv } from '../../src/lib/runtimeEnv'

describe('getEnv', () => {
  afterEach(() => {
    delete window.__VIBEXP_ENV__
  })

  it('prefers a runtime value from window.__VIBEXP_ENV__', () => {
    window.__VIBEXP_ENV__ = {
      VITE_MCP_ENDPOINT: 'https://runtime.test/mcp/v1/common',
    }
    expect(getEnv('VITE_MCP_ENDPOINT')).toBe(
      'https://runtime.test/mcp/v1/common'
    )
  })

  it('falls back to build-time import.meta.env when no runtime value is set', () => {
    // vitest.config.ts seeds import.meta.env.VITE_API_BASE_URL
    expect(getEnv('VITE_API_BASE_URL')).toBe('https://api.vibexp.io/api/v1')
  })

  it('treats an empty runtime value as unset and falls back', () => {
    window.__VIBEXP_ENV__ = { VITE_API_BASE_URL: '' }
    expect(getEnv('VITE_API_BASE_URL')).toBe('https://api.vibexp.io/api/v1')
  })

  it('returns undefined when neither source provides a value', () => {
    expect(getEnv('VITE_ERROR_TYPE_BASE_URI')).toBeUndefined()
  })
})
