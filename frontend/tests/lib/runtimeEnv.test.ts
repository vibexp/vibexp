/**
 * @jest-environment jsdom
 */
import { getEnv } from '../../src/lib/runtimeEnv'

describe('getEnv', () => {
  afterEach(() => {
    delete window.__VIBEXP_ENV__
  })

  it('prefers a runtime value from window.__VIBEXP_ENV__', () => {
    window.__VIBEXP_ENV__ = { VITE_SITE_NAME: 'Runtime Brand' }
    expect(getEnv('VITE_SITE_NAME')).toBe('Runtime Brand')
  })

  it('falls back to build-time import.meta.env when no runtime value is set', () => {
    // jest.config.js seeds import.meta.env.VITE_GTM_ENABLED = 'false'
    expect(getEnv('VITE_GTM_ENABLED')).toBe('false')
  })

  it('treats an empty runtime value as unset and falls back', () => {
    window.__VIBEXP_ENV__ = { VITE_GTM_ENABLED: '' }
    expect(getEnv('VITE_GTM_ENABLED')).toBe('false')
  })

  it('returns undefined when neither source provides a value', () => {
    expect(getEnv('VITE_SITE_NAME')).toBeUndefined()
  })
})
