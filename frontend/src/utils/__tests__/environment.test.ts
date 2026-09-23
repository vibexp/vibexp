import { getApiBaseUrl, getBackendOrigin } from '../environment'

afterEach(() => {
  vi.unstubAllEnvs()
  vi.unstubAllGlobals()
})

describe('getApiBaseUrl outside the test environment (Vite build)', () => {
  beforeEach(() => {
    vi.stubEnv('NODE_ENV', 'production')
  })

  it('prefers an explicit VITE_API_BASE_URL', () => {
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api/v1')
    vi.stubEnv('DEV', true)

    expect(getApiBaseUrl()).toBe('https://api.example.com/api/v1')
  })

  it('falls back to the local backend in development mode', () => {
    vi.stubEnv('VITE_API_BASE_URL', '')
    vi.stubEnv('DEV', true)

    expect(getApiBaseUrl()).toBe('http://localhost:8080/api/v1')
  })

  it('falls back to same-origin relative requests otherwise', () => {
    vi.stubEnv('VITE_API_BASE_URL', '')
    vi.stubEnv('DEV', false)

    expect(getApiBaseUrl()).toBe('')
  })
})

describe('getBackendOrigin', () => {
  it("returns the backend's origin for an absolute API base URL", () => {
    // Local dev: the SPA on :5173 talks to the backend on :8080 (#1129).
    vi.stubEnv('VITE_API_BASE_URL', 'http://localhost:8080/api/v1')

    expect(getBackendOrigin()).toBe('http://localhost:8080')
  })

  // Relative / empty: the combined image (#61), where SPA and API share one
  // origin. Malformed: nothing sensible to resolve, so stay on the SPA's.
  it.each([
    ['relative', '/api/v1'],
    ['empty', ''],
    ['malformed', 'http://['],
  ])('returns the browsing origin for a %s API base URL', (_kind, base) => {
    vi.stubEnv('VITE_API_BASE_URL', base)

    expect(getBackendOrigin()).toBe(window.location.origin)
  })

  it('returns an empty string outside a DOM', () => {
    vi.stubGlobal('window', undefined)

    expect(getBackendOrigin()).toBe('')
  })
})
