import { getEnv } from '../runtimeEnv'

describe('getEnv', () => {
  afterEach(() => {
    delete window.__VIBEXP_ENV__
    vi.unstubAllEnvs()
  })

  it('prefers the value /config.js set at runtime', () => {
    vi.stubEnv('VITE_MCP_ENDPOINT', 'https://build.test/mcp')
    window.__VIBEXP_ENV__ = { VITE_MCP_ENDPOINT: 'https://runtime.test/mcp' }

    expect(getEnv('VITE_MCP_ENDPOINT')).toBe('https://runtime.test/mcp')
  })

  it('falls back to the build-time value, key by key', () => {
    vi.stubEnv('VITE_MCP_ENDPOINT', 'https://build.test/mcp')
    vi.stubEnv('VITE_GTM_ID', 'GTM-BUILD')
    window.__VIBEXP_ENV__ = { VITE_GTM_ID: 'GTM-RUNTIME' }

    expect(getEnv('VITE_GTM_ID')).toBe('GTM-RUNTIME')
    expect(getEnv('VITE_MCP_ENDPOINT')).toBe('https://build.test/mcp')
  })

  // What a local backend with no runtime frontend env sends, now that the dev
  // server forwards /config.js to it.
  it('falls back for every key when /config.js sets an empty object', () => {
    vi.stubEnv('VITE_MCP_ENDPOINT', 'https://build.test/mcp')
    window.__VIBEXP_ENV__ = {}

    expect(getEnv('VITE_MCP_ENDPOINT')).toBe('https://build.test/mcp')
  })

  it('treats an empty runtime value as unset', () => {
    vi.stubEnv('VITE_MCP_ENDPOINT', 'https://build.test/mcp')
    window.__VIBEXP_ENV__ = { VITE_MCP_ENDPOINT: '' }

    expect(getEnv('VITE_MCP_ENDPOINT')).toBe('https://build.test/mcp')
  })
})
