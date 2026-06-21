import { storageUtils } from '../storage'

// Raw storage is accessed via `globalThis.localStorage` so this test can verify
// cleanup of keys that are intentionally NOT in STORAGE_KEYS (the legacy
// `auth_token` and the removed `vx_auth_token`) — the typed `storage` wrapper
// only exposes current, sanctioned keys.
const rawLocal = globalThis.localStorage

describe('storageUtils.migrateStorageKeys', () => {
  beforeEach(() => {
    globalThis.localStorage.clear()
    globalThis.sessionStorage.clear()
  })

  it('removes the legacy auth_token from localStorage', () => {
    rawLocal.setItem('auth_token', 'stale-jwt')

    storageUtils.migrateStorageKeys()

    expect(rawLocal.getItem('auth_token')).toBeNull()
  })

  it('removes the old vx_auth_token from localStorage', () => {
    rawLocal.setItem('vx_auth_token', 'stale-jwt')

    storageUtils.migrateStorageKeys()

    expect(rawLocal.getItem('vx_auth_token')).toBeNull()
  })

  it('removes both auth token keys in a single pass', () => {
    rawLocal.setItem('auth_token', 'legacy-jwt')
    rawLocal.setItem('vx_auth_token', 'prefixed-jwt')

    storageUtils.migrateStorageKeys()

    expect(rawLocal.getItem('auth_token')).toBeNull()
    expect(rawLocal.getItem('vx_auth_token')).toBeNull()
  })

  it('does not re-create auth token keys when none exist', () => {
    storageUtils.migrateStorageKeys()

    expect(rawLocal.getItem('auth_token')).toBeNull()
    expect(rawLocal.getItem('vx_auth_token')).toBeNull()
  })

  it('migrates a legacy non-auth key to its prefixed key', () => {
    rawLocal.setItem('vibexp_current_team_id', 'team-42')

    storageUtils.migrateStorageKeys()

    expect(rawLocal.getItem('vx_current_team_id')).toBe('team-42')
    expect(rawLocal.getItem('vibexp_current_team_id')).toBeNull()
  })
})
