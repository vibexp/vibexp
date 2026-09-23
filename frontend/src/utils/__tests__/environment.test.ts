import { getBackendOrigin } from '../environment'

describe('getBackendOrigin', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.unstubAllGlobals()
  })

  it("returns the backend's origin for an absolute API base URL", () => {
    // Local dev: the SPA on :5173 talks to the backend on :8080 (#1129).
    vi.stubEnv('VITE_API_BASE_URL', 'http://localhost:8080/api/v1')

    expect(getBackendOrigin()).toBe('http://localhost:8080')
  })

  it('returns the browsing origin for a relative API base URL', () => {
    // The combined image (#61): SPA and API share one origin.
    vi.stubEnv('VITE_API_BASE_URL', '/api/v1')

    expect(getBackendOrigin()).toBe(window.location.origin)
  })

  it('returns the browsing origin for an empty API base URL', () => {
    vi.stubEnv('VITE_API_BASE_URL', '')

    expect(getBackendOrigin()).toBe(window.location.origin)
  })

  it('falls back to the browsing origin for a malformed API base URL', () => {
    vi.stubEnv('VITE_API_BASE_URL', 'http://[')

    expect(getBackendOrigin()).toBe(window.location.origin)
  })

  it('returns an empty string outside a DOM', () => {
    vi.stubGlobal('window', undefined)

    expect(getBackendOrigin()).toBe('')
  })
})
